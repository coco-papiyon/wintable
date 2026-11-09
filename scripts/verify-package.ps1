param([Parameter(Mandatory)][string]$Zip, [Parameter(Mandatory)][string]$ExpectedSHA256)
$ErrorActionPreference = 'Stop'
if ($ExpectedSHA256 -notmatch '^[a-fA-F0-9]{64}$' -or (Get-FileHash -LiteralPath $Zip -Algorithm SHA256).Hash -ne $ExpectedSHA256) { throw 'ZIP hash mismatch' }
$temp = Join-Path ([IO.Path]::GetTempPath()) ('wintable-verify-' + [guid]::NewGuid())
try {
    # Reject traversal and duplicate entries before extracting.
    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $archive = [IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $Zip))
    try {
        $names = @{}
        foreach ($entry in $archive.Entries) {
            $name = $entry.FullName.Replace('\','/')
            if ($name -match '(^/|:|(^|/)\.\.(/|$))' -or $names.ContainsKey($name)) { throw 'Unsafe ZIP entry' }
            $names[$name] = $true
        }
    } finally { $archive.Dispose() }
    Expand-Archive -LiteralPath $Zip -DestinationPath $temp
    $rows = @(Get-Content "$temp/SHA256.json" -Raw | ConvertFrom-Json)
    $seen = @{}
    foreach ($row in $rows) {
        if ($row.path -match '(^/|\\|:|(^|/)\.\.(/|$))' -or $seen.ContainsKey($row.path) -or $row.path -eq 'SHA256.json') { throw 'Invalid hash manifest path' }
        $seen[$row.path] = $true
        if ($row.sha256 -notmatch '^[a-fA-F0-9]{64}$' -or (Get-FileHash -LiteralPath (Join-Path $temp $row.path) -Algorithm SHA256).Hash -ne $row.sha256) { throw "File hash mismatch: $($row.path)" }
    }
    foreach ($required in @('wintable.exe','README.md','LICENSE','build.json','audit.json','docs/usage.md','docs/fetch-module.md','docs/release.md','docs/design/modules.md','docs/design/modules/command-module.md','docs/design/modules/file-content-module.md','docs/design/windows-ansible.md','docs/design/windows-ansible-validation.md','docs/adr/0001-linux-target-support.md','docs/linux-command.md','docs/linux-file-content.md','examples/fetch.json','examples/linux-inventory.json','examples/linux-file-content.json','examples/linux-file-content-diff.json','examples/inventory.json','examples/tasks.json','licenses/index.json','licenses/Go-LICENSE')) {
        if (-not $seen.ContainsKey($required)) { throw "Missing required file: $required" }
    }
    $licenses = @(Get-Content "$temp/licenses/index.json" -Raw | ConvertFrom-Json)
    if ($licenses.Count -lt 1 -or @($licenses | Where-Object { $_.name -eq 'Go' -and $_.version -eq 'go1.27.3' }).Count -ne 1) { throw 'Missing Go license inventory' }
    foreach ($license in $licenses) {
        if (-not $license.name -or -not $license.version -or $license.license -match '[/\\:]|^\.' -or -not $seen.ContainsKey("licenses/$($license.license)")) { throw 'Missing dependency license' }
    }
    if (@(Get-ChildItem $temp -File -Recurse).Count -ne $rows.Count + 1) { throw 'Unexpected files in ZIP' }
    Write-Output 'Package hashes and contents verified'
} finally {
    # Only this freshly allocated temporary directory is removed.
    if (Test-Path -LiteralPath $temp) { Remove-Item -LiteralPath $temp -Recurse -Force }
}

