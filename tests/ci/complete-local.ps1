#requires -Version 7.3
param(
    [Parameter(Mandatory)][ValidatePattern('^[0-9]+$')][string]$RunId,
    [string]$OutputDirectory,
    [string]$DockerContext='desktop-linux',
    [switch]$WindowsLocal,
    [string]$WindowsDockerContext='desktop-windows'
)
$ErrorActionPreference='Stop'
$repo=(Resolve-Path "$PSScriptRoot/../..").Path
. "$PSScriptRoot/check-run.ps1"
function Invoke-Gh([string[]]$Arguments) {
    $value=& gh @Arguments
    if ($LASTEXITCODE) { throw 'GitHub operation failed' }
    return $value
}
$previousEncoding=[Console]::OutputEncoding
$previousContext=$env:DOCKER_CONTEXT
$previousZip=$env:WINTABLE_DISTRIBUTION_ZIP
Push-Location $repo
try {
    [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false)
    if (!$IsWindows) { throw 'Windows controller required' }
    $sha=(& git rev-parse HEAD).Trim()
    if ($LASTEXITCODE -or @(& git status --porcelain --untracked-files=all).Count) { throw 'Checkout the clean source SHA used by the CI run' }
    $repository=(Invoke-Gh @('repo','view','--json','nameWithOwner','--jq','.nameWithOwner')).Trim()
    $run=(Invoke-Gh @('api',"repos/$repository/actions/runs/$RunId")) | ConvertFrom-Json
    $pages=(Invoke-Gh @('api','--paginate','--slurp',"repos/$repository/actions/runs/$RunId/jobs?per_page=100")) | ConvertFrom-Json
    $jobs=@($pages | ForEach-Object { $_.jobs })
    $windowsExecution=Get-AcceptancePlan -Run $run -Jobs $jobs -SourceSHA $sha -Repository $repository -WindowsLocal:$WindowsLocal
    if (!$OutputDirectory) { $OutputDirectory="$repo/.cache/local-acceptance/$([guid]::NewGuid().ToString('N'))" }
    $output=[IO.Path]::GetFullPath($OutputDirectory)
    if (Test-Path -LiteralPath $output) { throw 'OutputDirectory must be new' }
    New-Item $output -ItemType Directory | Out-Null
    Invoke-Gh @('run','download',$RunId,'--repo',$repository,'--name','candidate','--dir',"$output/candidate") | Out-Null
    $package="$output/candidate/package"
    & "$PSScriptRoot/check-package.ps1" -Directory $package -SourceSHA $sha
    & "$repo/scripts/verify-package.ps1" -Zip "$package.zip" -ExpectedSHA256 (Get-FileHash "$package.zip").Hash
    if ($windowsExecution -eq 'github-actions') {
        Invoke-Gh @('run','download',$RunId,'--repo',$repository,'--name','windows-evidence','--dir',"$output/evidence") | Out-Null
    } else {
        Invoke-Gh @('run','download',$RunId,'--repo',$repository,'--name','hosted-windows-capabilities','--dir',"$output/capability") | Out-Null
        $capability=Get-Content "$output/capability/windows-capability.json" -Raw | ConvertFrom-Json
        if ($capability.source_sha -cne $sha -or $capability.status -cne 'unavailable') { throw 'Local Windows fallback requires matching capability evidence' }
        $env:DOCKER_CONTEXT=$WindowsDockerContext
        $env:WINTABLE_DISTRIBUTION_ZIP="$package.zip"
        & pwsh -NoProfile -File "$repo/tests/validation/run.ps1" -DistributionDirectory $package -OutputDirectory "$output/raw/windows"
        if ($LASTEXITCODE) { throw 'Local Windows acceptance failed' }
        & "$PSScriptRoot/export-evidence.ps1" -Kind windows -InputDirectory "$output/raw/windows" -OutputDirectory "$output/evidence"
    }
    & pwsh -NoProfile -File "$repo/tests/linux-ssh/run.ps1" -DockerContext $DockerContext -ProductTestScript "$repo/tests/linux-ssh/product.ps1" -ProductDistributionDirectory $package -OutputDirectory "$output/raw/linux"
    if ($LASTEXITCODE) { throw 'Local Linux acceptance failed' }
    & "$PSScriptRoot/export-evidence.ps1" -Kind linux -InputDirectory "$output/raw/linux" -OutputDirectory "$output/evidence"
    & "$PSScriptRoot/gate.ps1" -EvidenceDirectory "$output/evidence" -DistributionDirectory $package -SourceSHA $sha -WindowsJob success -LinuxJob success -WindowsExecution $windowsExecution -LinuxExecution local -WorkflowRunId $RunId
    Copy-Item -LiteralPath "$package.zip" -Destination "$output/validated-distribution.zip"
    @{source_sha=$sha;workflow_run_id=$RunId;workflow_run_attempt=$run.run_attempt;workflow_url=$run.html_url;windows_execution=$windowsExecution;linux_execution='local'} |
        ConvertTo-Json | Set-Content "$output/evidence/execution.json" -Encoding utf8NoBOM
    Write-Host "Acceptance passed: $output/evidence/publication.json"
} finally {
    $env:DOCKER_CONTEXT=$previousContext
    $env:WINTABLE_DISTRIBUTION_ZIP=$previousZip
    [Console]::OutputEncoding=$previousEncoding
    Pop-Location
}
