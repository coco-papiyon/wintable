#requires -Version 7.3
[CmdletBinding()]
param([string]$Revision = 'HEAD', [string]$OutputDirectory)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path "$PSScriptRoot/../..").Path
if (!$OutputDirectory) { $OutputDirectory = "$repo/.cache/git-evidence/$([Guid]::NewGuid().ToString('N'))" }
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'OutputDirectory must be new' }
New-Item -ItemType Directory -Path "$output/source" -Force | Out-Null
$sha = & git -C $repo rev-parse "$($Revision)^{commit}"
if ($LASTEXITCODE) { throw 'Could not resolve source revision' }
& git -C $repo archive --format=tar "--output=$output/source.tar" $sha
if ($LASTEXITCODE) { throw 'git archive failed' }
& tar -xf "$output/source.tar" -C "$output/source"
if ($LASTEXITCODE) { throw 'Archive extraction failed' }
$source = "$output/source"
$evidence = "$source/docs/validation/evidence/20261108-complete"
# Verify the immutable input before the summarizer writes any derived output.
$verified = 0
foreach ($row in Import-Csv "$evidence/sha256.csv") {
    $path = [IO.Path]::GetFullPath((Join-Path $evidence $row.path))
    if (![IO.Path]::GetRelativePath($evidence, $path).StartsWith('..') -and
        (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash -eq $row.sha256) {
        $verified++
    } else { throw "Saved evidence hash mismatch: $($row.path)" }
}
$pwsh = (Get-Command pwsh -CommandType Application).Source
& $pwsh -NoProfile -File "$source/tests/validation/test-summary.ps1" 2>&1 | Tee-Object "$output/tests.log" | Out-Host
$testExit = $LASTEXITCODE
& $pwsh -NoProfile -File "$source/tests/validation/summarize.ps1" -EvidenceDirectory "$evidence/final" 2>&1 | Tee-Object "$output/summary.log" | Out-Host
$summaryExit = $LASTEXITCODE
$manifest = Get-Content "$evidence/final/manifest.json" -Raw | ConvertFrom-Json
$scenarios = @(Import-Csv "$evidence/final/scenarios.csv")
$record = [ordered]@{
    source_sha=$sha; verified_input_files=$verified; test_exit_code=$testExit; summary_exit_code=$summaryExit
    source_coverage_sha256=(Get-FileHash "$source/tests/validation/coverage.json" -Algorithm SHA256).Hash
    evidence_coverage_sha256=(Get-FileHash "$evidence/final/coverage.json" -Algorithm SHA256).Hash
    acceptance=$manifest.acceptance; prerequisites_passed=$manifest.prerequisites_passed
    scenario_count=$scenarios.Count; passed_count=@($scenarios | Where-Object status -eq 'passed').Count
    finished_utc=[DateTime]::UtcNow.ToString('o')
}
$record | ConvertTo-Json -Depth 5 | Set-Content "$output/result.json" -Encoding utf8NoBOM
if ($testExit -ne 0 -or $summaryExit -ne 0 -or $manifest.acceptance -ne 'passed' -or
    $manifest.prerequisites_passed -ne $true -or $scenarios.Count -ne 18 -or $record.passed_count -ne 18) {
    throw "Git-acquired evidence did not reproduce acceptance; see $output/result.json"
}
Write-Output "PASS git archive $sha : 18 scenarios passed; $verified input hashes verified"
exit 0
