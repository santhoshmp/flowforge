# FlowForge release build: cross-compile the single binary for the full
# distribution matrix (linux/darwin/windows x amd64/arm64), package archives,
# and emit SHA256SUMS. Pure-Go deps (modernc sqlite, wazero, starlark) mean
# CGO_ENABLED=0 works everywhere.
#
# Usage:  scripts/build.ps1 [-Version v1.2.3] [-OutDir dist]
#
# The embedded UI must be built first (npm --prefix app run build; copy
# app/dist/* to server-go/ui/dist/) - the script warns if the placeholder
# embed is detected.

param(
    [string]$Version = "dev",
    [string]$OutDir = "dist"
)

$ErrorActionPreference = "Stop"
$repo = Split-Path $PSScriptRoot -Parent
$uiDist = Join-Path $repo "server-go/ui/dist"

if (-not (Test-Path (Join-Path $uiDist "assets"))) {
    Write-Warning "server-go/ui/dist has no assets/ - the placeholder UI will be embedded."
    Write-Warning "Build the real UI first:  npm --prefix app install; npm --prefix app run build; Copy-Item -Recurse -Force app/dist/* server-go/ui/dist/"
}

$targets = @(
    @{ os = "linux";   arch = "amd64" },
    @{ os = "linux";   arch = "arm64" },
    @{ os = "darwin";  arch = "amd64" },
    @{ os = "darwin";  arch = "arm64" },
    @{ os = "windows"; arch = "amd64" },
    @{ os = "windows"; arch = "arm64" }
)

$out = Join-Path $repo $OutDir
New-Item -ItemType Directory -Force -Path $out | Out-Null
$ldflags = "-s -w -X main.version=$Version"
$archives = @()

foreach ($t in $targets) {
    $name = "flowforge-$Version-$($t.os)-$($t.arch)"
    $ext = if ($t.os -eq "windows") { ".exe" } else { "" }
    $bin = Join-Path $out "$name$ext"
    Write-Host "==> building $name"
    Push-Location (Join-Path $repo "server-go")
    try {
        $env:CGO_ENABLED = "0"
        $env:GOOS = $t.os
        $env:GOARCH = $t.arch
        go build -trimpath -ldflags $ldflags -o $bin ./cmd/flowforge
        if ($LASTEXITCODE -ne 0) { throw "build failed for $name" }
    } finally {
        Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED -ErrorAction SilentlyContinue
        Pop-Location
    }

    $entry = if ($t.os -eq "windows") { "flowforge.exe" } else { "flowforge" }
    if ($t.os -eq "windows") {
        $zip = Join-Path $out "$name.zip"
        Compress-Archive -Force -Path $bin -DestinationPath $zip
        Remove-Item $bin
        $archives += $zip
    } else {
        # Archive the binary under the plain name `flowforge`.
        Rename-Item $bin $entry
        $tgz = Join-Path $out "$name.tar.gz"
        tar -czf $tgz -C $out $entry
        Remove-Item (Join-Path $out $entry)
        $archives += $tgz
    }
}

$sums = Join-Path $out "SHA256SUMS"
$lines = foreach ($a in $archives) {
    $h = (Get-FileHash -Algorithm SHA256 $a).Hash.ToLower()
    "$h  $(Split-Path $a -Leaf)"
}
$lines | Set-Content -Encoding ascii $sums
Write-Host "`nRelease artifacts in $out"
$lines | Write-Host
