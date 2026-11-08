#requires -Version 7.3
[CmdletBinding()]
param([string]$DockerContext='desktop-linux', [string]$TunnelHost, [string]$OutputDirectory)
$ErrorActionPreference='Stop'
if (!$OutputDirectory) { $OutputDirectory="$PSScriptRoot/../../.cache/linux-ssh-verification/$([guid]::NewGuid().ToString('N'))" }
$output=[IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) {throw 'OutputDirectory must be new'}
New-Item -ItemType Directory -Path $output | Out-Null
$results=@()
foreach ($case in @('normal','partial','startup','test','ready-timeout','timeout','cleanup','bad-lock')) {
    $argsList=@('-NoProfile','-File',"$PSScriptRoot/run.ps1",'-DockerContext',$DockerContext,'-OutputDirectory',"$output/$case")
    if ($TunnelHost) {$argsList+=@('-TunnelHost',$TunnelHost)}
    if ($case -eq 'normal') {$argsList+='-NoCache'}
    elseif ($case -eq 'bad-lock') {
        $lock=Get-Content "$PSScriptRoot/fixture.lock.json" -Raw | ConvertFrom-Json
        $lock.python='3.13.0'
        $lock | ConvertTo-Json -Depth 8 | Set-Content "$output/bad-lock.json" -Encoding utf8NoBOM
        $argsList+=@('-LockFile',"$output/bad-lock.json")
    } else {$argsList+=@('-Fault',$case)}
    # Each child has run(600s) + cleanup(90s); suite watchdog joins/kills it.
    $info=[Diagnostics.ProcessStartInfo]::new('pwsh'); $info.UseShellExecute=$false; $info.CreateNoWindow=$true
    foreach ($arg in $argsList) {$info.ArgumentList.Add($arg)}
    $process=[Diagnostics.Process]::Start($info)
    if (!$process.WaitForExit(720000)) {$process.Kill($true);$process.WaitForExit();throw "suite watchdog: $case"}
    $exitCode=$process.ExitCode; $process.Dispose()
    $manifest=Get-Content "$output/$case/manifest.json" -Raw | ConvertFrom-Json
    $shouldPass=$case -eq 'normal'
    if (($exitCode -eq 0) -ne $shouldPass) {throw "unexpected exit for $case"}
    if ($manifest.status -ne $(if($shouldPass){'passed'}else{'failed'})) {throw "unexpected status for $case"}
    $expectedFailure=switch ($case) {
        'partial' {'injected_partial_failure'}; 'startup' {'sshd startup failed'}; 'test' {'injected_test_failure'}
        'ready-timeout' {'ready_deadline_exceeded'}; 'timeout' {'deadline_exceeded'}; 'bad-lock' {'image version lock mismatch'}
    }
    if ($expectedFailure -and $manifest.failure -notlike "$expectedFailure*") {throw "wrong failure phase for $case"}
    $physical=@($manifest.cleanup | Where-Object { $_.kind -ne 'injection' })
    if ($physical.Count -ne 4 -or @($physical | Where-Object { $_.remaining -ne 0 -or $_.error }).Count) {throw "residual resource for $case"}
    if ($manifest.cleanup_ok -ne ($case -ne 'cleanup')) {throw "wrong cleanup status for $case"}
    $results+=@{case=$case;exit_code=$exitCode;expected_exit=$(if($shouldPass){'0'}else{'nonzero'});actual_status=$manifest.status;physical_remaining=0;cleanup_ok=$manifest.cleanup_ok;run_id=$manifest.run_id}
    $results | ConvertTo-Json -Depth 6 | Set-Content "$output/summary.json" -Encoding utf8NoBOM
}
Write-Host "All fixture verification cases passed: $output"
