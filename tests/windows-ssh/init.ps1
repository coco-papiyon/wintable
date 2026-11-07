$ErrorActionPreference = 'Stop'
New-Item C:/ProgramData/ssh -ItemType Directory -Force | Out-Null
# ContainerAdministrator is a synthetic container identity and cannot be
# resolved by sshd's normal local-user logon. Create an isolated real SAM user.
$random = New-Object byte[] 32
$rng = [Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($random)
$password = ConvertTo-SecureString ('Aa1!' + [Convert]::ToBase64String($random)) -AsPlainText -Force
New-LocalUser -Name wintablessh -Password $password -PasswordNeverExpires | Out-Null
Add-LocalGroupMember -SID 'S-1-5-32-544' -Member wintablessh
# The service must run as SYSTEM for Windows public-key logon.
& sc.exe create sshd binPath= 'C:\OpenSSH\sshd.exe -E C:\ProgramData\ssh\sshd.log' start= demand obj= LocalSystem
if ($LASTEXITCODE) { throw 'sc create failed' }
& sc.exe privs sshd SeAssignPrimaryTokenPrivilege/SeTcbPrivilege/SeBackupPrivilege/SeRestorePrivilege/SeImpersonatePrivilege
if ($LASTEXITCODE) { throw 'sc privs failed' }
New-Item 'HKLM:/SOFTWARE/OpenSSH' -Force | Out-Null
New-ItemProperty 'HKLM:/SOFTWARE/OpenSSH' -Name DefaultShell -Value 'C:\Windows\System32\cmd.exe' -PropertyType String -Force | Out-Null
@'
Port 22
HostKey C:/ProgramData/ssh/ssh_host_ed25519_key
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitTTY no
AllowUsers wintablessh
AuthorizedKeysFile C:/ProgramData/ssh/administrators_authorized_keys
LogLevel VERBOSE
SyslogFacility LOCAL0
'@ | Set-Content C:/ProgramData/ssh/sshd_config -Encoding ascii
# ContainerAdministrator has a container-specific SID. Use the well-known
# Administrators group as owner, which sshd running as SYSTEM can validate.
foreach ($path in 'C:\OpenSSH','C:\ProgramData\ssh') {
    & icacls.exe $path /setowner '*S-1-5-32-544' /T | Out-Null
    if ($LASTEXITCODE) { throw 'directory owner setup failed' }
    & icacls.exe $path /inheritance:r /grant:r '*S-1-5-18:(OI)(CI)F' '*S-1-5-32-544:(OI)(CI)F' '*S-1-5-11:(OI)(CI)RX' | Out-Null
    if ($LASTEXITCODE) { throw 'directory ACL setup failed' }
}
# Keys are generated at runtime, never baked into an image.
