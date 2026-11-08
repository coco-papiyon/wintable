# Runs as ContainerUser in a disposable, digest-pinned Server Core container.
$ErrorActionPreference = 'Stop'
$ready = $false
foreach ($attempt in 1..60) {
    if (Test-Path C:/ready.txt) { $ready = $true; break }
    Start-Sleep -Seconds 1
}
if (!$ready) { throw 'Fixture preparation did not finish' }
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { throw 'ZIP installation must run without administrator rights' }
foreach ($tool in 'go','wsl','python','python3','py') {
    if (Get-Command $tool -ErrorAction SilentlyContinue) { throw "Unexpected tool: $tool" }
}
# ASCII source also works when Windows PowerShell 5.1 reads a BOM-less script.
$folder = (-join ([char[]]@(0x65e5,0x672c,0x8a9e))) + ' ZIP ' + (-join ([char[]]@(0x914d,0x5e03)))
$install = Join-Path $env:TEMP $folder
if ($install -notmatch '[\u3040-\u9fff]' -or $install -notmatch ' ') { throw 'Japanese/space path missing' }
Expand-Archive -LiteralPath C:/validation/candidate.zip -DestinationPath $install
$rows = Get-Content "$install/SHA256.json" -Raw | ConvertFrom-Json
foreach ($row in $rows) {
    if ((Get-FileHash (Join-Path $install $row.path) -Algorithm SHA256).Hash -ne $row.sha256) { throw 'Extracted package hash mismatch' }
}
$exe = Join-Path $install 'wintable.exe'
if ((Get-FileHash $exe).Hash -ne (Get-FileHash C:/validation/wintable.exe).Hash) { throw 'ZIP binary differs from validation binary' }
& $exe help
if ($LASTEXITCODE) { throw 'ZIP help failed' }
$tasks = Get-Content C:/validation/tasks.json -Raw | ConvertFrom-Json
$tasks.tasks[1].args.path += '.zip'
$taskFile = Join-Path $env:TEMP 'zip-tasks.json'
$tasks | ConvertTo-Json -Depth 8 | Set-Content $taskFile -Encoding UTF8
foreach ($iteration in 1..2) {
    $text = & $exe run --inventory C:/validation/inventory.json --tasks $taskFile --format json --include-output
    $code = $LASTEXITCODE
    $text | Write-Output
    if ($code) { throw "ZIP SSH run failed: $code" }
    $report = ($text -join "`n") | ConvertFrom-Json
    $expected = if ($iteration -eq 1) { 'changed' } else { 'ok' }
    if ($report.hosts.Count -ne 1 -or $report.hosts[0].tasks.Count -ne 2 -or
        $report.hosts[0].tasks[0].status -ne 'changed' -or $report.hosts[0].tasks[1].status -ne $expected) { throw 'Unexpected ZIP result or idempotence' }
}
[ordered]@{user=$identity.Name; non_admin=$true; install=$install; zip_sha256=(Get-FileHash C:/validation/candidate.zip).Hash; exe_sha256=(Get-FileHash $exe).Hash; build=(Get-Content "$install/build.json" -Raw | ConvertFrom-Json); tools_absent=@('go','wsl','python','python3','py'); exit_code=0} | ConvertTo-Json -Depth 8 | Write-Output
Write-Output 'ZIP_CONTROLLER_PASS'
