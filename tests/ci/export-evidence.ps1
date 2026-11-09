param([ValidateSet('windows','linux')][string]$Kind,[string]$InputDirectory,[string]$OutputDirectory)
$ErrorActionPreference='Stop'
# Explicit filenames only. Never upload a fixture directory, context or credentials.
$files=if ($Kind -eq 'windows') {
    @('manifest.json','coverage.json','scenarios.csv','test-cases.csv','sha256.csv','test.log','vet.log','wrappers.log','build.log','module_binary.log','native-container.log',
      'ssh/manifest.json','ssh/controller.json','ssh/controller.log','ssh/zip-controller.log','ssh/cli-test.log','ssh/command-test.log','ssh/test.log')
} else {
    @('manifest.json','versions.json','version-comparison.json','test.jsonl','product.jsonl','product-hash.json','product-failure.json','document-example.json','starts','hash.json','flood.json','wait.json','ssh-policy.json')
}
foreach ($file in $files) {
    $from=Join-Path $InputDirectory $file
    if (!(Test-Path -LiteralPath $from -PathType Leaf)) { continue }
    if (Select-String -LiteralPath $from -Pattern '-----BEGIN .*PRIVATE KEY-----' -Quiet) { throw 'Private key in evidence' }
    $to=Join-Path "$OutputDirectory/$Kind" $file
    New-Item (Split-Path $to) -ItemType Directory -Force | Out-Null
    Copy-Item -LiteralPath $from -Destination $to
}
if (!(Test-Path "$OutputDirectory/$Kind/manifest.json")) { throw 'Missing evidence manifest' }
