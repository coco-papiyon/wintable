$ErrorActionPreference='Stop'
# Synthetic evidence tests the gate only. It never certifies real SSH acceptance.
$root=Join-Path ([IO.Path]::GetTempPath()) ('wintable-gate-'+[Guid]::NewGuid().ToString('N'))
New-Item "$root/package","$root/windows/ssh","$root/linux" -ItemType Directory -Force | Out-Null
function Save($object,$path) { $object | ConvertTo-Json -Depth 15 | Set-Content "$root/$path" -Encoding utf8NoBOM }
try {
    'synthetic executable bytes' | Set-Content "$root/package/wintable.exe"
    $hash=(Get-FileHash "$root/package/wintable.exe").Hash
    $paths=@('wintable.exe','README.md','LICENSE','audit.json','build.json','SHA256.json','licenses/index.json','licenses/Go-LICENSE','docs/usage.md','docs/release.md','docs/file-content-module.md','docs/linux-command.md','docs/support.md','docs/ci.md','examples/linux-inventory.json','examples/linux-command.json','examples/mixed-inventory.json','examples/mixed-command.json')
    foreach ($path in $paths | Select-Object -Skip 1) {
        New-Item (Split-Path "$root/package/$path") -ItemType Directory -Force | Out-Null
        'synthetic' | Set-Content "$root/package/$path"
    }
    Save @{source_sha='source';go='go version go1.27.3 windows/amd64';files=@($paths | ForEach-Object {@{path=$_;sha256=(Get-FileHash "$root/package/$_").Hash}})} 'package/distribution.json'
    foreach ($mode in 'valid','skip','missing','duplicate','null-exit','wrong-source','wrong-binary','versions','cleanup','deadline','job-failed','job-skipped','job-cancelled','missing-artifact','windows-missing') {
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
        Save @{source_sha='source';exe_sha256=$hash;exit_code=0;removed=$true} 'windows/ssh/controller.json'
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
        }
        Save $w 'windows/manifest.json'; Save $ws 'windows/ssh/manifest.json'
        Save $l 'linux/manifest.json'; Save $e 'linux/document-example.json'; Save $v 'linux/version-comparison.json'
        Save @{Hash=$hash} 'linux/product-hash.json'
        if ($mode -eq 'missing-artifact') { Remove-Item -LiteralPath "$root/linux/product-hash.json" }
        $output=& pwsh -NoProfile -File "$PSScriptRoot/gate.ps1" -EvidenceDirectory $root -DistributionDirectory "$root/package" -SourceSHA source -WindowsJob success -LinuxJob $job 2>&1
        if (($LASTEXITCODE -eq 0) -ne ($mode -eq 'valid')) { throw "Incorrect gate verdict for ${mode}: $output" }
        Write-Host "PASS gate: $mode"
    }
} finally {
    # Only this script's generated GUID directory under the system temp root.
    $full=[IO.Path]::GetFullPath($root)
    if (!$full.StartsWith([IO.Path]::GetFullPath([IO.Path]::GetTempPath()),[StringComparison]::OrdinalIgnoreCase) -or (Split-Path $full -Leaf) -notlike 'wintable-gate-*') { throw 'Unsafe cleanup target' }
    Remove-Item -LiteralPath $full -Recurse -Force
}
