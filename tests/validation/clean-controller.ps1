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
    $arguments = @('run','--name',$name,'--isolation',$Isolation,'--mount',"type=bind,source=$root,target=C:/validation,readonly",$BaseImage,'powershell.exe','-NoProfile','-File','C:/validation/cases.ps1')
    $record.command = 'docker ' + ($arguments -join ' ')
    & docker @arguments 2>&1 | Tee-Object "$FixtureDirectory/controller.log" | Out-Host
    $record.exit_code = $LASTEXITCODE
    if ($record.exit_code -ne 0) { throw "Clean controller failed: $($record.exit_code)" }
    if (!(Select-String -LiteralPath "$FixtureDirectory/controller.log" -SimpleMatch 'CLEAN_CONTROLLER_PASS' -Quiet)) { throw 'Missing controller completion marker' }
} finally {
    & docker rm -f $name 2>&1 | Out-Null
    $record.removed = $LASTEXITCODE -eq 0
    Remove-Item -LiteralPath "$root/client" -Force -ErrorAction SilentlyContinue
    $record | ConvertTo-Json -Depth 6 | Set-Content "$FixtureDirectory/controller.json" -Encoding utf8NoBOM
}
if (!$record.removed) { throw 'Controller cleanup failed' }
