#requires -Version 7.3
$ErrorActionPreference = 'Stop'
$root = Join-Path ([IO.Path]::GetTempPath()) ('wintable-summary-' + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory "$root/ssh" | Out-Null
try {
    foreach ($mode in 'valid','missing-log','wrong-binary','wrong-source','null-exit','failed','not-removed','missing-marker','missing-build') {
        $manifest = @{ source_sha='source'; exe_sha256='binary'; acceptance='unconfirmed'; checks=@(@{ name='build'; exit_code=0 }) }
        $controller = @{ source_sha='source'; exe_sha256='binary'; exit_code=0; removed=$true }
        'CLEAN_CONTROLLER_PASS' | Set-Content "$root/ssh/controller.log"
        switch ($mode) {
            'missing-log' { Remove-Item -LiteralPath "$root/ssh/controller.log" }
            'wrong-binary' { $controller.exe_sha256='other' }
            'wrong-source' { $controller.source_sha='other' }
            'null-exit' { $controller.exit_code=$null }
            'failed' { $controller.exit_code=1 }
            'not-removed' { $controller.removed=$false }
            'missing-marker' { 'incomplete' | Set-Content "$root/ssh/controller.log" }
            'missing-build' { $manifest.checks=@() }
        }
        $manifest | ConvertTo-Json -Depth 8 | Set-Content "$root/manifest.json"
        $controller | ConvertTo-Json | Set-Content "$root/ssh/controller.json"
        & "$PSScriptRoot/summarize.ps1" -EvidenceDirectory $root
        if ($LASTEXITCODE -ne 1) { throw 'Partial coverage incorrectly accepted' }
        $rows = @(Import-Csv "$root/scenarios.csv")
        $expected = if ($mode -eq 'valid') { 'passed' } else { 'failed' }
        if ($rows.Count -ne 18 -or $rows[12].status -ne $expected) { throw "Incorrect V-13 verdict for $mode" }
        if (@($rows | Where-Object status -eq 'unconfirmed').Count -ne 17) { throw 'Unverified scenarios were promoted' }
        Write-Output "PASS $mode"
    }
    $coverage = Get-Content "$PSScriptRoot/coverage.json" -Raw | ConvertFrom-Json
    foreach ($mode in 'complete','missing-case','skipped-case','failed-case','duplicate-case','dirty-source','failed-vet','ssh-wrong-source','ssh-missing-exit','ssh-not-removed','native-not-removed','missing-vet-log') {
        $records = @(foreach ($group in $coverage) { foreach ($property in $group.checks.PSObject.Properties) { foreach ($name in $property.Value) { [pscustomobject]@{ log=$property.Name; test=$name } } } })
        foreach ($group in ($records | Group-Object log)) {
            @($group.Group.test | Sort-Object -Unique | ForEach-Object { "--- PASS: $_ (0.01s)" }) | Set-Content "$root/$($group.Name)"
        }
        $manifest = @{ source_sha='source'; exe_sha256='binary'; source_status=@(); native_container_removed=$true; acceptance='unconfirmed'; checks=@() }
        foreach ($name in 'test','vet','build','wrappers','module_binary','native_container') {
            $log = "$name.log"
            if ($name -eq 'native_container') { $log='native-container.log' }
            if (!(Test-Path "$root/$log")) { New-Item -ItemType File "$root/$log" | Out-Null }
            $manifest.checks += @{name=$name;exit_code=0;log=$log}
        }
        $controller = @{ source_sha='source'; exe_sha256='binary'; exit_code=0; removed=$true }
        $ssh = @{source_sha='source';container_removed=$true;image_removed=$true;test_exit_code=0;command_test_exit_code=0;cli_test_exit_code=0}
        'CLEAN_CONTROLLER_PASS' | Set-Content "$root/ssh/controller.log"
        switch ($mode) {
            'missing-case' { (Get-Content "$root/test.log") | Where-Object { $_ -notmatch 'TestSelect ' } | Set-Content "$root/test.log" }
            'skipped-case' { (Get-Content "$root/test.log" -Raw).Replace('PASS: TestSelect ', 'SKIP: TestSelect ') | Set-Content "$root/test.log" }
            'failed-case' { (Get-Content "$root/test.log" -Raw).Replace('PASS: TestSelect ', 'FAIL: TestSelect ') | Set-Content "$root/test.log" }
            'duplicate-case' { '--- PASS: TestSelect (0.01s)' | Add-Content "$root/test.log" }
            'dirty-source' { $manifest.source_status=@(' M changed.go') }
            'failed-vet' { ($manifest.checks | Where-Object name -eq 'vet').exit_code=1 }
            'ssh-wrong-source' { $ssh.source_sha='other' }
            'ssh-missing-exit' { $ssh.cli_test_exit_code=$null }
            'ssh-not-removed' { $ssh.container_removed=$false }
            'native-not-removed' { $manifest.native_container_removed=$false }
            'missing-vet-log' { Remove-Item -LiteralPath "$root/vet.log" }
        }
        $manifest | ConvertTo-Json -Depth 8 | Set-Content "$root/manifest.json"
        $controller | ConvertTo-Json | Set-Content "$root/ssh/controller.json"
        $ssh | ConvertTo-Json | Set-Content "$root/ssh/manifest.json"
        & "$PSScriptRoot/summarize.ps1" -EvidenceDirectory $root
        $actual = Get-Content "$root/manifest.json" -Raw | ConvertFrom-Json
        $expected = if ($mode -eq 'complete') { 'passed' } else { 'unconfirmed' }
        if ($actual.acceptance -ne $expected) { throw "Incorrect acceptance for $mode : $($actual.acceptance)" }
        Write-Output "PASS $mode"
    }
} finally {
    # Only the literal, generated files owned by this test are removed.
    Get-ChildItem -LiteralPath "$root/ssh" -File | Remove-Item -Force
    Get-ChildItem -LiteralPath $root -File | Remove-Item -Force
    Remove-Item -LiteralPath "$root/ssh"
    Remove-Item -LiteralPath $root
}
