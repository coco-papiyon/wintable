#requires -Version 7.3
param([string]$OutputDirectory = "$PSScriptRoot/../../dist/package")
$ErrorActionPreference='Stop'
$repo=(Resolve-Path "$PSScriptRoot/../..").Path
if (Test-Path $OutputDirectory) { throw 'Package directory must be new' }
New-Item $OutputDirectory -ItemType Directory -Force | Out-Null
Copy-Item "$repo/dist/wintable.exe","$repo/README.md" $OutputDirectory
Copy-Item "$repo/docs","$repo/examples" $OutputDirectory -Recurse
$files=@(Get-ChildItem $OutputDirectory -Recurse -File | ForEach-Object {
    @{path=[IO.Path]::GetRelativePath($OutputDirectory,$_.FullName).Replace('\','/');sha256=(Get-FileHash $_.FullName).Hash}
})
@{schema=1;source_sha=(& git -C $repo rev-parse HEAD);go=(& go version);files=$files} |
    ConvertTo-Json -Depth 8 | Set-Content "$OutputDirectory/distribution.json" -Encoding utf8NoBOM
Compress-Archive -Path "$OutputDirectory/*" -DestinationPath "$OutputDirectory.zip"
$expanded="$OutputDirectory-check"
Expand-Archive "$OutputDirectory.zip" $expanded
foreach ($file in $files) {
    if ((Get-FileHash (Join-Path $expanded $file.path)).Hash -ne $file.sha256) { throw 'ZIP content mismatch' }
}
& "$expanded/wintable.exe" help | Out-Null
if ($LASTEXITCODE) { throw 'Packaged executable failed' }
