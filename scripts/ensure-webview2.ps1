# CI prerequisite only; the application does not run this installer.
$ErrorActionPreference = 'Stop'
function Runtime-Version {
 $client = '{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}'
 foreach ($base in @('HKLM:\SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients', 'HKLM:\SOFTWARE\Microsoft\EdgeUpdate\Clients', 'HKCU:\Software\Microsoft\EdgeUpdate\Clients')) {
  $value = (Get-ItemProperty -Path "$base\$client" -Name pv -ErrorAction SilentlyContinue).pv
  if ($value -and [version]$value -gt [version]'0.0.0.0') { return $value }
 }
}
try {
 $version = Runtime-Version
 if ($version) { "WebView2 runtime available: $version"; return }
 $installer = Join-Path $env:RUNNER_TEMP 'MicrosoftEdgeWebview2Setup.exe'
 Invoke-WebRequest 'https://go.microsoft.com/fwlink/p/?LinkId=2124703' -OutFile $installer
 $signature = Get-AuthenticodeSignature $installer
 if ($signature.Status -ne 'Valid' -or $signature.SignerCertificate.Subject -notmatch 'Microsoft Corporation') { throw 'WebView2 installer signature rejected' }
 $setup = Start-Process $installer -ArgumentList '/silent', '/install' -Wait -PassThru
 if ($setup.ExitCode -ne 0) { throw "WebView2 setup failed: $($setup.ExitCode)" }
 $end = (Get-Date).AddMinutes(2)
 do {
  $version = Runtime-Version
  if ($version) { "WebView2 runtime installed: $version"; return }
  Start-Sleep -Seconds 2
 } while ((Get-Date) -lt $end)
 throw 'WebView2 setup completed without a registered runtime'
} catch {
 Write-Output "::error::$($_.Exception.Message -replace '[\r\n]', ' ')"
 throw
}
