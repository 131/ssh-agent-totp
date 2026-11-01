param([Parameter(Mandatory = $true)][string]$Executable)
$ErrorActionPreference = 'Stop'
$Executable = (Resolve-Path $Executable).Path
Add-Type @'
using System;
using System.Text;
using System.Runtime.InteropServices;
public class LifecycleWin {
 public delegate bool EnumProc(IntPtr hwnd, IntPtr data);
 [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc callback, IntPtr data);
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr hwnd, StringBuilder name, int size);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern uint RegisterWindowMessage(string text);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr hwnd, uint msg, IntPtr w, IntPtr l);
 [DllImport("user32.dll")] public static extern bool IsWindow(IntPtr hwnd);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
 public static IntPtr Find(uint pid, string cls) {
  IntPtr found = IntPtr.Zero;
  EnumWindows((hwnd, data) => {
   uint owner; GetWindowThreadProcessId(hwnd, out owner);
   var name = new StringBuilder(256); GetClassName(hwnd, name, name.Capacity);
   if (owner == pid && name.ToString() == cls) { found = hwnd; return false; }
   return true;
  }, IntPtr.Zero);
  return found;
 }
}
'@
function Wait-For([scriptblock]$Condition, [string]$Failure) {
 $end = (Get-Date).AddSeconds(20)
 while (-not (& $Condition)) {
  if ((Get-Date) -gt $end) { throw $Failure }
  Start-Sleep -Milliseconds 100
 }
}
function Browser-Tree([int]$Owner) {
 $all = @(Get-CimInstance Win32_Process)
 $parents = @($Owner)
 $result = @()
 while ($parents.Count) {
  $children = @($all | Where-Object { $_.ParentProcessId -in $parents -and $_.Name -eq 'msedgewebview2.exe' })
  $parents = @($children | Select-Object -ExpandProperty ProcessId)
  $result += $parents
 }
 return $result
}
# Visibility is the publication boundary: a hidden HWND can exist while its
# controller/frame are still being constructed and is not yet an open session.
function Visible-Gui([int]$Owner) {
 $window = [LifecycleWin]::Find($Owner, 'webview')
 if ($window -ne [IntPtr]::Zero -and [LifecycleWin]::IsWindowVisible($window)) { return $window }
 return [IntPtr]::Zero
}
function Post([IntPtr]$Window, [uint32]$Message) {
 if (-not [LifecycleWin]::PostMessage($Window, $Message, [IntPtr]::Zero, [IntPtr]::Zero)) { throw 'PostMessage failed' }
}
$previous = @($env:APPDATA, $env:LOCALAPPDATA, $env:SSH_AUTH_SOCK)
$testData = Join-Path ([IO.Path]::GetTempPath()) ('ssh-agent-totp-lifecycle-' + [Guid]::NewGuid().ToString('N'))
$env:APPDATA = Join-Path $testData 'appdata'
$env:LOCALAPPDATA = Join-Path $testData 'localappdata'
$env:SSH_AUTH_SOCK = '\\.\pipe\ssh-agent-totp-no-agent-test'
$settings = Join-Path $env:APPDATA 'ssh-agent-totp'
New-Item -ItemType Directory -Path $settings -Force | Out-Null
Set-Content -Path (Join-Path $settings 'settings.yml') -Encoding UTF8 -Value "update:`n  enable: false"
$stderr = Join-Path $testData 'application.stderr'
$process = Start-Process $Executable -PassThru -RedirectStandardError $stderr
$null = $process.Handle
$hostWindow = [IntPtr]::Zero
try {
 Wait-For { $process.HasExited -or [LifecycleWin]::Find($process.Id, 'ssh-agent-totp-tray') -ne [IntPtr]::Zero } 'No tray host'
 if ($process.HasExited) { throw 'App exited at startup (another instance may be running)' }
 $hostWindow = [LifecycleWin]::Find($process.Id, 'ssh-agent-totp-tray')
 Start-Sleep -Seconds 1
 if ([LifecycleWin]::IsWindowVisible($hostWindow)) { throw 'Tray host is visible' }
 if ([LifecycleWin]::Find($process.Id, 'webview') -ne [IntPtr]::Zero -or @(Browser-Tree $process.Id).Count) { throw 'Browser created at startup' }
 'PASS: tray-only startup'
 $show = [LifecycleWin]::RegisterWindowMessage('ssh-agent-totp-show')
 for ($cycle = 1; $cycle -le 3; $cycle++) {
  Post $hostWindow $show
  Wait-For { (Visible-Gui $process.Id) -ne [IntPtr]::Zero } 'GUI did not open'
  $gui = [LifecycleWin]::Find($process.Id, 'webview')
  Wait-For { @(Browser-Tree $process.Id).Count -gt 0 } 'No browser processes on Open'
  # Let the page's async bindings and lock screen initialize before closing.
  Start-Sleep -Seconds 2
  $browsers = @(Browser-Tree $process.Id)
  Post $gui 0x10
  Wait-For { -not [LifecycleWin]::IsWindow($gui) } 'GUI was hidden instead of destroyed'
  Wait-For { @(Get-Process -Id $browsers -ErrorAction SilentlyContinue).Count -eq 0 } 'WebView2 processes survived Close'
  if ($process.HasExited -or -not [LifecycleWin]::IsWindow($hostWindow)) { throw 'Close killed tray' }
  "PASS: cycle $cycle creates GUI, destroys it and exits all $($browsers.Count) browser processes"
 }
 # Reopening through a second launch must work while only the tray exists.
 $second = Start-Process $Executable -PassThru
 $null = $second.Handle
 if (-not $second.WaitForExit(10000)) { throw 'Second instance did not exit' }
 if ($second.ExitCode -ne 0) { throw 'Second instance failed' }
 Wait-For { (Visible-Gui $process.Id) -ne [IntPtr]::Zero } 'Second launch did not recreate GUI'
 $gui = [LifecycleWin]::Find($process.Id, 'webview')
 Wait-For { @(Browser-Tree $process.Id).Count -gt 0 } 'Second launch did not start browser'
 'PASS: second launch recreates GUI in the original process'
 # Immediate close/open must not crash or terminate the host's queue.
 Post $gui 0x10
 Post $hostWindow $show
 Wait-For { $process.HasExited -or (Visible-Gui $process.Id) -ne [IntPtr]::Zero } 'Rapid reopen failed'
 Start-Sleep -Seconds 2
 if ($process.HasExited) { throw 'Rapid reopen exited the application' }
 $gui = [LifecycleWin]::Find($process.Id, 'webview')
 if ($gui -eq [IntPtr]::Zero) { throw 'Rapid reopen left no GUI' }
 $browsers = @(Browser-Tree $process.Id)
 Post $gui 0x10
 Wait-For { -not [LifecycleWin]::IsWindow($gui) } 'Rapid-cycle GUI did not close'
 Wait-For { @(Get-Process -Id $browsers -ErrorAction SilentlyContinue).Count -eq 0 } 'Rapid-cycle browsers survived Close'
 'PASS: rapid close/reopen'
 Post $hostWindow 0x10
 if (-not $process.WaitForExit(10000)) { throw 'Quit without GUI did not exit' }
 'PASS: Quit without GUI'
 # Keep a process handle before rapid exit: PowerShell can otherwise expose
 # a null ExitCode after WaitForExit on a process whose handle was not cached.
 # Quit queued behind Open must still terminate during WebView initialization.
 $stderr = Join-Path $testData 'initialization.stderr'
 $process = Start-Process $Executable -PassThru -RedirectStandardError $stderr
$null = $process.Handle
 $hostWindow = [IntPtr]::Zero
 Wait-For { $process.HasExited -or [LifecycleWin]::Find($process.Id, 'ssh-agent-totp-tray') -ne [IntPtr]::Zero } 'No host for initialization-quit check'
 if ($process.HasExited) { throw 'App exited before initialization-quit check' }
 $hostWindow = [LifecycleWin]::Find($process.Id, 'ssh-agent-totp-tray')
 Post $hostWindow $show
 Post $hostWindow 0x10
 if (-not $process.WaitForExit(20000)) { throw 'Quit during WebView initialization did not exit' }
 if ($process.ExitCode -ne 0) { throw 'Quit during initialization failed' }
 'PASS: Quit during WebView initialization'
} catch {
 $message = $_.Exception.Message -replace '[\r\n]', ' '
 if ($process.HasExited) { $message += "; exit code=$($process.ExitCode)" }
 $lines = @(Get-Content $stderr -ErrorAction SilentlyContinue)
 $details = @($lines | Select-Object -First 15) -join ' | '
 if ($details) { $message += "; stderr=$details" }
 Write-Output "::error::$message"
 throw
} finally {
 if (-not $process.HasExited -and $hostWindow -ne [IntPtr]::Zero) {
  [LifecycleWin]::PostMessage($hostWindow, 0x10, [IntPtr]::Zero, [IntPtr]::Zero) | Out-Null
  $process.WaitForExit(5000) | Out-Null
 }
 $env:APPDATA, $env:LOCALAPPDATA, $env:SSH_AUTH_SOCK = $previous
}
