$ErrorActionPreference='Stop'
. "$PSScriptRoot/check-run.ps1"
foreach ($mode in 'valid','source','status','cleanup-false','cleanup-missing','cleanup-null','cleanup-string','cleanup-number') {
    $capability=@{source_sha='source';status='unavailable';container_removed=$true}
    switch ($mode) {
        'source' { $capability.source_sha='other' }
        'status' { $capability.status='available' }
        'cleanup-false' { $capability.container_removed=$false }
        'cleanup-missing' { $capability.Remove('container_removed') }
        'cleanup-null' { $capability.container_removed=$null }
        'cleanup-string' { $capability.container_removed='true' }
        'cleanup-number' { $capability.container_removed=1 }
    }
    # Exercise the same JSON artifact decoding and guard used before local SSH tests.
    $artifact=$capability | ConvertTo-Json | ConvertFrom-Json
    $accepted=$false
    try {
        Assert-WindowsFallbackCapability -Capability $artifact -SourceSHA source
        $accepted=$true
    } catch { if ($mode -eq 'valid') { throw } }
    if ($accepted -ne ($mode -eq 'valid')) { throw "Wrong capability verdict: $mode" }
    Write-Host "PASS capability: $mode"
}
foreach ($mode in 'valid','source','repository','workflow','build','queued','test-failed','capability-without-opt-in','capability-fallback') {
    $run=@{head_sha='source';repository=@{full_name='owner/repo'};path='.github/workflows/ssh-acceptance.yml'}
    $build=@{name='build';conclusion='success'}
    $windows=@{name='windows';conclusion='success';steps=@()}
    $local=$false
    switch ($mode) {
        'source' { $run.head_sha='other' }
        'repository' { $run.repository.full_name='other/repo' }
        'workflow' { $run.path='other.yml' }
        'build' { $build.conclusion='failure' }
        'queued' { $windows.conclusion=$null }
        'test-failed' { $windows.conclusion='failure';$windows.steps=@(@{name='Windows real SSH acceptance';conclusion='failure'});$local=$true }
        'capability-without-opt-in' { $windows.conclusion='failure';$windows.steps=@(@{name='Windows container capability';conclusion='failure'}) }
        'capability-fallback' { $windows.conclusion='failure';$windows.steps=@(@{name='Windows container capability';conclusion='failure'});$local=$true }
    }
    $accepted=$false
    try {
        $result=Get-AcceptancePlan -Run $run -Jobs @($build,$windows) -SourceSHA source -Repository owner/repo -WindowsLocal:$local
        $accepted=$true
        if ($result -cne $(if ($local) {'local'} else {'github-actions'})) { throw 'Wrong execution provenance' }
    } catch { if ($mode -in 'valid','capability-fallback') { throw } }
    if ($accepted -ne ($mode -in 'valid','capability-fallback')) { throw "Wrong verdict: $mode" }
    Write-Host "PASS run: $mode"
}
