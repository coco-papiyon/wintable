#requires -Version 7.3
[CmdletBinding()]
param([Parameter(Mandatory)][string]$EvidenceDirectory)
$ErrorActionPreference = 'Stop'
$root = [IO.Path]::GetFullPath($EvidenceDirectory)
$manifest = Get-Content "$root/manifest.json" -Raw | ConvertFrom-Json
$rows = @(1..18 | ForEach-Object {
    [pscustomobject]@{ scenario=('V-{0:00}' -f $_); status='unconfirmed'; evidence='manifest.json; ssh/manifest.json'; note='Subcase coverage review still required' }
})
# V-13 is an actual binary/SSH check, independently of Go fixture test success.
$controllerFile = "$root/ssh/controller.json"
if (Test-Path -LiteralPath $controllerFile) {
    $controller = Get-Content $controllerFile -Raw | ConvertFrom-Json
    $row = $rows[12]
    $row.evidence = 'ssh/controller.json; ssh/controller.log; ssh/controller/cases.ps1; ssh/controller/tasks.json'
    $row.status = 'failed'
    $row.note = 'Controller failed, incomplete, or inconsistent with build manifest'
    $log = "$root/ssh/controller.log"
    $sameBinary = $controller.exe_sha256 -and ($controller.exe_sha256 -eq $manifest.exe_sha256)
    $sameSource = $controller.source_sha -eq $manifest.source_sha
    $build = @($manifest.checks | Where-Object { $_.name -eq 'build' -and $_.exit_code -eq 0 })
    if ($controller.exit_code -eq 0 -and $null -ne $controller.exit_code -and $controller.removed -eq $true -and
        $sameBinary -and $sameSource -and $build.Count -eq 1 -and (Test-Path -LiteralPath $log) -and
        (Select-String -LiteralPath $log -SimpleMatch 'CLEAN_CONTROLLER_PASS' -Quiet)) {
        $row.status = 'passed'
        $row.note = 'Clean amd64 controller: absent Go/WSL/Python; command and file_content over SSH; repeat is unchanged'
    }
}
$rows | Export-Csv "$root/scenarios.csv" -NoTypeInformation -Encoding utf8NoBOM
# Preserve individual PASS/FAIL/SKIP records: a parent PASS never erases SKIP.
$tests = @(foreach ($relative in 'test.log','wrappers.log','native-container.log','ssh/cli-test.log','ssh/command-test.log','ssh/test.log') {
    if (!(Test-Path -LiteralPath "$root/$relative")) { continue }
    foreach ($line in Get-Content "$root/$relative") {
        if ($line -match '^\s*--- (PASS|FAIL|SKIP): (\S+)') {
            [pscustomobject]@{ test=$Matches[2]; status=$Matches[1]; evidence=$relative }
        }
    }
})
$tests | Export-Csv "$root/test-cases.csv" -NoTypeInformation -Encoding utf8NoBOM
$coverage = Get-Content "$PSScriptRoot/coverage.json" -Raw | ConvertFrom-Json
$details = @(foreach ($group in $coverage) {
    foreach ($property in $group.checks.PSObject.Properties) {
        foreach ($name in $property.Value) {
            $records = @($tests | Where-Object { $_.test -ceq $name -and $_.evidence -ceq $property.Name })
            $status = if ($records.Count -eq 1 -and $records[0].status -ceq 'PASS') { 'passed' } elseif (@($records | Where-Object status -eq 'FAIL').Count) { 'failed' } else { 'unconfirmed' }
            [pscustomobject]@{ scenario=$group.scenario; test=$name; status=$status; evidence=$property.Name; scope=$group.scope }
        }
    }
})
# Check every test record, not just a parent PASS. A missing/SKIP record remains unconfirmed.
foreach ($row in $rows | Where-Object scenario -ne 'V-13') {
    $cases = @($details | Where-Object scenario -eq $row.scenario)
    if ($cases.Count -and !@($cases | Where-Object status -ne 'passed').Count) { $row.status='passed'; $row.note=($coverage | Where-Object scenario -eq $row.scenario).scope }
    elseif (@($cases | Where-Object status -eq 'failed').Count) { $row.status='failed' }
    $row.evidence = 'coverage.csv; ' + (($cases.evidence | Sort-Object -Unique) -join '; ')
}
$details | Export-Csv "$root/coverage.csv" -NoTypeInformation -Encoding utf8NoBOM
$rows | Export-Csv "$root/scenarios.csv" -NoTypeInformation -Encoding utf8NoBOM
$prerequisites = $true
# Keep the recorded evidence's raw-byte hash mandatory. Git may convert the
# source copy between LF and CRLF, so compare source/evidence text separately.
# Normalize CRLF to LF only: do not ignore whitespace, BOMs, case or content.
function Read-CoverageText([string]$Path) {
    $utf8 = [Text.UTF8Encoding]::new($false, $true)
    return $utf8.GetString([IO.File]::ReadAllBytes($Path)).Replace("`r`n", "`n")
}
if (!(Test-Path -LiteralPath "$root/coverage.json") -or
    (Get-FileHash "$root/coverage.json" -Algorithm SHA256).Hash -ne $manifest.coverage_sha256) {
    $prerequisites=$false
} elseif (![string]::Equals((Read-CoverageText "$root/coverage.json"),
    (Read-CoverageText "$PSScriptRoot/coverage.json"), [StringComparison]::Ordinal)) {
    $prerequisites=$false
}
foreach ($name in 'test','vet','build','wrappers','module_binary','native_container') {
    $check = @($manifest.checks | Where-Object name -eq $name)
    if ($check.Count -ne 1 -or $null -eq $check[0].exit_code -or $check[0].exit_code -ne 0 -or !(Test-Path -LiteralPath "$root/$($check[0].log)")) { $prerequisites=$false }
}
if (!(Test-Path "$root/ssh/manifest.json")) { $prerequisites=$false } else {
    $ssh = Get-Content "$root/ssh/manifest.json" -Raw | ConvertFrom-Json
    if ($ssh.source_sha -ne $manifest.source_sha -or @($ssh.source_status).Count -or $ssh.failure -or $ssh.container_removed -ne $true -or $ssh.image_removed -ne $true) { $prerequisites=$false }
    foreach ($name in 'test_exit_code','command_test_exit_code','cli_test_exit_code') { if ($null -eq $ssh.$name -or $ssh.$name -ne 0) { $prerequisites=$false } }
}
if ($manifest.failure -or $manifest.native_container_removed -ne $true -or @($manifest.source_status).Count -or @($tests | Where-Object status -eq 'FAIL').Count) { $prerequisites=$false }
$manifest.acceptance = if (@($rows | Where-Object status -ne 'passed').Count) { 'unconfirmed' } else { 'passed' }
if (!$prerequisites) { $manifest.acceptance='unconfirmed' }
$manifest | Add-Member -NotePropertyName prerequisites_passed -NotePropertyValue $prerequisites -Force
$manifest | ConvertTo-Json -Depth 10 | Set-Content "$root/manifest.json" -Encoding utf8NoBOM
Get-ChildItem $root -Recurse -File | Where-Object { $_.Name -ne 'sha256.csv' -and $_.Extension -ne '.exe' } |
    ForEach-Object { [pscustomobject]@{ path=[IO.Path]::GetRelativePath($root,$_.FullName); sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash } } |
    Export-Csv "$root/sha256.csv" -NoTypeInformation -Encoding utf8NoBOM
if ($manifest.acceptance -ne 'passed') { exit 1 }
exit 0
