#requires -Version 7.3
[CmdletBinding()]
param(
    [string]$BaseImage = 'mcr.microsoft.com/windows/servercore:ltsc2022',
    [ValidateSet('hyperv','process')][string]$Isolation = 'hyperv',
    [string]$OpenSSHDirectory = "$env:WINDIR\System32\OpenSSH",
    [string]$ModuleTestPattern = '^Test(Command(Windows|SSHLaunchAndDisconnect)|FileContentSSH(LaunchAndDisconnect)?|FileContentLinkSwapWindows)$',
    [string]$OutputDirectory,
    [string]$ValidationExecutable
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
    $info = [Diagnostics.ProcessStartInfo]::new('docker')
    $info.UseShellExecute = $false
    $info.CreateNoWindow = $true
    $info.RedirectStandardOutput = $true
    $info.RedirectStandardError = $true
    foreach ($arg in $args) { $info.ArgumentList.Add([string]$arg) }
    $process = [Diagnostics.Process]::Start($info)
    $stdout = $process.StandardOutput.ReadToEndAsync()
    $stderr = $process.StandardError.ReadToEndAsync()
    $timeout = if ($args[0] -eq 'build') { 600000 } else { 30000 }
    if (!$process.WaitForExit($timeout)) {
        $process.Kill($true)
        throw "docker timed out: $($args[0])"
    }
    $code = $process.ExitCode
    $text = $stdout.GetAwaiter().GetResult()
    $errors = $stderr.GetAwaiter().GetResult()
    $process.Dispose()
    if ($errors) { Write-Host $errors.TrimEnd() }
    if ($text) { $text.TrimEnd() }
    if ($code) { throw "docker failed ($code): $($args[0])" }
}
$oldFixture = $env:WINTABLE_SSH_FIXTURE
$oldExecutable = $env:WINTABLE_VALIDATION_EXE
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
    Copy-Item "$repo/tests/command/CRTReceiver.cs" $context
    $manifest = @{
        source_sha = (git rev-parse HEAD)
        source_status = @(git status --short)
        test_sha256 = (Get-FileHash (Join-Path $repo 'internal/transport/ssh_windows_container_test.go') -Algorithm SHA256).Hash
        command_sources = @(Get-ChildItem "$repo/internal/module" -File | ForEach-Object { @{ name=$_.Name; sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash } })
        crt_sha256 = (Get-FileHash "$repo/tests/command/CRTReceiver.cs" -Algorithm SHA256).Hash
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
        if ((Invoke-Docker inspect $name --format '{{.State.Running}}') -ne 'true') { throw 'Container exited during sshd startup; see container.log' }
        $status = Invoke-Docker exec $name powershell.exe -NoProfile -Command '(Get-Service sshd).Status.ToString()'
        if ($status -eq 'Running') { $ready=$true; break }
    }
    if (!$ready) { throw 'sshd did not start; see container.log' }
    $address = (Invoke-Docker inspect $name --format '{{range .NetworkSettings.Networks}}{{.IPAddress}}{{end}}').Trim()
    if (!$address) { throw 'Container has no reachable NAT address' }
    # Trust comes through Docker, never unauthenticated ssh-keyscan or auto-enrolment.
    # Hyper-V containers do not support docker cp while running.
    Invoke-Docker exec $name cmd.exe /c type C:\ProgramData\ssh\ssh_host_ed25519_key.pub | Set-Content "$output/host.pub" -Encoding ascii
    $pub = (Get-Content "$output/host.pub" -Raw).Trim().Split(' ')
    "${address} $($pub[0]) $($pub[1])" | Set-Content "$output/known_hosts" -Encoding utf8NoBOM
    @{ Address=$address; Port=22; User='wintablessh'; Container=$name } | ConvertTo-Json | Set-Content "$output/fixture.json" -Encoding utf8NoBOM
    $env:WINTABLE_SSH_FIXTURE = $output
    if ($ValidationExecutable) {
        $env:WINTABLE_VALIDATION_EXE = [IO.Path]::GetFullPath($ValidationExecutable)
        & "$repo/tests/validation/clean-controller.ps1" -Executable $ValidationExecutable -FixtureDirectory $output -BaseImage $BaseImage -Isolation $Isolation
        $manifest.clean_controller = 'see controller.json and controller.log'
        $manifest.cli_test_command = 'go test ./internal/cli -run ^TestWindowsCLIProcess$ -count=1 -v -timeout=2m'
        & go test ./internal/cli -run '^TestWindowsCLIProcess$' -count=1 -v -timeout=2m 2>&1 | Tee-Object "$output/cli-test.log"
        $manifest.cli_test_exit_code = $LASTEXITCODE
        if ($LASTEXITCODE) { throw 'CLI process acceptance failed' }
    }
    $manifest.command_test_command = "go test ./internal/module -run $ModuleTestPattern -count=1 -v -timeout=4m"
    & go test ./internal/module -run $ModuleTestPattern -count=1 -v -timeout=4m 2>&1 | Tee-Object "$output/command-test.log"
    $manifest.command_test_exit_code = $LASTEXITCODE
    if ($LASTEXITCODE) { throw 'command module acceptance failed' }
    $manifest.test_command = 'go test ./internal/transport -run ^TestWindowsOpenSSH$ -count=1 -v -timeout=4m'
    & go test ./internal/transport -run '^TestWindowsOpenSSH$' -count=1 -v -timeout=4m 2>&1 | Tee-Object "$output/test.log"
    $result = $LASTEXITCODE
    $manifest.test_exit_code = $result
} catch {
    if ($manifest) { $manifest.failure = $_.Exception.Message }
    throw
} finally {
    $env:WINTABLE_SSH_FIXTURE = $oldFixture
    $env:WINTABLE_VALIDATION_EXE = $oldExecutable
    if ($created) {
        try { Invoke-Docker exec $name powershell.exe -NoProfile -Command 'Get-CimInstance Win32_Process | Select-Object Name,ProcessId,ParentProcessId,CommandLine | ConvertTo-Json -Depth 3' | Set-Content "$output/processes.json" } catch { Write-Warning $_ }
        try { Invoke-Docker logs $name | Set-Content "$output/container.log" } catch { Write-Warning $_ }
        try { Invoke-Docker stop --time 1 $name | Out-Null } catch { Write-Warning $_ }
        foreach ($entry in @(@('C:/fixture/execs.log','execs.log'),@('C:/fixture/received-transfer.json','received-transfer.json'),@('C:/fixture/received-wait.json','received-wait.json'),@('C:/ProgramData/ssh/sshd.log','sshd.log'))) {
            try { Invoke-Docker cp "${name}:$($entry[0])" "$output/$($entry[1])" } catch { Write-Warning $_ }
        }
        try { Invoke-Docker rm -f $name | Out-Null; $containerRemoved = $true } catch { $containerRemoved = $false; Write-Warning $_ }
    }
    if ($built) { try { Invoke-Docker image rm $image | Out-Null; $imageRemoved=$true } catch { $imageRemoved=$false; Write-Warning $_ } }
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
