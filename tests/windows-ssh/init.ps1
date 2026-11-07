$ErrorActionPreference = 'Stop'
New-Item C:/ProgramData/ssh -ItemType Directory -Force | Out-Null
# The service must run as SYSTEM for Windows public-key logon.
& sc.exe create sshd binPath= 'C:\OpenSSH\sshd.exe' start= demand obj= LocalSystem
if ($LASTEXITCODE) { throw 'sc create failed' }
New-Item 'HKLM:/SOFTWARE/OpenSSH' -Force | Out-Null
New-ItemProperty 'HKLM:/SOFTWARE/OpenSSH' -Name DefaultShell -Value 'C:\Windows\System32\cmd.exe' -PropertyType String -Force | Out-Null
@'
Port 22
HostKey C:/ProgramData/ssh/ssh_host_ed25519_key
PubkeyAuthentication yes
PasswordAuthentication no
KbdInteractiveAuthentication no
PermitTTY no
AllowUsers containeradministrator
AuthorizedKeysFile C:/ProgramData/ssh/administrators_authorized_keys
LogLevel VERBOSE
'@ | Set-Content C:/ProgramData/ssh/sshd_config -Encoding ascii
# Keys are generated at runtime, never baked into an image.
