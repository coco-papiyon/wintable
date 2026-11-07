param([ValidateSet('transfer','blocked','wait','disconnect','version')][string]$Mode = 'transfer')
$ErrorActionPreference = 'Stop'
[IO.File]::AppendAllText('C:\fixture\execs.log', "$Mode`n")
if ($Mode -eq 'version') {
    [Console]::WriteLine((@{ OS = [Environment]::OSVersion.VersionString; PowerShell = $PSVersionTable.PSVersion.ToString(); DefaultShell = (Get-ItemProperty HKLM:/SOFTWARE/OpenSSH).DefaultShell } | ConvertTo-Json -Compress))
    exit 0
}
if ($Mode -eq 'blocked') { Start-Sleep 120; exit 0 }
if ($Mode -eq 'disconnect') { Start-Sleep 2; Stop-Process -Name sshd -Force; exit 0 }
# Write both streams before consuming stdin: this exercises bidirectional flow
# control while the client is still sending more than the SSH channel window.
Add-Type -TypeDefinition @'
using System;
using System.IO;
using System.Threading.Tasks;
public static class FixtureIO {
    static void Emit(Stream stream, int count, byte value) {
        byte[] data = new byte[count];
        for (int i = 0; i < count; i++) data[i] = value;
        stream.Write(data, 0, data.Length);
        stream.Flush();
    }
    public static byte[] Exchange() {
        Task a = Task.Run(() => Emit(Console.OpenStandardOutput(), 1048576 + 12345, 97));
        Task b = Task.Run(() => Emit(Console.OpenStandardError(), 1048576 + 7, 98));
        Stream input = Console.OpenStandardInput();
        using (MemoryStream all = new MemoryStream()) {
            byte[] buffer = new byte[7];
            int n;
            while ((n = input.Read(buffer, 0, buffer.Length)) > 0) all.Write(buffer, 0, n);
            Task.WaitAll(a, b);
            return all.ToArray();
        }
    }
}
'@
$received = [FixtureIO]::Exchange()
$sha = [Security.Cryptography.SHA256]::Create()
$digest = [BitConverter]::ToString($sha.ComputeHash($received)).Replace('-', '').ToLowerInvariant()
[IO.File]::WriteAllText("C:\fixture\received-$Mode.json", (@{ bytes = $received.Length; sha256 = $digest; eof = $true } | ConvertTo-Json -Compress))
if ($Mode -eq 'wait') { Start-Sleep 120 }
exit 7
