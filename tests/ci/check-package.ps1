param([Parameter(Mandatory)][string]$Directory,[Parameter(Mandatory)][string]$SourceSHA)
$ErrorActionPreference='Stop'
$m=Get-Content "$Directory/distribution.json" -Raw | ConvertFrom-Json
if ($m.source_sha -cne $SourceSHA -or $m.go -notmatch '^go version go1\.27\.3 windows/amd64$') { throw 'Distribution source/toolchain mismatch' }
$paths=@($m.files.path)
foreach ($required in 'wintable.exe','README.md','docs/linux-command.md','docs/support.md','docs/ci.md','examples/linux-inventory.json','examples/linux-command.json','examples/mixed-inventory.json','examples/mixed-command.json') {
    if (@($paths | Where-Object {$_ -ceq $required}).Count -ne 1) { throw "Missing/duplicate distribution entry: $required" }
}
foreach ($f in $m.files) {
    $full=[IO.Path]::GetFullPath((Join-Path $Directory $f.path))
    if (!$full.StartsWith([IO.Path]::GetFullPath($Directory)+[IO.Path]::DirectorySeparatorChar,[StringComparison]::OrdinalIgnoreCase) -or
        @($paths | Where-Object {$_ -ceq $f.path}).Count -ne 1 -or
        (Get-FileHash -LiteralPath $full).Hash -cne $f.sha256) { throw 'Distribution inventory mismatch' }
}
