param(
    [Parameter(Mandatory)][string]$EvidenceDirectory,
    [Parameter(Mandatory)][string]$SourceSHA,
    [Parameter(Mandatory)][string]$DistributionDirectory,
    [Parameter(Mandatory)][string]$WindowsJob,
    [Parameter(Mandatory)][string]$LinuxJob,
    [ValidateSet('local','github-actions')][string]$WindowsExecution = 'local',
    [ValidateSet('local','github-actions')][string]$LinuxExecution = 'local',
    [string]$WorkflowRunId
)
$ErrorActionPreference='Stop'
if ($WindowsJob -cne 'success' -or $LinuxJob -cne 'success') { throw 'Both real SSH jobs must succeed' }
if (($WindowsExecution -eq 'github-actions' -or $LinuxExecution -eq 'github-actions') -and $WorkflowRunId -notmatch '^[1-9][0-9]*$') { throw 'CI execution requires a workflow run ID' }
& "$PSScriptRoot/check-package.ps1" -Directory $DistributionDirectory -SourceSHA $SourceSHA
$hash=(Get-FileHash "$DistributionDirectory/wintable.exe").Hash
function Read-JSON($path) { Get-Content -LiteralPath "$EvidenceDirectory/$path" -Raw | ConvertFrom-Json }
$w=Read-JSON 'windows/manifest.json'
$ws=Read-JSON 'windows/ssh/manifest.json'
$l=Read-JSON 'linux/manifest.json'
$ph=Read-JSON 'linux/product-hash.json'
$example=Read-JSON 'linux/document-example.json'
$controller=Read-JSON 'windows/ssh/controller.json'
if ($null -eq $controller.zip_exit_code -or $controller.zip_exit_code -ne 0 -or
    $controller.zip_container_removed -ne $true -or $controller.zip_sha256 -cne (Get-FileHash -LiteralPath "$DistributionDirectory.zip").Hash -or
    !(Select-String -LiteralPath "$EvidenceDirectory/windows/ssh/zip-controller.log" -SimpleMatch 'ZIP_CONTROLLER_PASS' -Quiet)) {
    throw 'Windows ZIP controller acceptance incomplete'
}
if (@($l.source_status | Where-Object { $_ }).Count -or $l.go -notmatch 'go1\.27\.3') { throw 'Linux source/toolchain mismatch' }
foreach ($m in @($w,$ws,$l,$example)) { if ($m.source_sha -cne $SourceSHA) { throw 'Evidence source mismatch' } }
foreach ($h in @($w.exe_sha256,$ph.Hash,$example.exe_sha256)) { if ($h -cne $hash) { throw 'Evidence executable mismatch' } }
if ($w.acceptance -cne 'passed' -or $w.prerequisites_passed -ne $true -or $ws.container_removed -ne $true -or $ws.image_removed -ne $true -or
    $l.status -cne 'passed' -or $l.cleanup_ok -ne $true -or $l.product.exit_code -ne 0 -or $null -eq $l.product.exit_code -or
    $example.status -cne 'passed' -or $null -eq $example.exit_code -or $example.exit_code -ne 0) { throw 'Acceptance or cleanup incomplete' }
foreach ($kind in 'container','network','image','credentials') {
    $c=@($l.cleanup | Where-Object kind -ceq $kind)
    if ($c.Count -ne 1 -or $null -eq $c[0].remaining -or $c[0].remaining -ne 0 -or $c[0].error) { throw 'Linux cleanup evidence incomplete' }
}
$duration=([DateTimeOffset]$l.finished_utc)-([DateTimeOffset]$l.started_utc)
if ($duration.TotalSeconds -le 0 -or $duration.TotalSeconds -gt 690) { throw 'Linux deadline exceeded' }
$versions=Read-JSON 'linux/version-comparison.json'
if ($versions.match -ne $true) { throw 'Linux versions mismatch' }
if ($l.lock_sha256 -cne (Get-FileHash "$PSScriptRoot/../linux-ssh/fixture.lock.json").Hash) { throw 'Linux fixture lock mismatch' }
foreach ($name in 'TestHostOS','TestOSSchemaBoundaries','TestOSPreflightNoConnections','TestMixedBackends','TestLinuxBoundaries','TestLinuxFixedBackend','TestLinuxDecoder') {
    $records=@(Get-Content "$EvidenceDirectory/windows/test.log" | Where-Object { $_ -match '^\s*--- (PASS|FAIL|SKIP): (\S+)' -and $Matches[2] -ceq $name })
    if ($records.Count -ne 1 -or $records[0] -notmatch '^\s*--- PASS:') { throw "Missing/duplicate unit coverage: $name" }
}
# Recompute Windows coverage from raw files; never trust a supplied PASS alone.
& pwsh -NoProfile -File "$PSScriptRoot/../validation/summarize.ps1" -EvidenceDirectory "$EvidenceDirectory/windows"
if ($LASTEXITCODE) { throw 'Windows coverage incomplete' }
$coverage=Get-Content "$PSScriptRoot/linux-required.json" -Raw | ConvertFrom-Json
foreach ($file in $coverage.PSObject.Properties) {
    $events=@(Get-Content "$EvidenceDirectory/linux/$($file.Name)" | Where-Object { $_ } | ForEach-Object { $_ | ConvertFrom-Json })
    if (@($events | Where-Object Action -in 'fail','skip').Count) { throw 'Linux test failed/skipped' }
    foreach ($name in $file.Value) {
        if (@($events | Where-Object { $_.Test -ceq $name -and $_.Action -ceq 'pass' }).Count -ne 1) { throw "Missing/duplicate case: $name" }
    }
}
@{source_sha=$SourceSHA;exe_sha256=$hash;zip_sha256=(Get-FileHash "$DistributionDirectory.zip").Hash;status='passed';mixed_real_ssh='unconfirmed';windows='V-01..V-18';linux='L01..L07 (layered)';windows_execution=$WindowsExecution;linux_execution=$LinuxExecution;workflow_run_id=$WorkflowRunId} |
    ConvertTo-Json | Set-Content "$EvidenceDirectory/publication.json" -Encoding utf8NoBOM

