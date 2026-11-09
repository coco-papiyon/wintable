function Assert-WindowsFallbackCapability {
    param($Capability,[string]$SourceSHA)
    if ($Capability.source_sha -cne $SourceSHA -or $Capability.status -cne 'unavailable') {
        throw 'Local Windows fallback requires matching capability evidence'
    }
    if ($Capability.container_removed -isnot [bool] -or $Capability.container_removed -ne $true) {
        throw 'Local Windows fallback requires successful capability container cleanup'
    }
}

function Get-AcceptancePlan {
    param($Run,[object[]]$Jobs,[string]$SourceSHA,[string]$Repository,[switch]$WindowsLocal)
    if ($Run.head_sha -cne $SourceSHA -or $Run.repository.full_name -cne $Repository -or
        $Run.path -cne '.github/workflows/ssh-acceptance.yml') { throw 'CI source/repository/workflow mismatch' }
    $build=@($Jobs | Where-Object name -ceq 'build')
    $windows=@($Jobs | Where-Object name -ceq 'windows')
    if ($build.Count -ne 1 -or $build[0].conclusion -cne 'success' -or $windows.Count -ne 1) { throw 'Common candidate build incomplete' }
    if ($windows[0].conclusion -ceq 'success') {
        if ($WindowsLocal) { throw 'Use successful hosted Windows evidence' }
        return 'github-actions'
    }
    $capability=@($windows[0].steps | Where-Object { $_.name -ceq 'Windows container capability' -and $_.conclusion -ceq 'failure' })
    $attempted=@($windows[0].steps | Where-Object { $_.name -ceq 'Windows real SSH acceptance' -and $_.conclusion -in 'success','failure','cancelled' })
    if ($WindowsLocal -and $windows[0].conclusion -ceq 'failure' -and $capability.Count -eq 1 -and !$attempted.Count) { return 'local' }
    throw 'Hosted Windows acceptance incomplete or failed; local fallback is allowed only for a capability failure'
}
