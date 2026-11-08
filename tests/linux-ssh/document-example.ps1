param([Parameter(Mandatory)][string]$FixtureDirectory,[Parameter(Mandatory)][string]$EvidenceDirectory)
$ErrorActionPreference='Stop'
$repo=(Resolve-Path "$PSScriptRoot/../..").Path
$package="$repo/dist/package"
& "$repo/tests/ci/check-package.ps1" -Directory $package -SourceSHA (& git -C $repo rev-parse HEAD)
$endpoint=Get-Content "$FixtureDirectory/fixture.json" -Raw | ConvertFrom-Json
$inventory=Get-Content "$package/examples/linux-inventory.json" -Raw | ConvertFrom-Json
$h=$inventory.hosts.linux01
$h.address=$endpoint.Address; $h.port=$endpoint.Port; $h.user=$endpoint.User
$h.private_key_file="$FixtureDirectory/client"; $h.known_hosts_file="$FixtureDirectory/known_hosts"
$inputFile="$FixtureDirectory/example-inventory.json"
try {
    $inventory | ConvertTo-Json -Depth 10 | Set-Content $inputFile -Encoding utf8NoBOM
    $text=& "$package/wintable.exe" run --inventory $inputFile --tasks "$package/examples/linux-command.json" --format json --include-output
    if ($LASTEXITCODE -ne 0) { throw 'Document example failed' }
    $result=($text -join "`n") | ConvertFrom-Json
    # Compare only fixed synthetic whoami output; never persist arbitrary output.
    $tasks=@($result.hosts | ForEach-Object { $_.tasks })
    if ($tasks.Count -ne 1 -or $tasks[0].status -ne 'changed' -or $tasks[0].exit_code -ne 0 -or
        [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($tasks[0].stdout_base64)) -cne "runner`n") { throw 'Document example result mismatch' }
    @{source_sha=(& git -C $repo rev-parse HEAD);status='passed';expected='runner LF';exit_code=0;exe_sha256=(Get-FileHash "$package/wintable.exe").Hash} |
        ConvertTo-Json | Set-Content "$EvidenceDirectory/document-example.json" -Encoding utf8NoBOM
} finally {
    if (Test-Path $inputFile) { Remove-Item -LiteralPath $inputFile }
}

