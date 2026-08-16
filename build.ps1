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
$DistDir = "dist"

function Build([string]$os, [string]$arch) {
    $suffix = if ($os -eq "windows") { ".exe" } else { "" }
    $out    = if ($All) {
        # Release mode: always fully qualified names, collected in dist/ for upload.
        Join-Path $DistDir "$Module-$os-$arch$suffix"
    } elseif ($os -eq (& go env GOOS) -and $arch -eq (& go env GOARCH)) {
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
    if (Test-Path $DistDir) { Remove-Item -Recurse -Force $DistDir }
    New-Item -ItemType Directory -Force $DistDir | Out-Null

    Build "linux"   "amd64"
    Build "linux"   "arm64"
    Build "darwin"  "amd64"
    Build "darwin"  "arm64"
    Build "windows" "amd64"

    Write-Host ""
    Write-Host "[-] Generating SHA256SUMS..." -ForegroundColor Cyan
    $sumsPath = Join-Path $DistDir "SHA256SUMS"
    Get-ChildItem -Path $DistDir -Filter "$Module-*" |
        Sort-Object Name |
        ForEach-Object { "$((Get-FileHash $_.FullName -Algorithm SHA256).Hash.ToLower())  $($_.Name)" } |
        Set-Content -Path $sumsPath -Encoding ascii
    Write-Host "  [OK] $sumsPath" -ForegroundColor Green
    Get-Content $sumsPath | Write-Host
} else {
    Build $Target $Arch
}

Write-Host ""
Write-Host "Build complete." -ForegroundColor Green
