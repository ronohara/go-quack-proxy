# install-mingw.ps1 - installs Chocolatey + mingw-w64 14.2.0 on w11pro.
#
# Why mingw 14.2.0: it is the EXACT toolchain duckdb CI used to build the
# prebuilt static libs (BundleStaticLibs.yml: choco install mingw --version 14.2.0).
# Newer gcc (16.x) no longer ships the libstdc++ 14-era internals those
# libs reference, so the prebuilt archives only link against 14.2.0.
#
# Run from an ELEVATED PowerShell:
#   powershell -ExecutionPolicy Bypass -File Z:\go-quack-proxy\scripts\install-mingw.ps1

#Requires -RunAsAdministrator

$ErrorActionPreference = "Stop"

Write-Host "==> Checking for Chocolatey..." -ForegroundColor Cyan
$chocoCmd = Get-Command choco -ErrorAction SilentlyContinue
if (-not $chocoCmd) {
    Write-Host "    Chocolatey not found - installing..."
    Set-ExecutionPolicy Bypass -Scope Process -Force
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    Invoke-Expression ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
} else {
    Write-Host "    Chocolatey already present: $($chocoCmd.Source)"
}

# Refresh PATH so this session sees the freshly installed choco.
$machinePath = [Environment]::GetEnvironmentVariable("Path", "Machine")
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
$env:Path = "$machinePath;$userPath"

Write-Host "==> Installing mingw 14.2.0..." -ForegroundColor Cyan
choco install mingw --version 14.2.0 -y

Write-Host "==> Verifying..." -ForegroundColor Cyan
$gcc = "C:\ProgramData\mingw64\mingw64\bin\gcc.exe"
if (Test-Path $gcc) {
    & $gcc --version | Select-Object -First 1
    Write-Host "==> DONE. gcc ready at $gcc" -ForegroundColor Green
} else {
    Write-Host "==> WARNING: gcc not found at $gcc - check the choco output above." -ForegroundColor Yellow
    exit 1
}
