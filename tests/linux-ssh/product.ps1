param([Parameter(Mandatory)][string]$FixtureDirectory,[Parameter(Mandatory)][string]$EvidenceDirectory)
$ErrorActionPreference='Stop'
$repo=(Resolve-Path "$PSScriptRoot/../..").Path
Push-Location $repo
try {
    $env:WINTABLE_LINUX_SSH_FIXTURE=$FixtureDirectory
    $env:WINTABLE_PRODUCT_EXE=Join-Path $repo 'dist/wintable.exe'
    if (!(Test-Path $env:WINTABLE_PRODUCT_EXE)) { throw 'Build product before acceptance' }
    $events = & go test ./internal/module -run '^TestLinuxCommandSSH$' -count=1 -json
    $code=$LASTEXITCODE
    $events | Set-Content "$EvidenceDirectory/product.jsonl" -Encoding utf8NoBOM
    $parsed=@($events | ForEach-Object { $_ | ConvertFrom-Json })
    if ($code -or @($parsed | Where-Object Action -eq 'skip').Count -or !@($parsed | Where-Object { $_.Action -eq 'pass' -and $_.Test -eq 'TestLinuxCommandSSH' }).Count) { throw 'Product acceptance failed or skipped' }
    $required=@('raw-invalid','raw-boundaries','raw-EOF','raw-runtime','raw-E2BIG','product-argv','product-output','product-exits','receiver-interruption','receiver-interruption/timeout','receiver-interruption/cancel','receiver-interruption/disconnect','product-E2BIG-fixture')
    foreach ($case in $required) {
        if (@($parsed | Where-Object { $_.Action -eq 'pass' -and $_.Test -ceq "TestLinuxCommandSSH/$case" }).Count -ne 1) { throw "Missing/duplicate product case: $case" }
    }
    & "$PSScriptRoot/document-example.ps1" -FixtureDirectory $FixtureDirectory -EvidenceDirectory $EvidenceDirectory
    Get-FileHash $env:WINTABLE_PRODUCT_EXE | Select-Object Algorithm,Hash | ConvertTo-Json | Set-Content "$EvidenceDirectory/product-hash.json"
} finally {
    Remove-Item Env:WINTABLE_LINUX_SSH_FIXTURE -ErrorAction SilentlyContinue
    Remove-Item Env:WINTABLE_PRODUCT_EXE -ErrorAction SilentlyContinue
    Pop-Location
}

