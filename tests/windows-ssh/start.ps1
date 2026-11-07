$ErrorActionPreference = 'Stop'
& C:/OpenSSH/ssh-keygen.exe -A
if ($LASTEXITCODE) { throw 'host key generation failed' }
& icacls.exe C:\ProgramData\ssh\ssh_host_ed25519_key /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
& icacls.exe C:\ProgramData\ssh\administrators_authorized_keys /inheritance:r /grant:r '*S-1-5-18:F' '*S-1-5-32-544:F' | Out-Null
Start-Service sshd
# Stay alive after the deliberate disconnect test so logs can be collected.
while ($true) { Start-Sleep 1 }
