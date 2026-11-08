#requires -Version 7.3
param([string]$OutputDirectory = "$PSScriptRoot/../../dist/package")
$ErrorActionPreference='Stop'
$repo=(Resolve-Path "$PSScriptRoot/../..").Path
if (Test-Path -LiteralPath $OutputDirectory) { throw 'Package directory must be new' }
$output=[IO.Path]::GetFullPath($OutputDirectory)
$auditOutput="$output-audit"
# Keep the existing vulnerability audit, license inventory and ZIP checks.
& "$repo/scripts/distribution.ps1" -OutputDirectory $auditOutput -ProductLicense "$repo/LICENSE"
New-Item (Split-Path $output) -ItemType Directory -Force | Out-Null
Copy-Item -LiteralPath "$auditOutput/package" -Destination $output -Recurse
& "$PSScriptRoot/check-package.ps1" -Directory $output -SourceSHA (& git -C $repo rev-parse HEAD)
$zips=@(Get-ChildItem $auditOutput -Filter '*.zip' -File)
if ($zips.Count -ne 1) { throw 'Expected one audited ZIP' }
Copy-Item -LiteralPath $zips[0].FullName -Destination "$output.zip"
