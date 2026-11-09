$ErrorActionPreference = 'Stop'
$root = Split-Path (Split-Path $PSScriptRoot)
$temp = Join-Path ([IO.Path]::GetTempPath()) ('wintable-distribution-test-' + [guid]::NewGuid())
function Reject([scriptblock]$Action) {
    $rejected = $false
    try { & $Action | Out-Null } catch { $rejected = $true }
    if (-not $rejected) { throw 'Invalid distribution was accepted' }
}
try {
    $stage = New-Item -ItemType Directory "$temp/stage"
    foreach ($name in @('wintable.exe','README.md','LICENSE','audit.json','docs/usage.md','docs/release.md','docs/command-module.md','docs/file-content-module.md','docs/linux-command.md','docs/linux-file-content.md','examples/linux-inventory.json','examples/linux-file-content.json','examples/linux-file-content-diff.json','examples/inventory.json','examples/tasks.json','licenses/index.json','licenses/Go-LICENSE')) {
        $path = Join-Path $stage $name
        New-Item -ItemType Directory (Split-Path $path) -Force | Out-Null
        Set-Content $path 'synthetic gate test; not a distribution'
    }
    if ((Get-Content "$root/scripts/distribution.ps1" -Raw) -notmatch 'Copy-Item docs/\*\.md') { throw 'Module and support documentation missing from distribution staging' }
    $sha = 'a' * 40
    @(@{name='Go';version='go1.27.3';license='Go-LICENSE'}) | ConvertTo-Json -AsArray | Set-Content "$stage/licenses/index.json"
    @{source_commit=$sha} | ConvertTo-Json | Set-Content "$stage/build.json"
    $rows = @(Get-ChildItem $stage -File -Recurse | ForEach-Object { @{path=[IO.Path]::GetRelativePath($stage.FullName,$_.FullName).Replace('\','/');sha256=(Get-FileHash $_.FullName).Hash} })
    ConvertTo-Json -InputObject $rows | Set-Content "$stage/SHA256.json"
    Compress-Archive "$stage/*" "$temp/test.zip"
    $hash = (Get-FileHash "$temp/test.zip").Hash
    & "$root/scripts/verify-package.ps1" -Zip "$temp/test.zip" -ExpectedSHA256 $hash
    Reject { & "$root/scripts/verify-package.ps1" -Zip "$temp/test.zip" -ExpectedSHA256 ('0' * 64) }
    Set-Content "$temp/log.txt" 'synthetic results'
    $cases = @(1..18 | ForEach-Object { @{id=('V-{0:00}' -f $_);status='passed';log='log.txt';sha256=(Get-FileHash "$temp/log.txt").Hash} })
    $record = @{source_commit=$sha;zip_sha256=$hash;issue11_url='https://github.com/coco-papiyon/wintable/issues/11';reviewer='test';clean_container=$true;no_go_wsl_python=$true;non_admin_install=$true;japanese_space_path=$true;scenarios=$cases}
    function Save-Record { $record | ConvertTo-Json -Depth 8 | Set-Content "$temp/evidence.json" }
    Save-Record
    & "$root/scripts/verify-acceptance.ps1" -Zip "$temp/test.zip" -Evidence "$temp/evidence.json"
    $record.source_commit = 'b' * 40; Save-Record
    Reject { & "$root/scripts/verify-acceptance.ps1" -Zip "$temp/test.zip" -Evidence "$temp/evidence.json" }
    $record.source_commit = $sha; $cases[0].status = 'unconfirmed'; Save-Record
    Reject { & "$root/scripts/verify-acceptance.ps1" -Zip "$temp/test.zip" -Evidence "$temp/evidence.json" }
    $cases[0].status = 'passed'; $record.scenarios = $cases[0..16]; Save-Record
    Reject { & "$root/scripts/verify-acceptance.ps1" -Zip "$temp/test.zip" -Evidence "$temp/evidence.json" }
    $record.scenarios = $cases; Save-Record; Set-Content "$temp/log.txt" 'modified'
    Reject { & "$root/scripts/verify-acceptance.ps1" -Zip "$temp/test.zip" -Evidence "$temp/evidence.json" }
    Set-Content "$stage/extra.txt" 'unexpected'
    Compress-Archive "$stage/*" "$temp/extra.zip"
    Reject { & "$root/scripts/verify-package.ps1" -Zip "$temp/extra.zip" -ExpectedSHA256 (Get-FileHash "$temp/extra.zip").Hash }
    # Missing required file is rejected even with an otherwise matching hash manifest.
    $reduced = @($rows | Where-Object { $_.path -ne 'LICENSE' })
    ConvertTo-Json -InputObject $reduced | Set-Content "$stage/SHA256.json"
    Compress-Archive "$stage/*" "$temp/missing.zip"
    Reject { & "$root/scripts/verify-package.ps1" -Zip "$temp/missing.zip" -ExpectedSHA256 (Get-FileHash "$temp/missing.zip").Hash }
    ConvertTo-Json -InputObject $rows | Set-Content "$stage/SHA256.json"
    Set-Content "$stage/README.md" 'tampered'
    Compress-Archive "$stage/*" "$temp/tampered.zip"
    Reject { & "$root/scripts/verify-package.ps1" -Zip "$temp/tampered.zip" -ExpectedSHA256 (Get-FileHash "$temp/tampered.zip").Hash }
    Write-Output 'PASS: valid package; reject wrong ZIP hash, source mismatch, unconfirmed, missing scenario, altered log, extra file, missing license and altered payload'
} finally {
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Recurse -Force }
}

