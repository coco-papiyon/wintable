# Runs inside the unmodified, digest-pinned Server Core image, with no Go install.
$ErrorActionPreference = 'Stop'
$root = 'C:/validation'
# Server Core ships a WSL launcher even though the subsystem is unavailable.
# Remove that launcher in this disposable container's writable layer only.
$launcher = 'C:\Windows\System32\wsl.exe'
if (Test-Path -LiteralPath $launcher) {
    & takeown.exe /F $launcher /A | Out-Null
    if ($LASTEXITCODE) { throw 'Could not take ownership of container WSL launcher' }
    & icacls.exe $launcher /grant '*S-1-5-32-544:F' | Out-Null
    if ($LASTEXITCODE) { throw 'Could not remove container WSL launcher protection' }
    Remove-Item -LiteralPath $launcher -Force
    Write-Output 'Preparation: removed preinstalled WSL launcher from disposable controller layer'
}
$environment = [ordered]@{
    os = [Environment]::OSVersion.VersionString
    architecture = $env:PROCESSOR_ARCHITECTURE
    powershell = $PSVersionTable.PSVersion.ToString()
    tools = @{}
    exe_sha256 = (Get-FileHash "$root/wintable.exe" -Algorithm SHA256).Hash
}
foreach ($tool in 'go','wsl','python','python3','py') {
    $found = @(Get-Command $tool -ErrorAction SilentlyContinue)
    $environment.tools[$tool] = @($found | ForEach-Object { $_.Source })
    if ($found.Count) { throw "Clean controller unexpectedly contains $tool" }
}
$environment | ConvertTo-Json -Depth 6 | Write-Output
function Invoke-Case($Name, $Expected, $Extra) {
    $arguments = @('run','--inventory',"$root/inventory.json",'--tasks',"$root/tasks.json",'--format','json') + $Extra
    $text = & "$root/wintable.exe" @arguments
    $code = $LASTEXITCODE
    [pscustomobject]@{ case=$Name; command=('wintable.exe ' + ($arguments -join ' ')); exit_code=$code; output=($text -join "`n") } | ConvertTo-Json -Depth 8 | Write-Output
    if ($code -ne 0) { throw "$Name exited $code" }
    $report = ($text -join "`n") | ConvertFrom-Json
    if ($report.hosts.Count -ne 1 -or $report.hosts[0].tasks.Count -ne 2) { throw 'Incorrect result shape' }
    if ($report.hosts[0].tasks[0].status -ne 'changed' -or $report.hosts[0].tasks[1].status -ne $Expected) { throw 'Unexpected command/file result' }
    if ($report.counts.failed -ne 0 -or $report.counts.unreachable -ne 0) { throw 'Unexpected failure counts' }
}
Invoke-Case 'V-13-first-command-file' 'changed' @('--include-output')
Invoke-Case 'V-13-idempotence' 'ok' @()
Write-Output 'CLEAN_CONTROLLER_PASS'
