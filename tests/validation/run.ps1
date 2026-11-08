#requires -Version 7.3
[CmdletBinding()]
param(
    [string]$OutputDirectory,
    [string]$BaseImage = 'mcr.microsoft.com/windows/servercore@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09'
)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path "$PSScriptRoot/../..").Path
if (!$OutputDirectory) { $OutputDirectory = "$repo/.cache/validation/$([Guid]::NewGuid().ToString('N'))" }
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'Evidence directory must be new' }
New-Item -ItemType Directory -Path $output | Out-Null
$manifest = [ordered]@{
    source_sha = $null; source_status = @(); started_utc = [DateTime]::UtcNow.ToString('o')
    base_image = $BaseImage; checks = @(); acceptance = 'unconfirmed'
    launch_measurement = 'UTF-16 code units of cmd.exe /c plus quoted fixed wrapper; non-BMP counts as 2; limit 8000'
}
function Invoke-Check([string]$Name, [string[]]$Arguments) {
    $command = 'go ' + ($Arguments -join ' ')
    New-Item -ItemType File -Path "$output/$Name.log" -Force | Out-Null
    & go @Arguments 2>&1 | Tee-Object "$output/$Name.log" | Out-Host
    $code = $LASTEXITCODE
    $manifest.checks += @{ name=$Name; command=$command; exit_code=$code; log="$Name.log" }
    if ($code) { Write-Warning "$Name failed: $code" }
}
$previous = @{}
foreach ($name in 'GOOS','GOARCH','CGO_ENABLED') { $previous[$name] = [Environment]::GetEnvironmentVariable($name) }
Push-Location $repo
try {
    $manifest.source_sha = (& git rev-parse HEAD)
    $manifest.source_status = @(& git status --short)
    $manifest.runner_sha256 = (Get-FileHash $PSCommandPath -Algorithm SHA256).Hash
    $manifest.go = (& go version)
    $manifest.controller = @{ os=[Environment]::OSVersion.VersionString; architecture=$env:PROCESSOR_ARCHITECTURE; powershell=$PSVersionTable.PSVersion.ToString() }
    Invoke-Check test @('test','./...','-count=1','-v','-timeout=8m')
    Invoke-Check vet @('vet','./...')
    $env:GOOS='windows'; $env:GOARCH='amd64'; $env:CGO_ENABLED='0'
    Invoke-Check wrappers @('test','./internal/module','-run','^TestDistributionWrapper$','-count=1','-v')
    Invoke-Check build @('build','-trimpath','-o',"$output/wintable.exe",'./cmd/wintable')
    Invoke-Check module_binary @('test','-c','-o',"$output/module.test.exe",'./internal/module')
    $nativeName = 'wintable-native-' + [Guid]::NewGuid().ToString('N').Substring(0,12)
    $nativeArgs = @('run','--name',$nativeName,'--isolation','hyperv','--mount',"type=bind,source=$output,target=C:/validation,readonly",$BaseImage,'C:/validation/module.test.exe','-test.run=^TestFileContentWindows$','-test.v','-test.timeout=3m')
    try {
        & docker @nativeArgs 2>&1 | Tee-Object "$output/native-container.log" | Out-Host
        $manifest.checks += @{ name='native_container'; command=('docker ' + ($nativeArgs -join ' ')); exit_code=$LASTEXITCODE; log='native-container.log' }
    } finally {
        & docker rm -f $nativeName 2>&1 | Out-Null
        $manifest.native_container_removed = $LASTEXITCODE -eq 0
    }
    $manifest.exe_sha256 = (Get-FileHash "$output/wintable.exe" -Algorithm SHA256).Hash
    & "$repo/tests/windows-ssh/run.ps1" -BaseImage $BaseImage -OutputDirectory "$output/ssh" -ValidationExecutable "$output/wintable.exe"
    $manifest.ssh_fixture = 'completed; see ssh/manifest.json for commands and exit codes'
} catch {
    $manifest.failure = $_.Exception.Message
    Write-Warning $_
} finally {
    foreach ($name in $previous.Keys) { [Environment]::SetEnvironmentVariable($name,$previous[$name]) }
    Pop-Location
    $manifest.finished_utc = [DateTime]::UtcNow.ToString('o')
    $manifest | ConvertTo-Json -Depth 10 | Set-Content "$output/manifest.json" -Encoding utf8NoBOM
    & "$PSScriptRoot/summarize.ps1" -EvidenceDirectory $output
    Write-Host "Evidence: $output"
}
# Partial coverage must never produce an acceptance success exit code.
exit 1
