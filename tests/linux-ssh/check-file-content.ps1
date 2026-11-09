param([Parameter(Mandatory)][object[]]$Events)
$ErrorActionPreference='Stop'
$coverage=Get-Content "$PSScriptRoot/../ci/linux-required.json" -Raw | ConvertFrom-Json
foreach ($name in @($coverage.'product.jsonl' | Where-Object { $_ -clike 'TestLinuxFileContentSSH*' })) {
    $records=@($Events | Where-Object { $_.Test -ceq $name -and $_.Action -in 'pass','fail','skip' })
    if ($records.Count -ne 1 -or $records[0].Action -cne 'pass') { throw "Missing/duplicate/failed/skipped L07 case: $name" }
}
