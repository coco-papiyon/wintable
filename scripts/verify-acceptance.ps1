param([Parameter(Mandatory)][string]$Zip, [Parameter(Mandatory)][string]$Evidence)
$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest
$record = Get-Content -LiteralPath $Evidence -Raw | ConvertFrom-Json
$hash = (Get-FileHash -LiteralPath $Zip -Algorithm SHA256).Hash
if ($record.zip_sha256 -ne $hash) { throw 'Acceptance evidence belongs to another ZIP' }
& "$PSScriptRoot/verify-package.ps1" -Zip $Zip -ExpectedSHA256 $record.zip_sha256
$archive = [IO.Compression.ZipFile]::OpenRead((Resolve-Path -LiteralPath $Zip))
try {
    $reader = [IO.StreamReader]::new($archive.GetEntry('build.json').Open())
    try { $build = $reader.ReadToEnd() | ConvertFrom-Json } finally { $reader.Dispose() }
} finally { $archive.Dispose() }
if ($record.source_commit -cne $build.source_commit -or $record.source_commit -notmatch '^[a-f0-9]{40}$') { throw 'Acceptance source mismatch' }
if ($record.issue11_url -notmatch '^https://github\.com/coco-papiyon/wintable/issues/11(#issuecomment-[0-9]+)?$' -or -not $record.reviewer) { throw 'Missing acceptance prerequisites' }
foreach ($flag in @('clean_container','no_go_wsl_python','non_admin_install','japanese_space_path')) {
    if ($record.$flag -isnot [bool] -or $record.$flag -ne $true) { throw "Unconfirmed prerequisite: $flag" }
}
$ids = @{}
foreach ($case in $record.scenarios) {
    if ($case.id -notmatch '^V-(0[1-9]|1[0-8])$' -or $ids.ContainsKey($case.id) -or $case.status -cne 'passed') { throw 'Invalid, duplicate or non-passing scenario' }
    $ids[$case.id] = $true
    if ($case.log -match '(^[/\\]|:|(^|[/\\])\.\.([/\\]|$))') { throw 'Invalid evidence path' }
    $log = Join-Path (Split-Path (Resolve-Path -LiteralPath $Evidence)) $case.log
    if ($case.sha256 -notmatch '^[a-fA-F0-9]{64}$' -or (Get-FileHash -LiteralPath $log -Algorithm SHA256).Hash -ne $case.sha256) { throw 'Evidence log hash mismatch' }
}
if ($ids.Count -ne 18) { throw 'All V-01 through V-18 are required' }
Write-Output "Acceptance manifest verified for $($build.source_commit). Human review of actual logs is still required before publication."
