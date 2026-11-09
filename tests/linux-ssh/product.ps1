param([Parameter(Mandatory)][string]$FixtureDirectory,[Parameter(Mandatory)][string]$EvidenceDirectory,[string]$DistributionDirectory)
$ErrorActionPreference='Stop'
$repo=(Resolve-Path "$PSScriptRoot/../..").Path
Push-Location $repo
$stage='product-tests'
try {
    $env:WINTABLE_LINUX_SSH_FIXTURE=$FixtureDirectory
    $env:WINTABLE_PRODUCT_EXE=if ($DistributionDirectory) { Join-Path $DistributionDirectory 'wintable.exe' } else { Join-Path $repo 'dist/wintable.exe' }
    if (!(Test-Path $env:WINTABLE_PRODUCT_EXE)) { throw 'Build product before acceptance' }
    $events = & go test ./internal/module -run '^TestLinux(CommandSSH|FileContentSSH|FetchSSH)$' -count=1 -json
    $code=$LASTEXITCODE
    $events | Set-Content "$EvidenceDirectory/product.jsonl" -Encoding utf8NoBOM
    $parsed=@($events | ForEach-Object { $_ | ConvertFrom-Json })
    if ($code -or @($parsed | Where-Object Action -eq 'skip').Count -or !@($parsed | Where-Object { $_.Action -eq 'pass' -and $_.Test -eq 'TestLinuxCommandSSH' }).Count) { throw 'Product acceptance failed or skipped' }
    $required=@('raw-invalid','raw-boundaries','raw-EOF','raw-runtime','raw-E2BIG','product-argv','product-output','product-exits','receiver-interruption','receiver-interruption/timeout','receiver-interruption/cancel','receiver-interruption/disconnect','product-E2BIG-fixture')
    foreach ($case in $required) {
        if (@($parsed | Where-Object { $_.Action -eq 'pass' -and $_.Test -ceq "TestLinuxCommandSSH/$case" }).Count -ne 1) { throw "Missing/duplicate product case: $case" }
    }
    & "$PSScriptRoot/check-file-content.ps1" -Events $parsed
    foreach ($case in @('TestLinuxFetchSSH','TestLinuxFetchSSH/empty','TestLinuxFetchSSH/text','TestLinuxFetchSSH/binary-over-1MiB','TestLinuxFetchSSH/failure-preserves-existing','TestLinuxFetchSSH/product-parallel-hosts')) {
        if (@($parsed | Where-Object { $_.Action -eq 'pass' -and $_.Test -ceq $case }).Count -ne 1) { throw "Missing/duplicate fetch case: $case" }
    }
    # Normal implementation checks use the standalone dist/wintable.exe.
    # Packaged documentation examples remain mandatory for distribution checks.
    if ($DistributionDirectory) {
        $stage='document-example'
        & "$PSScriptRoot/document-example.ps1" -FixtureDirectory $FixtureDirectory -EvidenceDirectory $EvidenceDirectory -DistributionDirectory $DistributionDirectory
    }
    Get-FileHash $env:WINTABLE_PRODUCT_EXE | Select-Object Algorithm,Hash | ConvertTo-Json | Set-Content "$EvidenceDirectory/product-hash.json"
} catch {
    # Persist only location metadata, never exception text or command output.
    @{stage=$stage;script=[IO.Path]::GetFileName($_.InvocationInfo.ScriptName);line=$_.InvocationInfo.ScriptLineNumber} |
        ConvertTo-Json | Set-Content "$EvidenceDirectory/product-failure.json" -Encoding utf8NoBOM
    throw
} finally {
    Remove-Item Env:WINTABLE_LINUX_SSH_FIXTURE -ErrorAction SilentlyContinue
    Remove-Item Env:WINTABLE_PRODUCT_EXE -ErrorAction SilentlyContinue
    Pop-Location
}

