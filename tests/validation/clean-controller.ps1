#requires -Version 7.3
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Executable,
    [Parameter(Mandatory)][string]$FixtureDirectory,
    [Parameter(Mandatory)][string]$BaseImage,
    [string]$Isolation = 'hyperv'
)
$ErrorActionPreference = 'Stop'
$fixture = Get-Content "$FixtureDirectory/fixture.json" -Raw | ConvertFrom-Json
$root = Join-Path $FixtureDirectory 'controller'
New-Item -ItemType Directory -Path $root | Out-Null
$name = 'wintable-controller-' + [Guid]::NewGuid().ToString('N').Substring(0,12)
$record = [ordered]@{ base_image=$BaseImage; isolation=$Isolation; source_sha=(& git rev-parse HEAD); exit_code=$null; removed=$false }
try {
    Copy-Item -LiteralPath $Executable -Destination "$root/wintable.exe"
    Copy-Item -LiteralPath "$FixtureDirectory/client" -Destination "$root/client"
    Copy-Item -LiteralPath "$FixtureDirectory/known_hosts" -Destination "$root/known_hosts"
    Copy-Item -LiteralPath "$PSScriptRoot/controller-cases.ps1" -Destination "$root/cases.ps1"
    @{ version=1; hosts=@{ target=@{ address=$fixture.Address; port=$fixture.Port; user=$fixture.User; private_key_file='C:\validation\client'; known_hosts_file='C:\validation\known_hosts' } } } |
        ConvertTo-Json -Depth 8 | Set-Content "$root/inventory.json" -Encoding utf8NoBOM
    @{ version=1; hosts='all'; tasks=@(
        @{ name='command'; module='command'; args=@{ executable='C:\Windows\System32\whoami.exe'; argv=@() } },
        @{ name='file'; module='file_content'; args=@{ path=('C:\fixture\clean-' + $name + '.txt'); content="日本語 `"quoted`"`n" } }
    ) } | ConvertTo-Json -Depth 8 | Set-Content "$root/tasks.json" -Encoding utf8NoBOM
    $record.exe_sha256 = (Get-FileHash "$root/wintable.exe" -Algorithm SHA256).Hash
    if ($env:WINTABLE_DISTRIBUTION_ZIP) {
        $zip = [IO.Path]::GetFullPath($env:WINTABLE_DISTRIBUTION_ZIP)
        $record.zip_sha256 = (Get-FileHash -LiteralPath $zip -Algorithm SHA256).Hash
        & "$PSScriptRoot/../../scripts/verify-package.ps1" -Zip $zip -ExpectedSHA256 $record.zip_sha256
        Copy-Item -LiteralPath $zip -Destination "$root/candidate.zip"
        Copy-Item -LiteralPath "$PSScriptRoot/zip-controller-cases.ps1" -Destination "$root/zip-cases.ps1"
    }
    $arguments = @('run','--name',$name,'--isolation',$Isolation,'--mount',"type=bind,source=$root,target=C:/validation,readonly",$BaseImage,'powershell.exe','-NoProfile','-File','C:/validation/cases.ps1')
    $record.command = 'docker ' + ($arguments -join ' ')
    & docker @arguments 2>&1 | Tee-Object "$FixtureDirectory/controller.log" | Out-Host
    $record.exit_code = $LASTEXITCODE
    if ($record.exit_code -ne 0) { throw "Clean controller failed: $($record.exit_code)" }
    if (!(Select-String -LiteralPath "$FixtureDirectory/controller.log" -SimpleMatch 'CLEAN_CONTROLLER_PASS' -Quiet)) { throw 'Missing controller completion marker' }
    if ($env:WINTABLE_DISTRIBUTION_ZIP) {
        # Administrative fixture preparation is separate from ordinary-user installation.
        $zipName = "$name-zip"
        try {
            & docker run -d --name $zipName --isolation $Isolation --mount "type=bind,source=$root,target=C:/validation,readonly" $BaseImage powershell.exe -NoProfile -Command '$ErrorActionPreference="Stop"; if (Test-Path C:\Windows\System32\wsl.exe) { takeown.exe /F C:\Windows\System32\wsl.exe /A | Out-Null; icacls.exe C:\Windows\System32\wsl.exe /grant *S-1-5-32-544:F | Out-Null; Remove-Item C:\Windows\System32\wsl.exe -Force }; Set-Content C:\ready.txt ready; Start-Sleep -Seconds 600'
            if ($LASTEXITCODE) { throw 'ZIP controller setup failed' }
            & docker exec --user ContainerUser $zipName powershell.exe -NoProfile -File C:/validation/zip-cases.ps1 2>&1 | Tee-Object "$FixtureDirectory/zip-controller.log" | Out-Host
            $record.zip_exit_code = $LASTEXITCODE
            if ($record.zip_exit_code -ne 0) { throw 'ZIP controller failed' }
            if (!(Select-String -LiteralPath "$FixtureDirectory/zip-controller.log" -SimpleMatch 'ZIP_CONTROLLER_PASS' -Quiet)) { throw 'Missing ZIP completion marker' }
        } finally {
            & docker rm -f $zipName | Out-Null
            $record.zip_container_removed = $LASTEXITCODE -eq 0
        }
        if (!$record.zip_container_removed) { throw 'ZIP controller cleanup failed' }
    }
} finally {
    & docker rm -f $name 2>&1 | Out-Null
    $record.removed = $LASTEXITCODE -eq 0
    Remove-Item -LiteralPath "$root/client" -Force -ErrorAction SilentlyContinue
    $record | ConvertTo-Json -Depth 6 | Set-Content "$FixtureDirectory/controller.json" -Encoding utf8NoBOM
}
if (!$record.removed) { throw 'Controller cleanup failed' }
