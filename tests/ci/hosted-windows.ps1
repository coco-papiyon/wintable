#requires -Version 7.3
param(
    [string]$OutputFile = '.cache/ci/windows-capability.json',
    [string]$BaseImage = 'mcr.microsoft.com/windows/servercore@sha256:b52799a67fa596d4bddcd82ae1dab0077a84767fbc33c7342b72b53ecc71ad09'
)
$ErrorActionPreference='Stop'
$name='wintable-capability-'+[guid]::NewGuid().ToString('N')
$record=@{source_sha=(& git rev-parse HEAD);status='unavailable';isolation='process';base_image=$BaseImage;container_removed=$false}
try {
    if (!$IsWindows) { throw 'Windows controller required' }
    $service=Get-Service docker -ErrorAction Stop
    if ($service.Status -ne 'Running') { Start-Service docker }
    $ready=$false
    for ($i=0;$i -lt 30;$i++) {
        $os=& docker info --format '{{.OSType}}' 2>$null
        if ($LASTEXITCODE -eq 0 -and $os -eq 'windows') { $ready=$true;break }
        Start-Sleep -Seconds 2
    }
    if (!$ready) { throw 'Windows Docker unavailable' }
    $ssh=Join-Path $env:WINDIR 'System32/OpenSSH'
    if (!(Test-Path "$ssh/sshd.exe")) {
        $installed=Add-WindowsCapability -Online -Name 'OpenSSH.Server~~~~0.0.1.0' -ErrorAction Stop
        if ($installed.RestartNeeded) { throw 'OpenSSH server installation requires reboot' }
    }
    foreach ($binary in 'sshd.exe','ssh-keygen.exe','ssh-shellhost.exe') {
        if (!(Test-Path "$ssh/$binary")) { throw 'OpenSSH server binaries unavailable' }
    }
    $record.openssh_directory=$ssh
    & docker run --name $name --isolation process $BaseImage powershell.exe -NoProfile -Command 'Write-Output ready'
    if ($LASTEXITCODE) { throw 'Process-isolated Windows containers unavailable' }
    $record.status='available'
} finally {
    & docker rm -f $name 2>$null | Out-Null
    # A missing container is also clean (e.g. failure before creation).
    $remaining=@(& docker ps -a --filter "name=^/$name$" --format '{{.Names}}' 2>$null)
    $record.container_removed=($LASTEXITCODE -eq 0 -and $remaining.Count -eq 0)
    New-Item (Split-Path ([IO.Path]::GetFullPath($OutputFile))) -ItemType Directory -Force | Out-Null
    $record | ConvertTo-Json | Set-Content $OutputFile -Encoding utf8NoBOM
}
if (!$record.container_removed) { throw 'Capability container cleanup incomplete' }
