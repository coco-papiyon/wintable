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
} finally {
    # Only the literal, generated files owned by this test are removed.
    Get-ChildItem -LiteralPath "$root/ssh" -File | Remove-Item -Force
    Get-ChildItem -LiteralPath $root -File | Remove-Item -Force
    Remove-Item -LiteralPath "$root/ssh"
    Remove-Item -LiteralPath $root
}
