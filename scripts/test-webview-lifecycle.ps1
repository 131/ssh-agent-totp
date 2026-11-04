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
 [DllImport("user32.dll")] public static extern bool EnumChildWindows(IntPtr parent, EnumProc callback, IntPtr data);
 [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr hwnd, out uint pid);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetClassName(IntPtr hwnd, StringBuilder name, int size);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetWindowText(IntPtr hwnd, StringBuilder text, int size);
 [DllImport("user32.dll")] public static extern bool PostMessage(IntPtr hwnd, uint msg, IntPtr w, IntPtr l);
 [DllImport("user32.dll")] public static extern bool IsWindow(IntPtr hwnd);
 [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr hwnd);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern IntPtr FindWindowEx(IntPtr parent, IntPtr after, string name, string title);
 [DllImport("user32.dll")] public static extern IntPtr SendMessage(IntPtr hwnd, uint msg, IntPtr w, IntPtr l);
 [DllImport("user32.dll")] public static extern int GetMenuItemCount(IntPtr menu);
 [DllImport("user32.dll", CharSet=CharSet.Unicode)] public static extern int GetMenuString(IntPtr menu, uint item, StringBuilder text, int count, uint flags);
 public static IntPtr Tray(uint pid) {
  IntPtr hwnd = IntPtr.Zero;
  while ((hwnd = FindWindowEx(new IntPtr(-3), hwnd, "WailsWebviewWindow", null)) != IntPtr.Zero) {
   uint owner; GetWindowThreadProcessId(hwnd, out owner);
   if (owner == pid) return hwnd;
  }
  return IntPtr.Zero;
 }
 public static IntPtr Popup(uint pid) {
  IntPtr found = IntPtr.Zero;
  EnumWindows((hwnd, data) => {
   uint owner; GetWindowThreadProcessId(hwnd, out owner);
   var name = new StringBuilder(256); GetClassName(hwnd, name, name.Capacity);
   if (owner == pid && name.ToString() == "#32768" && IsWindowVisible(hwnd)) { found = hwnd; return false; }
   return true;
  }, IntPtr.Zero);
  return found;
 }
 public static string MenuLabel(IntPtr menu, uint position) {
  var text = new StringBuilder(256);
  GetMenuString(menu, position, text, text.Capacity, 0x400);
  return text.ToString();
 }
 public static bool VisibleBrowser(IntPtr parent) {
  bool visible = false;
  EnumChildWindows(parent, (hwnd, data) => {
   var name = new StringBuilder(256); GetClassName(hwnd, name, name.Capacity);
   if (name.ToString().StartsWith("Chrome_WidgetWin_") && IsWindowVisible(hwnd)) visible = true;
   return true;
  }, IntPtr.Zero);
  return visible;
 }
 public static IntPtr Find(uint pid, string title) {
  IntPtr found = IntPtr.Zero;
  EnumWindows((hwnd, data) => {
   uint owner; GetWindowThreadProcessId(hwnd, out owner);
   var name = new StringBuilder(256); GetClassName(hwnd, name, name.Capacity);
   var text = new StringBuilder(256); GetWindowText(hwnd, text, text.Capacity);
   if (owner == pid && name.ToString() == "WailsWebviewWindow" && text.ToString() == title) { found = hwnd; return false; }
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
 $window = [LifecycleWin]::Find($Owner, 'ssh-agent-totp')
 if ($window -ne [IntPtr]::Zero -and [LifecycleWin]::IsWindowVisible($window) -and [LifecycleWin]::VisibleBrowser($window)) { return $window }
 return [IntPtr]::Zero
}
# Wails' single-instance lock forwards a second launch to the resident process.
function Show-Gui {
 $second = Start-Process $Executable -PassThru
 $null = $second.Handle
 if (-not $second.WaitForExit(10000)) { throw 'Second instance did not exit' }
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
 Wait-For { $process.HasExited -or [LifecycleWin]::Find($process.Id, '__wails_hidden_mainthread') -ne [IntPtr]::Zero } 'No tray host'
 if ($process.HasExited) { throw 'App exited at startup (another instance may be running)' }
 $hostWindow = [LifecycleWin]::Find($process.Id, '__wails_hidden_mainthread')
 Start-Sleep -Seconds 1
 if ([LifecycleWin]::IsWindowVisible($hostWindow)) { throw 'Tray host is visible' }
 if ([LifecycleWin]::Find($process.Id, 'ssh-agent-totp') -ne [IntPtr]::Zero -or @(Browser-Tree $process.Id).Count) { throw 'Browser created at startup' }
 'PASS: tray-only startup'
 # The first right-click must work before any primary click or unlock.
 $trayWindow = [LifecycleWin]::Tray($process.Id)
 if ($trayWindow -eq [IntPtr]::Zero) { throw 'No message-only tray window' }
 [LifecycleWin]::PostMessage($trayWindow, 0x401, [IntPtr]::Zero, [IntPtr]0x7B) | Out-Null
 Wait-For { [LifecycleWin]::Popup($process.Id) -ne [IntPtr]::Zero } 'First native context menu did not open'
 $popup = [LifecycleWin]::Popup($process.Id)
 $menu = [LifecycleWin]::SendMessage($popup, 0x1E1, [IntPtr]::Zero, [IntPtr]::Zero)
 if ([LifecycleWin]::GetMenuItemCount($menu) -ne 3 -or
     [LifecycleWin]::MenuLabel($menu, 0) -ne 'Ouvrir' -or
     [LifecycleWin]::MenuLabel($menu, 2) -ne 'Quitter') { throw 'Initial menu is not Open / separator / Quit' }
 [LifecycleWin]::PostMessage($popup, 0x100, [IntPtr]0x1B, [IntPtr]::Zero) | Out-Null
 Wait-For { [LifecycleWin]::Popup($process.Id) -eq [IntPtr]::Zero } 'Context menu did not close'
 if ([LifecycleWin]::Find($process.Id, 'ssh-agent-totp') -ne [IntPtr]::Zero) { throw 'Context menu created a GUI' }
 'PASS: first native context menu contains Open and Quit before unlock'
 # One primary-click event creates the GUI, without a double-click or relaunch.
 [LifecycleWin]::PostMessage($trayWindow, 0x401, [IntPtr]::Zero, [IntPtr]0x202) | Out-Null
 Wait-For { (Visible-Gui $process.Id) -ne [IntPtr]::Zero } 'Primary click did not open GUI'
 $primaryGui = [LifecycleWin]::Find($process.Id, 'ssh-agent-totp')
 [LifecycleWin]::PostMessage($trayWindow, 0x401, [IntPtr]::Zero, [IntPtr]0x202) | Out-Null
 Start-Sleep -Milliseconds 300
 if ([LifecycleWin]::Find($process.Id, 'ssh-agent-totp') -ne $primaryGui) { throw 'Repeated primary click replaced GUI' }
 $primaryBrowsers = @(Browser-Tree $process.Id)
 Post $primaryGui 0x10
 Wait-For { -not [LifecycleWin]::IsWindow($primaryGui) } 'Primary-click GUI did not close'
 Wait-For { @(Get-Process -Id $primaryBrowsers -ErrorAction SilentlyContinue).Count -eq 0 } 'Primary-click browser survived Close'
 'PASS: primary click opens GUI; repeated click restores the same session'
 for ($cycle = 1; $cycle -le 3; $cycle++) {
  Show-Gui
  Wait-For { (Visible-Gui $process.Id) -ne [IntPtr]::Zero } 'GUI did not open'
  $gui = [LifecycleWin]::Find($process.Id, 'ssh-agent-totp')
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
 $gui = [LifecycleWin]::Find($process.Id, 'ssh-agent-totp')
 Wait-For { @(Browser-Tree $process.Id).Count -gt 0 } 'Second launch did not start browser'
 'PASS: second launch recreates GUI in the original process'
 # System minimize must hide, not destroy, the existing GUI/browser session.
 $retainedGui = $gui
 $retainedBrowsers = @(Browser-Tree $process.Id)
 if (-not [LifecycleWin]::PostMessage($gui, 0x112, [IntPtr]0xF020, [IntPtr]::Zero)) { throw 'Native minimize failed' }
 Wait-For { -not [LifecycleWin]::IsWindowVisible($retainedGui) } 'Minimize did not hide GUI'
 if (-not [LifecycleWin]::IsWindow($retainedGui)) { throw 'Minimize destroyed GUI' }
 if (@(Get-Process -Id $retainedBrowsers -ErrorAction SilentlyContinue).Count -ne $retainedBrowsers.Count) { throw 'Minimize destroyed browser processes' }
 Show-Gui
 Wait-For { (Visible-Gui $process.Id) -ne [IntPtr]::Zero } 'Hidden GUI did not reopen'
 $gui = [LifecycleWin]::Find($process.Id, 'ssh-agent-totp')
 if ($gui -ne $retainedGui) { throw 'Reopen replaced the minimized session' }
 if (@(Get-Process -Id $retainedBrowsers -ErrorAction SilentlyContinue).Count -ne $retainedBrowsers.Count) { throw 'Reopen replaced browser processes' }
 'PASS: native minimize hides GUI and reopen retains its window/browser'
 # Immediate close/open must not crash or terminate the host's queue.
 Post $gui 0x10
 Show-Gui
 Wait-For { $process.HasExited -or (Visible-Gui $process.Id) -ne [IntPtr]::Zero } 'Rapid reopen failed'
 Start-Sleep -Seconds 2
 if ($process.HasExited) { throw 'Rapid reopen exited the application' }
 $gui = [LifecycleWin]::Find($process.Id, 'ssh-agent-totp')
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
 Wait-For { $process.HasExited -or [LifecycleWin]::Find($process.Id, '__wails_hidden_mainthread') -ne [IntPtr]::Zero } 'No host for initialization-quit check'
 if ($process.HasExited) { throw 'App exited before initialization-quit check' }
 $hostWindow = [LifecycleWin]::Find($process.Id, '__wails_hidden_mainthread')
 Show-Gui
 Post $hostWindow 0x10
 if (-not $process.WaitForExit(20000)) { throw 'Quit during WebView initialization did not exit' }
 # Wails v3.1 treats WM_QUIT during its synchronous controller wait as a
 # WebView2 error: the process exits, but with code 1.
 if ($process.ExitCode -ne 0) { "WARN: Quit during WebView initialization exited with code $($process.ExitCode)" }
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
