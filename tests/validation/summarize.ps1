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
$tests = @(foreach ($relative in 'test.log','wrappers.log','ssh/command-test.log','ssh/test.log') {
    if (!(Test-Path -LiteralPath "$root/$relative")) { continue }
    foreach ($line in Get-Content "$root/$relative") {
        if ($line -match '^\s*--- (PASS|FAIL|SKIP): (\S+)') {
            [pscustomobject]@{ test=$Matches[2]; status=$Matches[1]; evidence=$relative }
        }
    }
})
$tests | Export-Csv "$root/test-cases.csv" -NoTypeInformation -Encoding utf8NoBOM
$manifest.acceptance = if (@($rows | Where-Object status -ne 'passed').Count) { 'unconfirmed' } else { 'passed' }
$manifest | ConvertTo-Json -Depth 10 | Set-Content "$root/manifest.json" -Encoding utf8NoBOM
Get-ChildItem $root -Recurse -File | Where-Object { $_.Name -ne 'sha256.csv' -and $_.Extension -ne '.exe' } |
    ForEach-Object { [pscustomobject]@{ path=[IO.Path]::GetRelativePath($root,$_.FullName); sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash } } |
    Export-Csv "$root/sha256.csv" -NoTypeInformation -Encoding utf8NoBOM
if ($manifest.acceptance -ne 'passed') { exit 1 }
