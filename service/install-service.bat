@echo off
REM install-service.bat (quack-proxy)
REM Registers quack-proxy as a Windows service via NSSM.
REM
REM Why NSSM: plain `sc.exe create` cannot run console apps - `sc.exe start`
REM fails with 1053 (the binary has no StartServiceCtrlDispatcher). NSSM hosts
REM the console app and speaks the SCM protocol, so the registered service is a
REM real SCM service (sc.exe stop/delete also work for removal).
REM
REM Runs from {app}\service\ - all paths resolve from this script's location.
REM Requires administrator privileges (the installer runs elevated).

setlocal
set "PROXY_DIR=%~dp0..\quack-proxy"
set "DATA_DIR=%~dp0..\data"
set "NSSM=%~dp0nssm.exe"

echo [1/6] Ensuring database files exist...
REM Option B: quack-proxy links libduckdb in-process - it creates and
REM validates its own database files. No duckdb.exe CLI is shipped.
REM No -c flag: the config is installed as the default name
REM (quack-proxy.yaml) next to the exe, so no path argument is needed.
"%PROXY_DIR%\quack-proxy.exe" init-db >nul 2>&1
if errorlevel 1 (
    echo ERROR: database initialisation failed
    exit /b 1
)

echo [2/6] Opening Windows Firewall for the quack and admin ports...
REM The proxy binds 0.0.0.0 (blended/remote-access model). The admin
REM endpoint serves read-only status, so its port is opened as well.
REM NOTE: the default network setup is for a SINGLE shard - only the
REM first shard port (9491) is opened; additional shards need their own
REM rules. Idempotent: delete any previous rules first, then add.
netsh advfirewall firewall delete rule name="Quack-Proxy" >nul 2>&1
netsh advfirewall firewall add rule name="Quack-Proxy" dir=in action=allow protocol=TCP localport=9491
netsh advfirewall firewall delete rule name="Quack-Proxy Admin" >nul 2>&1
netsh advfirewall firewall add rule name="Quack-Proxy Admin" dir=in action=allow protocol=TCP localport=9490
if errorlevel 1 (
    echo WARNING: firewall rule creation failed
)

echo [3/6] Registering QuackProxy service via NSSM...
REM Idempotent: clear any existing registration first.
"%NSSM%" status QuackProxy >nul 2>&1
if not errorlevel 1 (
    echo   Existing QuackProxy registration found - removing...
    "%NSSM%" stop QuackProxy >nul 2>&1
    "%NSSM%" remove QuackProxy confirm >nul 2>&1
    timeout /t 2 /nobreak >nul
)

REM NO paths in AppParameters: NSSM mangles quoted args containing spaces
REM ("C:\Program Files\..." arrives as "-c C:\Program"). The proxy finds
REM quack-proxy.yaml next to its own exe by default.
"%NSSM%" install QuackProxy "%PROXY_DIR%\quack-proxy.exe" start
if errorlevel 1 (
    echo ERROR: nssm install failed
    exit /b 1
)

echo [4/6] Configuring restart-on-failure recovery...
REM Restart on any exit, after a 5-second delay.
"%NSSM%" set QuackProxy AppExit Default Restart
"%NSSM%" set QuackProxy AppRestartDelay 5000

echo [5/6] Capturing service output...
REM NSSM can rotate the console output the SCM cannot show.
if not exist "%DATA_DIR%\" mkdir "%DATA_DIR%"
"%NSSM%" set QuackProxy AppStdout "%DATA_DIR%\quack-proxy.out.log"
"%NSSM%" set QuackProxy AppStderr "%DATA_DIR%\quack-proxy.err.log"
"%NSSM%" set QuackProxy AppRotateFiles 1
"%NSSM%" set QuackProxy AppRotateBytes 1048576

echo [6/6] Starting QuackProxy service...
"%NSSM%" start QuackProxy
if errorlevel 1 (
    echo ERROR: nssm start failed
    exit /b 1
)

echo.
echo QuackProxy service installed and started.
endlocal
