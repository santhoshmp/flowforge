# Run FlowForge locally on Windows.
#
#   scripts/run-local.ps1 [-Port 8080] [-NoDemo] [-NoBuild]
#
# Builds the binary (skippable), loads the Meridian demo org (idempotent,
# skippable), and serves. Data lands in the repo root (flowforge.db is
# gitignored). First run without a DB: the UI offers admin account setup.

param(
    [int]$Port = 8080,
    [switch]$NoDemo,
    [switch]$NoBuild
)

$ErrorActionPreference = "Stop"
$repo = Split-Path $PSScriptRoot -Parent
Set-Location $repo

if (-not $NoBuild) {
    Write-Host "==> building flowforge.exe"
    Push-Location "$repo\server-go"
    try { go build -o flowforge.exe ./cmd/flowforge }
    finally { Pop-Location }
}

if (-not $NoDemo) {
    Write-Host "==> loading demo org (idempotent)"
    & "$repo\server-go\flowforge.exe" demo
}

# Avoid the common WSL relay squatter on 8080.
$busy = Get-NetTCPConnection -LocalPort $Port -State Listen -ErrorAction SilentlyContinue
if ($busy -and $Port -eq 8080) {
    Write-Warning "Port 8080 is in use (WSL relay?) - falling back to 8081."
    $Port = 8081
}

Write-Host "==> serving on http://localhost:$Port  (Ctrl+C to stop)"
$env:PORT = "$Port"
& "$repo\server-go\flowforge.exe" serve
