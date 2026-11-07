#requires -Version 7.3
[CmdletBinding()]
param(
    [string]$BaseImage = 'mcr.microsoft.com/windows/servercore:ltsc2022',
    [ValidateSet('hyperv','process')][string]$Isolation = 'hyperv',
    [string]$OpenSSHDirectory = "$env:WINDIR\System32\OpenSSH",
    [string]$OutputDirectory
)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path (Join-Path $PSScriptRoot '../..')).Path
$runId = [Guid]::NewGuid().ToString('N').Substring(0,12)
$name = "wintable-ssh-$runId"
$image = "wintable-ssh-fixture:$runId"
if (!$OutputDirectory) { $OutputDirectory = Join-Path $repo ".cache/windows-ssh/$runId" }
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path $output) { throw 'OutputDirectory must be new (contains ephemeral test credentials)' }
New-Item $output -ItemType Directory -Force | Out-Null
$context = Join-Path $output 'context'
New-Item "$context/OpenSSH" -ItemType Directory -Force | Out-Null
function Invoke-Docker {
    & docker @args
    if ($LASTEXITCODE) { throw "docker failed ($LASTEXITCODE): $($args[0])" }
}
$oldFixture = $env:WINTABLE_SSH_FIXTURE
$created = $false
$built = $false
$result = 1
Push-Location $repo
try {
    if ((Invoke-Docker info --format '{{.OSType}}') -ne 'windows') { throw 'Switch Docker to Windows containers' }
    foreach ($file in 'sshd.exe','ssh-keygen.exe','ssh-shellhost.exe') {
        if (!(Test-Path (Join-Path $OpenSSHDirectory $file))) { throw "Missing OpenSSH binary: $file" }
    }
    # Copy program files only: a custom installation may also contain host keys.
    Get-ChildItem $OpenSSHDirectory -File | Where-Object { $_.Extension -in '.exe','.dll' -or $_.Name -in 'LICENSE.txt','NOTICE.txt','moduli','sshd_config_default' } | Copy-Item -Destination "$context/OpenSSH"
    Copy-Item "$PSScriptRoot/Dockerfile", "$PSScriptRoot/init.ps1", "$PSScriptRoot/start.ps1", "$PSScriptRoot/receiver.ps1" $context
    $manifest = @{
        source_sha = (git rev-parse HEAD)
        source_status = @(git status --short)
        started_utc = [DateTime]::UtcNow.ToString('o')
        base_image = $BaseImage
        isolation = $Isolation
        docker = @(& docker version --format '{{json .}}')
        go = (& go version)
        openssh = @(Get-ChildItem "$context/OpenSSH" -File | ForEach-Object { @{ name=$_.Name; version=$_.VersionInfo.FileVersion; sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash } })
        fixture_files = @(Get-ChildItem $PSScriptRoot -File | ForEach-Object { @{ name=$_.Name; sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash } })
    }
    $keygen = Join-Path $OpenSSHDirectory 'ssh-keygen.exe'
    & $keygen -q -t ed25519 -N '' -f "$output/client"
    if ($LASTEXITCODE) { throw 'client key generation failed' }
    Invoke-Docker build --isolation $Isolation --build-arg "BASE_IMAGE=$BaseImage" -t $image $context 2>&1 | Tee-Object "$output/build.log"
    $built = $true
    $manifest.image = (Invoke-Docker image inspect $image --format '{{.Id}}')
    $manifest.base_image_id = (Invoke-Docker image inspect $BaseImage --format '{{.Id}}')
    $manifest.base_repo_digests = (Invoke-Docker image inspect $BaseImage --format '{{json .RepoDigests}}')
    Invoke-Docker create --isolation $Isolation --name $name $image | Out-Null
    $created = $true
    Invoke-Docker cp "$output/client.pub" "${name}:C:/ProgramData/ssh/administrators_authorized_keys"
    Invoke-Docker start $name | Out-Null
    $ready = $false
    for ($i=0; $i -lt 60; $i++) {
        Start-Sleep 1
        $status = & docker exec $name powershell.exe -NoProfile -Command '(Get-Service sshd).Status.ToString()' 2>$null
        if ($LASTEXITCODE -eq 0 -and $status -eq 'Running') { $ready=$true; break }
    }
    if (!$ready) { throw 'sshd did not start; see container.log' }
    $address = (Invoke-Docker inspect $name --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}').Trim()
    if (!$address) { throw 'Container has no reachable NAT address' }
    # Trust comes through Docker, never unauthenticated ssh-keyscan or auto-enrolment.
    Invoke-Docker cp "${name}:C:/ProgramData/ssh/ssh_host_ed25519_key.pub" "$output/host.pub"
    $pub = (Get-Content "$output/host.pub" -Raw).Trim().Split(' ')
    "${address} $($pub[0]) $($pub[1])" | Set-Content "$output/known_hosts" -Encoding utf8NoBOM
    @{ Address=$address; Port=22; User='ContainerAdministrator'; Container=$name } | ConvertTo-Json | Set-Content "$output/fixture.json" -Encoding utf8NoBOM
    $env:WINTABLE_SSH_FIXTURE = $output
    $manifest.test_command = 'go test ./internal/transport -run ^TestWindowsOpenSSH$ -count=1 -v -timeout=4m'
    & go test ./internal/transport -run '^TestWindowsOpenSSH$' -count=1 -v -timeout=4m 2>&1 | Tee-Object "$output/test.log"
    $result = $LASTEXITCODE
    $manifest.test_exit_code = $result
} catch {
    if ($manifest) { $manifest.failure = $_.Exception.Message }
    throw
} finally {
    $env:WINTABLE_SSH_FIXTURE = $oldFixture
    if ($created) {
        & docker logs $name 2>&1 | Set-Content "$output/container.log"
        & docker cp "${name}:C:/fixture/execs.log" "$output/execs.log" 2>$null
        & docker cp "${name}:C:/fixture/received.json" "$output/received.json" 2>$null
        & docker rm -f $name | Out-Null
        $containerRemoved = $LASTEXITCODE -eq 0
    }
    if ($built) { & docker image rm $image | Out-Null; $imageRemoved=$LASTEXITCODE -eq 0 }
    # Only remove this run's known credential files, never directories supplied by a caller.
    if (Test-Path "$output/client") { Remove-Item -LiteralPath "$output/client" -Force }
    if ($manifest) {
        $manifest.container_removed = $containerRemoved
        $manifest.image_removed = $imageRemoved
        $manifest.finished_utc = [DateTime]::UtcNow.ToString('o')
        $manifest | ConvertTo-Json -Depth 8 | Set-Content "$output/manifest.json" -Encoding utf8NoBOM
    }
    Pop-Location
    Write-Host "Evidence: $output"
}
if ($result) { throw "Windows SSH acceptance failed (exit $result); see $output" }
