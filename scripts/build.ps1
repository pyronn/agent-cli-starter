param(
    [string]$Version = "dev"
)

$ErrorActionPreference = "Stop"
$Root = Split-Path -Parent $PSScriptRoot
$OutputDirectory = Join-Path $Root "dist"
$Commit = "none"
$PreviousErrorActionPreference = $ErrorActionPreference
$ErrorActionPreference = "SilentlyContinue"
$ResolvedCommit = git -C $Root rev-parse --verify --short HEAD 2>$null
$GitExitCode = $LASTEXITCODE
$ErrorActionPreference = $PreviousErrorActionPreference
if ($GitExitCode -eq 0 -and $ResolvedCommit) {
    $Commit = $ResolvedCommit
}
$BuildDate = (Get-Date).ToUniversalTime().ToString("yyyy-MM-ddTHH:mm:ssZ")
$Ldflags = "-s -w -X main.version=$Version -X main.commit=$Commit -X main.date=$BuildDate"

New-Item -ItemType Directory -Force -Path $OutputDirectory | Out-Null
go build -trimpath -ldflags $Ldflags -o (Join-Path $OutputDirectory "agentctl.exe") ./cmd/agentctl
Write-Host "Built $OutputDirectory\agentctl.exe"
