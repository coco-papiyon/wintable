#requires -Version 7.3
[CmdletBinding()]
param(
    [string]$DockerContext = 'desktop-linux',
    # ssh-config alias for the SAME dedicated host as DockerContext (ssh://).
    [string]$TunnelHost,
    [string]$OutputDirectory,
    [string]$LockFile = "$PSScriptRoot/fixture.lock.json",
    [switch]$NoCache,
    [ValidateSet('none','partial','startup','test','ready-timeout','timeout','cleanup')]
    [string]$Fault = 'none',
    # #28 may supply a fixed, repository-owned product acceptance script.
    [string]$ProductTestScript
)
$ErrorActionPreference = 'Stop'
$repo = (Resolve-Path "$PSScriptRoot/../..").Path
$runId = [Guid]::NewGuid().ToString('N')
$name = "wintable-linux-$runId"
$image = "wintable-linux-ssh:$runId"
$label = "wintable.linux-ssh.run=$runId"
if (!$OutputDirectory) { $OutputDirectory = "$repo/.cache/linux-ssh/$runId" }
$output = [IO.Path]::GetFullPath($OutputDirectory)
if (Test-Path -LiteralPath $output) { throw 'OutputDirectory must be new' }
New-Item -ItemType Directory -Path $output | Out-Null
$secret = Join-Path ([IO.Path]::GetTempPath()) $name
$manifest = [ordered]@{
    schema=1; run_id=$runId; source_sha=$null; source_status=@(); started_utc=[DateTime]::UtcNow.ToString('o')
    controller_os=[Environment]::OSVersion.VersionString; controller_arch=[Runtime.InteropServices.RuntimeInformation]::OSArchitecture.ToString()
    docker_context=$DockerContext; container=$name; network=$name; image_tag=$image
    volumes='unnecessary'; status='failed'; fault=$Fault; cleanup=@(); product='not_run (#28)'
}
$oldFixture = $env:WINTABLE_LINUX_SSH_FIXTURE
$deadline = [DateTime]::UtcNow.AddMinutes(10) # Includes preflight, build and all tests.
$tunnel = $null
$failed = $false
$cleanupFailed = $false
$phase = 'run'
$managementValidated = $false
function Save-JSON($Value, [string]$File) {
    $Value | ConvertTo-Json -Depth 12 | Set-Content -LiteralPath "$output/$File" -Encoding utf8NoBOM
}
function Invoke-Bounded([string]$File, [string[]]$Arguments, [int]$MaxSeconds = 600, [switch]$AllowFailure) {
    $remaining = [int][Math]::Floor(($deadline - [DateTime]::UtcNow).TotalMilliseconds)
    if ($remaining -le 0) { throw "deadline_exceeded ($phase)" }
    $info = [Diagnostics.ProcessStartInfo]::new($File)
    $info.UseShellExecute=$false; $info.CreateNoWindow=$true
    $info.RedirectStandardOutput=$true; $info.RedirectStandardError=$true
    foreach ($arg in $Arguments) { $info.ArgumentList.Add($arg) }
    $process = [Diagnostics.Process]::Start($info)
    $stdout=$process.StandardOutput.ReadToEndAsync(); $stderr=$process.StandardError.ReadToEndAsync()
    try {
        if (!$process.WaitForExit([Math]::Min($remaining, $MaxSeconds*1000))) {
            $process.Kill($true); $process.WaitForExit(); throw "deadline_exceeded ($phase child)"
        }
        $result=@{ code=$process.ExitCode; stdout=$stdout.GetAwaiter().GetResult(); stderr=$stderr.GetAwaiter().GetResult() }
    } finally { $process.Dispose() }
    # Never include arguments or arbitrary process output in exceptions.
    if ($result.code -and !$AllowFailure) { throw "child_failed ($File exit $($result.code))" }
    return $result
}
function Docker([string[]]$Arguments, [int]$MaxSeconds=30, [switch]$AllowFailure) {
    Invoke-Bounded docker (@('--context',$DockerContext)+$Arguments) $MaxSeconds -AllowFailure:$AllowFailure
}
function Restrict-Directory([string]$Path) {
    $sid=[Security.Principal.WindowsIdentity]::GetCurrent().User
    $acl=[Security.AccessControl.DirectorySecurity]::new()
    $acl.SetOwner($sid); $acl.SetAccessRuleProtection($true,$false)
    $acl.AddAccessRule([Security.AccessControl.FileSystemAccessRule]::new($sid,'FullControl','ContainerInherit,ObjectInherit','None','Allow'))
    Set-Acl -LiteralPath $Path -AclObject $acl
}
Push-Location $repo
try {
    if (!$IsWindows -or [Runtime.InteropServices.RuntimeInformation]::OSArchitecture -ne 'X64') { throw 'Windows amd64 controller required' }
    $manifest.source_sha=(Invoke-Bounded git @('rev-parse','HEAD') 10).stdout.Trim()
    $manifest.source_status=@((Invoke-Bounded git @('status','--short') 10).stdout.Trim())
    $lock=Get-Content -LiteralPath $LockFile -Raw | ConvertFrom-Json
    $manifest.lock_sha256=(Get-FileHash -LiteralPath $LockFile -Algorithm SHA256).Hash
    $manifest.transport_test_sha256=(Get-FileHash "$repo/internal/transport/ssh_linux_container_test.go" -Algorithm SHA256).Hash
    $manifest.fixture_hashes=@(Get-ChildItem $PSScriptRoot -File | ForEach-Object { @{name=$_.Name; sha256=(Get-FileHash $_.FullName -Algorithm SHA256).Hash} })
    $endpoint=((Docker @('context','inspect',$DockerContext)).stdout | ConvertFrom-Json)[0].Endpoints.docker.Host
    if ($TunnelHost) {
        if ($TunnelHost -notmatch '^[a-zA-Z0-9][a-zA-Z0-9_.-]*$' -or $endpoint -notmatch '^ssh://' -or ([uri]$endpoint).Host -ne $TunnelHost) { throw 'TunnelHost must match the ssh-config alias of the ssh:// Docker context' }
        $sshConfig=(Invoke-Bounded ssh @('-G',$TunnelHost) 10).stdout
        if ($sshConfig -notmatch '(?m)^stricthostkeychecking (yes|true)\r?$' -or $sshConfig -notmatch '(?m)^batchmode (yes|true)\r?$') { throw 'Docker management ssh-config must set StrictHostKeyChecking yes and BatchMode yes' }
    } elseif ($endpoint -notmatch '^npipe://') { throw 'Remote Docker requires TunnelHost; only local named pipes are accepted without it' }
    $managementValidated=$true
    $dockerVersion=(Docker @('version','--format','{{json .}}')).stdout | ConvertFrom-Json
    $manifest.docker=$dockerVersion
    if ([version]$dockerVersion.Client.Version -lt [version]'28.0' -or [version]$dockerVersion.Server.Version -lt [version]'28.0' -or $dockerVersion.Server.Os -ne 'linux' -or $dockerVersion.Server.Arch -ne 'amd64') { throw 'Docker client/server >=28 and linux/amd64 required' }
    $manifest.go=(Invoke-Bounded go @('version') 30).stdout.Trim()
    if ($manifest.go -notmatch 'go1\.27\.3 windows/amd64') { throw 'go.mod toolchain go1.27.3 windows/amd64 required' }
    $dockerfile=Get-Content "$PSScriptRoot/Dockerfile" -Raw
    if (!$dockerfile.Contains("FROM $($lock.base)`n") -and !$dockerfile.Contains("FROM $($lock.base)`r`n")) { throw 'base digest lock mismatch' }
    if ($lock.schema -ne 1 -or $lock.snapshot -ne '20261105T000000Z' -or $lock.platform -ne 'linux/amd64') { throw 'unsupported lock' }
    $install=@(Get-Content "$PSScriptRoot/packages.install")
    if (@(Compare-Object $install @($lock.packages)).Count) { throw 'package install lock mismatch' }
    $buildArgs=@('build','--platform','linux/amd64','--label',$label,'-t',$image)
    if ($NoCache) { $buildArgs+='--no-cache' }
    $build=Docker ($buildArgs+@($PSScriptRoot)) 600 -AllowFailure
    # Build only processes this fixed context; it never sees temporary credentials.
    ($build.stdout+$build.stderr) | Set-Content "$output/build.log" -Encoding utf8NoBOM
    if ($build.code) { throw 'image build failed' }
    $manifest.image_id=(Docker @('image','inspect',$image,'--format','{{.Id}}')).stdout.Trim()
    Docker @('network','create','--label',$label,$name) | Out-Null
    if ($Fault -eq 'partial') { throw 'injected_partial_failure' }
    New-Item -ItemType Directory -Path $secret | Out-Null
    Restrict-Directory $secret
    Invoke-Bounded ssh-keygen @('-q','-t','ed25519','-N','','-f',"$secret/client") 15 | Out-Null
    $create=@('create','--name',$name,'--label',$label,'--network',$name,'--publish','127.0.0.1::22',$image)
    if ($Fault -eq 'startup') { $create=@('create','--name',$name,'--label',$label,'--network',$name,'--publish','127.0.0.1::22','--entrypoint','/bin/false',$image) }
    Docker $create | Out-Null
    Docker @('cp',"$secret/client.pub","${name}:/home/runner/.ssh/authorized_keys") | Out-Null
    Docker @('start',$name) | Out-Null
    $readyDeadline=[DateTime]::UtcNow.AddSeconds(60)
    if ($Fault -eq 'ready-timeout') { $readyDeadline=[DateTime]::UtcNow.AddMilliseconds(-1) }
    $runDeadline=$deadline
    if ($readyDeadline -lt $deadline) { $deadline=$readyDeadline }
    do {
        if ([DateTime]::UtcNow -ge $readyDeadline) { throw 'ready_deadline_exceeded' }
        $savedDeadline=$deadline; if ($readyDeadline -lt $deadline) { $deadline=$readyDeadline }
        try { $key=Docker @('exec',$name,'cat','/run/sshd/ssh_host_ed25519_key.pub') 5 -AllowFailure } finally { $deadline=$savedDeadline }
        if ($key.code -eq 0 -and $key.stdout -match '^ssh-ed25519 ') { break }
        if ((Docker @('inspect',$name,'--format','{{.State.Running}}')).stdout.Trim() -ne 'true') { throw 'sshd startup failed' }
        Start-Sleep -Milliseconds 200
    } while ($true)
    $binding=((Docker @('inspect',$name,'--format','{{json .NetworkSettings.Ports}}')).stdout | ConvertFrom-Json).'22/tcp'
    if (@($binding).Count -ne 1 -or $binding[0].HostIp -ne '127.0.0.1') { throw 'unsafe port publication' }
    $port=[int]$binding[0].HostPort
    $manifest.docker_port=$port
    if ($TunnelHost) {
        $listener=[Net.Sockets.TcpListener]::new([Net.IPAddress]::Loopback,0); $listener.Start()
        $localPort=([Net.IPEndPoint]$listener.LocalEndpoint).Port; $listener.Stop()
        $info=[Diagnostics.ProcessStartInfo]::new('ssh'); $info.UseShellExecute=$false; $info.CreateNoWindow=$true
        foreach ($arg in @('-N','-T','-o','BatchMode=yes','-o','StrictHostKeyChecking=yes','-o','ExitOnForwardFailure=yes','-o','ConnectTimeout=10','-o','ServerAliveInterval=5','-o','ServerAliveCountMax=2','-L',"127.0.0.1:${localPort}:127.0.0.1:$port",$TunnelHost)) { $info.ArgumentList.Add($arg) }
        $tunnel=[Diagnostics.Process]::Start($info); $manifest.tunnel_pid=$tunnel.Id; $port=$localPort
    }
    $parts=$key.stdout.Trim().Split(' ')
    "[127.0.0.1]:$port $($parts[0]) $($parts[1])" | Set-Content "$secret/known_hosts" -Encoding utf8NoBOM
    @{Address='127.0.0.1';Port=$port;User='runner';Container=$name;DockerContext=$DockerContext;RunId=$runId} | ConvertTo-Json | Set-Content "$secret/fixture.json" -Encoding utf8NoBOM
    $actual=(Docker @('exec',$name,'/usr/bin/python3','-I','/fixture/versions.py')).stdout | ConvertFrom-Json
    Save-JSON $actual 'versions.json'
    $versionOK= !@(Compare-Object @($lock.packages) @($actual.packages)).Count -and $actual.python -eq $lock.python -and $actual.sshd -eq $lock.sshd -and $actual.arch -eq 'x86_64' -and $actual.implementation -eq 'cpython' -and $actual.python.StartsWith('3.13.') -and $actual.shell -eq 'dash'
    Save-JSON @{expected=$lock;actual=$actual;match=$versionOK} 'version-comparison.json'
    if (!$versionOK) { throw 'image version lock mismatch' }
    $effective=(Docker @('exec',$name,'/usr/sbin/sshd','-T')).stdout
    foreach ($required in @('permitrootlogin no','passwordauthentication no','kbdinteractiveauthentication no','authenticationmethods publickey','allowusers runner')) {
        if ($effective -notmatch "(?m)^$([regex]::Escape($required))\r?$") { throw 'effective sshd policy mismatch' }
    }
    Save-JSON @{required_policy_verified=$true;password_login='disabled';root_login='disabled'} 'ssh-policy.json'
    # Ready includes actual SSH banner through the Windows-facing endpoint.
    do {
        if ([DateTime]::UtcNow -ge $readyDeadline -or [DateTime]::UtcNow -ge $deadline) { throw 'ready_deadline_exceeded' }
        if ($tunnel -and $tunnel.HasExited) { throw 'authenticated tunnel exited' }
        $client=[Net.Sockets.TcpClient]::new()
        try {
            $client.ConnectAsync('127.0.0.1',$port).Wait(500) | Out-Null
            if ($client.Connected) {
                $stream=$client.GetStream(); $stream.ReadTimeout=500
                $buffer=[byte[]]::new(256); $n=$stream.Read($buffer,0,256)
                if ([Text.Encoding]::ASCII.GetString($buffer,0,$n).StartsWith('SSH-2.0-')) { break }
            }
        } catch { } finally { $client.Dispose() }
        Start-Sleep -Milliseconds 100
    } while ($true)
    $deadline=$runDeadline
    $env:WINTABLE_LINUX_SSH_FIXTURE=$secret
    if ($Fault -eq 'test') { throw 'injected_test_failure' }
    if ($Fault -eq 'timeout') {
        $deadline=[DateTime]::UtcNow.AddSeconds(1)
        Invoke-Bounded pwsh @('-NoProfile','-Command','Start-Sleep 30') 30 | Out-Null
    }
    $test=Invoke-Bounded go @('test','./internal/transport','-run','^TestLinuxOpenSSH$','-count=1','-json','-timeout=4m') 240 -AllowFailure
    $test.stdout | Set-Content "$output/test.jsonl" -Encoding utf8NoBOM
    # The test only uses fixed synthetic input. Do not save arbitrary product output.
    $events=@($test.stdout -split "`n" | Where-Object {$_} | ForEach-Object { $_ | ConvertFrom-Json })
    $expected=@('TestLinuxOpenSSH/reject/unknown','TestLinuxOpenSSH/reject/changed','TestLinuxOpenSSH/reject/auth','TestLinuxOpenSSH/reject/root','TestLinuxOpenSSH/reject/password','TestLinuxOpenSSH/reuse_hash_flood','TestLinuxOpenSSH/deadline_no-read','TestLinuxOpenSSH/deadline_wait','TestLinuxOpenSSH/cancel','TestLinuxOpenSSH/disconnect','TestLinuxOpenSSH')
    $passed=@($events | Where-Object Action -eq 'pass' | ForEach-Object Test)
    $manifest.test_exit_code=$test.code
    $manifest.cases=@($events | Where-Object { $_.Action -in 'pass','fail','skip' -and $_.Test } | Select-Object Test,Action,Elapsed)
    if ($test.code -or @($events | Where-Object Action -eq 'skip').Count -or @($expected | Where-Object {$_ -notin $passed}).Count) { throw 'required Linux SSH acceptance failed or skipped' }
    if ($ProductTestScript) {
        $product=Invoke-Bounded pwsh @('-NoProfile','-File',([IO.Path]::GetFullPath($ProductTestScript)),'-FixtureDirectory',$secret,'-EvidenceDirectory',$output) 240 -AllowFailure
        $manifest.product=@{exit_code=$product.code}; if ($product.code) { throw 'product acceptance failed' }
    }
    $manifest.status='passed'
} catch {
    $failed=$true; $manifest.failure=$_.Exception.Message
} finally {
    $manifest.test_finished_utc=[DateTime]::UtcNow.ToString('o')
    $env:WINTABLE_LINUX_SSH_FIXTURE=$oldFixture
    $phase='cleanup'; $deadline=[DateTime]::UtcNow.AddSeconds(90)
    # Every resource name is owned from before its create call (including partial failures).
    try {
        $exists=if ($managementValidated) {Docker @('container','ls','-a','-q','--filter',"label=$label") 10} else {@{stdout=''}}
        if ($exists.stdout.Trim()) {
            $logs=Docker @('logs',$name) 10 -AllowFailure
            if ($logs.code) {throw 'log collection failed'}
            ($logs.stdout+$logs.stderr) | Set-Content "$output/sshd.log" -Encoding utf8NoBOM
            foreach ($file in @('starts','hash.json','flood.json','wait.json')) {
                $copy=Docker @('cp',"${name}:/fixture/results/$file","$output/$file") 5 -AllowFailure
                if ($manifest.Contains('test_exit_code') -and $manifest.test_exit_code -eq 0 -and $copy.code) {throw 'required probe evidence missing'}
            }
        }
    } catch { $manifest.log_collection='incomplete'; $failed=$true }
    if ($tunnel) { try { if (!$tunnel.HasExited) {$tunnel.Kill($true);$tunnel.WaitForExit(3000)|Out-Null}; if (!$tunnel.HasExited) {throw 'tunnel survived'} } catch {$cleanupFailed=$true} finally {$tunnel.Dispose()} }
    foreach ($kind in @('container','network','image')) {
        try {
            if (!$managementValidated) {
                $manifest.cleanup+=@{kind=$kind;remaining=0;inspection='no creation attempted; management preflight rejected'}
                continue
            }
            $target=if ($kind -eq 'image') {$image} else {$name}
            $present=Docker @($kind,'ls','-q','--filter',"label=$label") 10
            if ($kind -eq 'container') { $present=Docker @('container','ls','-aq','--filter',"label=$label") 10 }
            if ($present.stdout.Trim()) {
                $removeArgs=@($kind,'rm'); if ($kind -in 'container','image') {$removeArgs+='-f'}
                Docker ($removeArgs+@($target)) 15 | Out-Null
            }
            $query=@($kind,'ls','-q','--filter',"label=$label"); if ($kind -eq 'container') {$query=@('container','ls','-aq','--filter',"label=$label")}
            $left=(Docker $query 10).stdout.Trim()
            if ($left) { throw 'owned resource remains' }
            $manifest.cleanup+=@{kind=$kind;remaining=0}
        } catch { $cleanupFailed=$true; $manifest.cleanup+=@{kind=$kind;error='cleanup or residual inspection failed'} }
    }
    # Fixed filenames only: never recursively delete a user-supplied directory.
    try {
        foreach ($file in @('client','client.pub','known_hosts','fixture.json','wrong-client')) {
            $path=Join-Path $secret $file
            if (Test-Path -LiteralPath $path) { Remove-Item -LiteralPath $path -Force }
        }
        if (Test-Path -LiteralPath $secret) { Remove-Item -LiteralPath $secret }
        if (Test-Path -LiteralPath $secret) {throw 'credential directory remains'}
        $manifest.cleanup+=@{kind='credentials';remaining=0}
    } catch { $cleanupFailed=$true; $manifest.cleanup+=@{kind='credentials';error='credential cleanup failed'} }
    if ($Fault -eq 'cleanup') { $cleanupFailed=$true; $manifest.cleanup+=@{kind='injection';error='simulated cleanup failure after actual removal'} }
    $manifest.cleanup_ok=!$cleanupFailed
    if ($cleanupFailed -or $failed) { $manifest.status='failed' }
    $manifest.finished_utc=[DateTime]::UtcNow.ToString('o')
    Save-JSON $manifest 'manifest.json'
    Pop-Location
    Write-Host "Evidence: $output"
}
if ($failed -or $cleanupFailed) { throw "Linux SSH fixture failed; see $output/manifest.json" }
