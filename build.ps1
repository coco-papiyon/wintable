$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    # A distribution whose fixed OpenSSH/cmd line exceeds 8,000 units must not ship.
    go test ./internal/module -run '^TestDistributionWrapper$' -count=1 -v
    if ($LASTEXITCODE) { throw 'Fixed wrapper validation failed' }
    go build -o dist/wintable.exe ./cmd/wintable
    if ($LASTEXITCODE) { throw 'Build failed' }
} finally { Pop-Location }
