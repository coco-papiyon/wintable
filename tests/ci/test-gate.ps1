$ErrorActionPreference='Stop'
# Synthetic evidence tests the gate only. It never certifies real SSH acceptance.
$root=Join-Path ([IO.Path]::GetTempPath()) ('wintable-gate-'+[Guid]::NewGuid().ToString('N'))
New-Item "$root/package","$root/windows/ssh","$root/linux" -ItemType Directory -Force | Out-Null
function Save($object,$path) { $object | ConvertTo-Json -Depth 15 | Set-Content "$root/$path" -Encoding utf8NoBOM }
try {
    'synthetic executable bytes' | Set-Content "$root/package/wintable.exe"
    $hash=(Get-FileHash "$root/package/wintable.exe").Hash
    'synthetic ZIP bytes' | Set-Content "$root/package.zip"
    $zipHash=(Get-FileHash "$root/package.zip").Hash
    $paths=@('wintable.exe','README.md','LICENSE','audit.json','build.json','SHA256.json','licenses/index.json','licenses/Go-LICENSE','docs/usage.md','docs/fetch-module.md','examples/fetch.json','docs/release.md','docs/design/modules.md','docs/design/modules/command-module.md','docs/design/modules/file-content-module.md','docs/design/windows-ansible.md','docs/design/windows-ansible-validation.md','docs/adr/0001-linux-target-support.md','docs/linux-file-content.md','docs/linux-command.md','docs/support.md','docs/ci.md','examples/linux-inventory.json','examples/linux-file-content.json','examples/linux-file-content-diff.json','examples/linux-command.json','examples/mixed-inventory.json','examples/mixed-command.json')
    foreach ($path in $paths | Select-Object -Skip 1) {
        New-Item (Split-Path "$root/package/$path") -ItemType Directory -Force | Out-Null
        'synthetic' | Set-Content "$root/package/$path"
    }
    Save @{source_sha='source';go='go version go1.27.3 windows/amd64';files=@($paths | ForEach-Object {@{path=$_;sha256=(Get-FileHash "$root/package/$_").Hash}})} 'package/distribution.json'
    foreach ($mode in 'fetch-linux-missing','fetch-windows-missing','fetch-native-skip','l07-attribute-skip','l07-race-missing','l07-duplicate','l07-empty','example-missing','valid','valid-hybrid','missing-provenance','skip','missing','duplicate','null-exit','wrong-source','wrong-binary','versions','cleanup','deadline','job-failed','job-skipped','job-cancelled','missing-artifact','windows-missing','zip-missing','zip-failed','zip-not-removed','zip-wrong-hash') {
        $wc=Get-Content "$PSScriptRoot/../validation/coverage.json" -Raw | ConvertFrom-Json
        foreach ($file in @($wc | ForEach-Object {$_.checks.PSObject.Properties.Name}) | Sort-Object -Unique) {
            $names=@($wc | ForEach-Object {$_.checks.PSObject.Properties | Where-Object Name -ceq $file | ForEach-Object Value}) | Sort-Object -Unique
            $names | ForEach-Object { "--- PASS: $_ (0.01s)" } | Set-Content "$root/windows/$file"
        }
        Copy-Item "$PSScriptRoot/../validation/coverage.json" "$root/windows/coverage.json"
        $checks=@(foreach ($name in 'test','vet','build','wrappers','module_binary','native_container') {
            $file=if ($name -eq 'native_container') {'native-container.log'} else {"$name.log"}
            if (!(Test-Path "$root/windows/$file")) { '' | Set-Content "$root/windows/$file" }
            @{name=$name;exit_code=0;log=$file}
        })
        $w=@{source_sha='source';exe_sha256=$hash;acceptance='passed';prerequisites_passed=$true;checks=$checks;source_status=@();native_container_removed=$true;coverage_sha256=(Get-FileHash "$root/windows/coverage.json").Hash}
        $ws=@{source_sha='source';source_status=@();container_removed=$true;image_removed=$true;test_exit_code=0;command_test_exit_code=0;cli_test_exit_code=0}
        $controller=@{source_sha='source';exe_sha256=$hash;exit_code=0;removed=$true;zip_exit_code=0;zip_container_removed=$true;zip_sha256=$zipHash}
        'ZIP_CONTROLLER_PASS' | Set-Content "$root/windows/ssh/zip-controller.log"
        'CLEAN_CONTROLLER_PASS' | Set-Content "$root/windows/ssh/controller.log"
        foreach ($name in 'TestHostOS','TestOSSchemaBoundaries','TestOSPreflightNoConnections','TestMixedBackends','TestLinuxBoundaries','TestLinuxFixedBackend','TestLinuxDecoder') { "--- PASS: $name (0.01s)" | Add-Content "$root/windows/test.log" }
        $l=@{source_sha='source';go='go version go1.27.3 windows/amd64';source_status=@();lock_sha256=(Get-FileHash "$PSScriptRoot/../linux-ssh/fixture.lock.json").Hash;status='passed';cleanup_ok=$true;product=@{exit_code=0};started_utc='2026-11-08T00:00:00Z';finished_utc='2026-11-08T00:02:00Z';cleanup=@('container','network','image','credentials' | ForEach-Object {@{kind=$_;remaining=0}})}
        $e=@{source_sha='source';exe_sha256=$hash;status='passed';exit_code=0}
        $v=@{match=$true}
        $coverage=Get-Content "$PSScriptRoot/linux-required.json" -Raw | ConvertFrom-Json
        foreach ($f in $coverage.PSObject.Properties) {
            $f.Value | ForEach-Object { @{Action='pass';Test=$_} | ConvertTo-Json -Compress } | Set-Content "$root/linux/$($f.Name)"
        }
        $job='success'
        switch ($mode) {
            'fetch-linux-missing' { Get-Content "$root/linux/product.jsonl" | Where-Object {$_ -notmatch 'TestLinuxFetchSSH'} | Set-Content "$root/linux/changed"; Move-Item "$root/linux/changed" "$root/linux/product.jsonl" -Force }
            'fetch-windows-missing' { Get-Content "$root/windows/ssh/command-test.log" | Where-Object {$_ -notmatch 'TestFetchSSH'} | Set-Content "$root/windows/ssh/changed"; Move-Item "$root/windows/ssh/changed" "$root/windows/ssh/command-test.log" -Force }
            'fetch-native-skip' { '--- SKIP: TestStoreCommitFailuresAndRace/symlink-swap (0.01s)' | Add-Content "$root/windows/ssh/fetch-native.log" }
            'l07-attribute-skip' { '{"Action":"skip","Test":"TestLinuxFileContentSSH/API-cases/foreign-owner"}' | Add-Content "$root/linux/product.jsonl" }
            'l07-race-missing' { Get-Content "$root/linux/product.jsonl" | Where-Object {$_ -notmatch 'API-cases/ancestor-rename'} | Set-Content "$root/linux/changed"; Move-Item "$root/linux/changed" "$root/linux/product.jsonl" -Force }
            'l07-duplicate' { '{"Action":"pass","Test":"TestLinuxFileContentSSH/API-cases/new-destination-race"}' | Add-Content "$root/linux/product.jsonl" }
            'l07-empty' { '' | Set-Content "$root/linux/product.jsonl" }
            'example-missing' { Get-Content "$root/linux/product.jsonl" | Where-Object {$_ -notmatch 'distribution-example'} | Set-Content "$root/linux/changed"; Move-Item "$root/linux/changed" "$root/linux/product.jsonl" -Force }

            'skip' { '{"Action":"skip","Test":"TestLinuxCommandSSH/product-output"}' | Add-Content "$root/linux/product.jsonl" }
            'missing' { Get-Content "$root/linux/product.jsonl" | Where-Object {$_ -notmatch 'product-output'} | Set-Content "$root/linux/changed"; Move-Item "$root/linux/changed" "$root/linux/product.jsonl" -Force }
            'duplicate' { '{"Action":"pass","Test":"TestLinuxCommandSSH/product-output"}' | Add-Content "$root/linux/product.jsonl" }
            'null-exit' {$l.product.exit_code=$null}
            'wrong-source' {$l.source_sha='other'}
            'wrong-binary' {$e.exe_sha256='other'}
            'versions' {$v.match=$false}
            'cleanup' {$l.cleanup[0].remaining=1}
            'deadline' {$l.finished_utc='2026-11-08T01:00:00Z'}
            'job-failed' {$job='failure'}
            'job-skipped' {$job='skipped'}
            'job-cancelled' {$job='cancelled'}
            'windows-missing' { '--- PASS: parent (0.01s)' | Set-Content "$root/windows/ssh/test.log" }
            'zip-missing' { $controller.zip_exit_code=$null }
            'zip-failed' { $controller.zip_exit_code=1 }
            'zip-not-removed' { $controller.zip_container_removed=$false }
            'zip-wrong-hash' { $controller.zip_sha256=('0'*64) }
        }
        Save $w 'windows/manifest.json'; Save $ws 'windows/ssh/manifest.json'
        Save $controller 'windows/ssh/controller.json'
        Save $l 'linux/manifest.json'; Save $e 'linux/document-example.json'; Save $v 'linux/version-comparison.json'
        Save @{Hash=$hash} 'linux/product-hash.json'
        if ($mode -eq 'missing-artifact') { Remove-Item -LiteralPath "$root/linux/product-hash.json" }
        $provenance=@()
        if ($mode -in 'valid-hybrid','missing-provenance') { $provenance=@('-WindowsExecution','github-actions') }
        if ($mode -eq 'valid-hybrid') { $provenance+=@('-WorkflowRunId','123') }
        $output=& pwsh -NoProfile -File "$PSScriptRoot/gate.ps1" -EvidenceDirectory $root -DistributionDirectory "$root/package" -SourceSHA source -WindowsJob success -LinuxJob $job @provenance 2>&1
        if (($LASTEXITCODE -eq 0) -ne ($mode -in 'valid','valid-hybrid')) { throw "Incorrect gate verdict for ${mode}: $output" }
        Write-Host "PASS gate: $mode"
    }
} finally {
    # Only this script's generated GUID directory under the system temp root.
    $full=[IO.Path]::GetFullPath($root)
    if (!$full.StartsWith([IO.Path]::GetFullPath([IO.Path]::GetTempPath()),[StringComparison]::OrdinalIgnoreCase) -or (Split-Path $full -Leaf) -notlike 'wintable-gate-*') { throw 'Unsafe cleanup target' }
    Remove-Item -LiteralPath $full -Recurse -Force
}
