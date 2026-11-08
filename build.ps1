$ErrorActionPreference = 'Stop'
Push-Location $PSScriptRoot
try {
    $env:GOTOOLCHAIN = 'go1.27.3'
    $env:GOOS = 'windows'
    $env:GOARCH = 'amd64'
    $env:CGO_ENABLED = '0'
    # A distribution whose fixed OpenSSH/cmd line exceeds 8,000 units must not ship.
    go test ./internal/module -run '^TestDistributionWrapper$' -count=1 -v
    if ($LASTEXITCODE) { throw 'Fixed wrapper validation failed' }
    go build -trimpath -buildvcs=true -o dist/wintable.exe ./cmd/wintable
    if ($LASTEXITCODE) { throw 'Build failed' }
} finally { Pop-Location }
