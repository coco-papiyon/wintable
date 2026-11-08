param([string]$OutputDirectory = 'dist', [string]$ProductLicense = 'LICENSE')
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
function Invoke-Go {
    param([string[]]$Arguments)
    & go @Arguments
    if ($LASTEXITCODE -ne 0) { throw "go failed: $Arguments" }
}
Push-Location (Split-Path $PSScriptRoot)
try {
    $env:GOTOOLCHAIN = 'go1.27.3'
    $env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
    if ((& go version) -notmatch '^go version go1\.27\.3 windows/amd64$') { throw 'Go version/platform mismatch' }
    $sha = (& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE) { throw 'Cannot identify source' }
    $status = & git status --porcelain --untracked-files=all
    if ($LASTEXITCODE -or $status) { throw 'Source must be clean, including untracked files' }
    $out = [IO.Path]::GetFullPath($OutputDirectory)
    if (Test-Path $out) { throw 'Output directory must not exist; use a fresh directory' }
    New-Item -ItemType Directory -Path $out | Out-Null
    Invoke-Go -Arguments @('mod','verify')
    Invoke-Go -Arguments @('test','./...','-count=1')
    Invoke-Go -Arguments @('vet','./...')
    Invoke-Go -Arguments @('test','./internal/module','-run','^TestDistributionWrapper$','-count=1','-v')
    # JSON mode exits zero even with findings; explicitly reject them before staging.
    & go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 -json ./... > "$out/audit.json"
    if ($LASTEXITCODE) { throw 'Vulnerability audit failed' }
    Invoke-Go -Arguments @('run','./scripts/auditcheck',"$out/audit.json")
    $stage = New-Item -ItemType Directory -Path "$out/package"
    Invoke-Go -Arguments @('build','-trimpath','-buildvcs=true','-o',"$stage/wintable.exe",'./cmd/wintable')
    Invoke-Go -Arguments @('version','-m',"$stage/wintable.exe") | Set-Content "$stage/build-info.txt" -Encoding utf8
    Copy-Item examples "$stage/examples" -Recurse
    New-Item -ItemType Directory "$stage/docs" | Out-Null
    Copy-Item docs/usage.md,docs/release.md,docs/command-module.md,docs/file-content-module.md,docs/linux-command.md,docs/support.md,docs/ci.md "$stage/docs"
    @('# wintable Windows amd64', '', '導入・SSH準備・利用方法: [docs/usage.md](docs/usage.md)', '', '配布条件と検証範囲: [docs/release.md](docs/release.md)', '', 'このZIPは検証用候補です。公開可否はbuild.jsonと対応する受け入れ証跡で確認してください。') | Set-Content "$stage/README.md" -Encoding utf8
    $licenses = New-Item -ItemType Directory "$stage/licenses"
    $goroot = (& go env GOROOT).Trim()
    Copy-Item "$goroot/LICENSE" "$licenses/Go-LICENSE"
    $inventory = @([ordered]@{name='Go'; version='go1.27.3'; license='Go-LICENSE'})
    # Resolve license sources from the exact module versions, including transitive dependencies.
    $rows = & go list -m -f '{{if not .Main}}{{.Path}} {{.Version}}{{end}}' all
    if ($LASTEXITCODE) { throw 'Module enumeration failed' }
    foreach ($row in $rows) {
        if (-not $row.Trim()) { continue }
        $parts = $row.Split(' ')
        $info = (& go mod download -json "$($parts[0])@$($parts[1])" | ConvertFrom-Json)
        if ($LASTEXITCODE -or -not $info.Dir) { throw 'Module download failed' }
        $name = $parts[0].Replace('/','-') + '-LICENSE'
        if (-not (Test-Path "$($info.Dir)/LICENSE")) { throw "Missing license: $row" }
        Copy-Item "$($info.Dir)/LICENSE" "$licenses/$name"
        foreach ($notice in @('PATENTS','NOTICE','COPYING')) {
            if (Test-Path "$($info.Dir)/$notice") { Copy-Item "$($info.Dir)/$notice" "$licenses/$($parts[0].Replace('/','-'))-$notice" }
        }
        $inventory += [ordered]@{name=$parts[0]; version=$parts[1]; license=$name}
    }
    $inventory | ConvertTo-Json -Depth 5 | Set-Content "$licenses/index.json" -Encoding utf8
    # The owner-approved product license is tracked separately from dependency licenses.
    if (-not $ProductLicense -or -not (Test-Path -LiteralPath $ProductLicense -PathType Leaf)) {
        throw 'Product license required: supply -ProductLicense with an owner-approved redistribution license. Audit/build outputs retained; no ZIP created.'
    }
    Copy-Item -LiteralPath $ProductLicense -Destination "$stage/LICENSE"
    Copy-Item "$out/audit.json" "$stage/audit.json"
    [ordered]@{source_commit=$sha; go='go1.27.3'; scanner='govulncheck v1.9.0'; created_utc=[DateTime]::UtcNow.ToString('o'); release_status='candidate; requires acceptance evidence'} | ConvertTo-Json | Set-Content "$stage/build.json" -Encoding utf8
    # Bind the common SSH candidate to the audited, licensed distribution.
    $files = @(Get-ChildItem $stage -File -Recurse | ForEach-Object {
        @{path=[IO.Path]::GetRelativePath($stage.FullName,$_.FullName).Replace('\','/');sha256=(Get-FileHash $_.FullName).Hash}
    })
    @{schema=1;source_sha=$sha;go=(& go version);files=$files} |
        ConvertTo-Json -Depth 8 | Set-Content "$stage/distribution.json" -Encoding utf8NoBOM
    $hashes = @(Get-ChildItem $stage -File -Recurse | ForEach-Object {
        [ordered]@{path=[IO.Path]::GetRelativePath($stage.FullName,$_.FullName).Replace('\','/'); sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLowerInvariant()}
    })
    ConvertTo-Json -InputObject $hashes | Set-Content "$stage/SHA256.json" -Encoding utf8
    $zip = "$out/wintable-windows-amd64-$sha.zip"
    Compress-Archive -Path "$stage/*" -DestinationPath $zip
    $hash = (Get-FileHash $zip -Algorithm SHA256).Hash.ToLowerInvariant()
    "$hash  $([IO.Path]::GetFileName($zip))" | Set-Content "$zip.sha256" -Encoding ascii
    & "$PSScriptRoot/verify-package.ps1" -Zip $zip -ExpectedSHA256 $hash
    if (-not $?) { throw 'Package verification failed' }
} finally { Pop-Location }


