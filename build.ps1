#!/usr/bin/env pwsh
# build.ps1 — Cross-platform build script for ai-ssh-tools
# Usage: .\build.ps1 [-Target linux|windows|darwin] [-Arch amd64|arm64]

param(
    [string]$Target = "windows",
    [string]$Arch   = "amd64",
    [switch]$UPX,
    [switch]$All
)

$Module  = "ai-ssh-tools"
$LdFlags = "-s -w"

function Build([string]$os, [string]$arch) {
    $suffix = if ($os -eq "windows") { ".exe" } else { "" }
    $out    = if ($os -eq (& go env GOOS) -and $arch -eq (& go env GOARCH)) {
        "$Module$suffix"
    } else {
        "$Module-$os-$arch$suffix"
    }

    Write-Host "[-] Building $out ($os/$arch)..." -ForegroundColor Cyan
    $env:GOOS   = $os
    $env:GOARCH = $arch
    & go build -ldflags="$LdFlags" -trimpath -o $out ./cmd/ai-ssh-tools
    if ($LASTEXITCODE -ne 0) {
        Write-Error "Build failed for $os/$arch"
        exit 1
    }

    $size = [math]::Round((Get-Item $out).Length / 1MB, 2)
    Write-Host "  [OK] $out ($size MB)" -ForegroundColor Green

    if ($UPX) {
        Write-Host "  Compressing with UPX..." -ForegroundColor Yellow
        & upx --best --lzma $out 2>&1 | Out-Null
        $sizePost = [math]::Round((Get-Item $out).Length / 1MB, 2)
        Write-Host "  [OK] Compressed to $sizePost MB" -ForegroundColor Green
    }
}

Write-Host ""
Write-Host "+----------------------------------+" -ForegroundColor DarkCyan
Write-Host "|   ai-ssh-tools build script      |" -ForegroundColor DarkCyan
Write-Host "+----------------------------------+" -ForegroundColor DarkCyan
Write-Host ""

# Ensure dependencies are up to date
Write-Host "[-] Tidying module dependencies..." -ForegroundColor Cyan
& go mod tidy
if ($LASTEXITCODE -ne 0) { exit 1 }

if ($All) {
    Build "linux"   "amd64"
    Build "linux"   "arm64"
    Build "darwin"  "amd64"
    Build "darwin"  "arm64"
    Build "windows" "amd64"
} else {
    Build $Target $Arch
}

Write-Host ""
Write-Host "Build complete." -ForegroundColor Green
