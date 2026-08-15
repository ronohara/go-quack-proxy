@echo off
REM === Quack-Proxy Windows build control (go-quack-proxy) ===
REM Builds the quack-proxy binary to windows\quack-proxy.exe and stages
REM the installer inputs under dist\ (gitignored).
REM
REM Option B: the build is CGo - libduckdb 1.5.5 links in-process. That
REM requires the mingw-w64 gcc the prebuilt static libs were built with:
REM Chocolatey mingw 14.2.0 (duckdb CI's exact toolchain - the libs carry
REM libstdc++ 14-era internals that gcc 16 no longer ships).
REM Install: choco install mingw --version 14.2.0 -y
REM PATH is set for this session only - no permanent environment change.

cd /d Z:\go-quack-proxy

echo [1/3] Locating C toolchain...
set "TOOLCHAIN=C:\ProgramData\mingw64\mingw64\bin"
if exist "%TOOLCHAIN%\gcc.exe" (
    set "PATH=%TOOLCHAIN%;%PATH%"
    echo   using %TOOLCHAIN%
) else (
    echo ERROR: gcc not found in %TOOLCHAIN%
    echo Install: choco install mingw --version 14.2.0 -y
    exit /b 1
)

if not exist windows mkdir windows
if not exist dist\config mkdir dist\config

echo [2/3] Building quack-proxy (CGo)...
go build -buildvcs=false -o windows\quack-proxy.exe ./cmd
if %ERRORLEVEL% NEQ 0 (
    echo BUILD FAILED
    exit /b 1
)

echo [3/3] Staging config template...
copy /y quack-proxy.example.yaml dist\config\quack-proxy.example.yaml >nul
if %ERRORLEVEL% NEQ 0 (
    echo STAGING FAILED
    exit /b 1
)

if exist windows\quack-proxy.exe (
    echo BUILD OK - quack-proxy.exe written to windows\
    exit /b 0
) else (
    echo BUILD FAILED - output not found
    exit /b 1
)
