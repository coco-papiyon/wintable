$ErrorActionPreference = 'Stop'
$serverPath = Join-Path $PSScriptRoot '../tools/github-mcp-server/github-mcp-server.exe'
if (-not (Test-Path -LiteralPath $serverPath)) {
    throw 'GitHub MCP server binary is missing. See docs/github-mcp-setup.md.'
}

# Read the existing credential into this process only; never print or persist it.
if (-not $env:GITHUB_PERSONAL_ACCESS_TOKEN) {
    $token = & gh auth token --hostname github.com 2>$null
    if ($LASTEXITCODE -ne 0 -or -not $token) {
        throw 'GitHub authentication is unavailable. Run gh auth login --hostname github.com.'
    }
    $env:GITHUB_PERSONAL_ACCESS_TOKEN = ($token -join '').Trim()
    $token = $null
}

try {
    & $serverPath stdio --toolsets context,repos,issues,pull_requests,labels,users
    $serverExitCode = $LASTEXITCODE
} finally {
    Remove-Item Env:GITHUB_PERSONAL_ACCESS_TOKEN -ErrorAction SilentlyContinue
}
exit $serverExitCode
