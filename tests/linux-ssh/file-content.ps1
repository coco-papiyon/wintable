param([Parameter(Mandatory)][string]$FixtureDirectory,[Parameter(Mandatory)][string]$EvidenceDirectory,[string]$DistributionDirectory)
$ErrorActionPreference='Stop'
$env:WINTABLE_LINUX_SSH_FIXTURE=$FixtureDirectory
$env:WINTABLE_PRODUCT_EXE=(Resolve-Path "$PSScriptRoot/../../dist/wintable.exe").Path
try {
 $events = & go test ./internal/module -run '^TestLinux(FileContentSSH|CommandSSH)$' -count=1 -json
 $code=$LASTEXITCODE
 $events | Set-Content "$EvidenceDirectory/file-content.jsonl" -Encoding utf8NoBOM
 $parsed=@($events|ForEach-Object {$_|ConvertFrom-Json})
 if ($code -or @($parsed|Where-Object Action -eq skip).Count) {throw 'Linux acceptance failed or skipped'}
 foreach($test in @('TestLinuxFileContentSSH/API-cases','TestLinuxFileContentSSH/commit-disconnect/before','TestLinuxFileContentSSH/commit-disconnect/after','TestLinuxFileContentSSH/product-bytes','TestLinuxCommandSSH')) {
  if (!@($parsed|Where-Object {$_.Action -eq 'pass' -and $_.Test -eq $test}).Count) {throw "Missing case $test"}
 }
 Get-FileHash $env:WINTABLE_PRODUCT_EXE | ConvertTo-Json | Set-Content "$EvidenceDirectory/file-content-product-hash.json"
} finally {
 Remove-Item Env:WINTABLE_LINUX_SSH_FIXTURE -ErrorAction SilentlyContinue
 Remove-Item Env:WINTABLE_PRODUCT_EXE -ErrorAction SilentlyContinue
}
