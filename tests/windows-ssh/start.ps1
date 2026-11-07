$ErrorActionPreference = 'Stop'
& C:/OpenSSH/ssh-keygen.exe -A
if ($LASTEXITCODE) { throw 'host key generation failed' }
foreach ($path in 'C:\ProgramData\ssh\ssh_host_ed25519_key','C:\ProgramData\ssh\administrators_authorized_keys') {
    $acl = Get-Acl $path
    $acl.SetSecurityDescriptorSddlForm('O:BAG:SYD:P(A;;FA;;;SY)(A;;FA;;;BA)')
    Set-Acl -LiteralPath $path -AclObject $acl
}
& icacls.exe C:\ProgramData\ssh /setowner '*S-1-5-32-544' /T | Out-Null
if ($LASTEXITCODE) { throw 'SSH file owner setup failed' }
& C:/OpenSSH/sshd.exe -t -e
if ($LASTEXITCODE) { throw 'sshd configuration check failed' }
Start-Service sshd
# Stay alive after the deliberate disconnect test so logs can be collected.
while ($true) { Start-Sleep 1 }
