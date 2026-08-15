@echo off
REM uninstall-service.bat (quack-proxy)
REM Stops and removes the QuackProxy service registration (NSSM-registered)
REM and its firewall rule. Called by the uninstaller ([UninstallRun])
REM BEFORE files are deleted.
REM
REM Requires administrator privileges (the uninstaller runs elevated).

setlocal
set "NSSM=%~dp0nssm.exe"

"%NSSM%" status QuackProxy >nul 2>&1
if errorlevel 1 (
    echo QuackProxy service not registered - nothing to do.
    goto :remove_firewall
)

echo Stopping QuackProxy service...
"%NSSM%" stop QuackProxy >nul 2>&1
timeout /t 3 /nobreak >nul

echo Removing QuackProxy service registration...
"%NSSM%" remove QuackProxy confirm >nul 2>&1
if errorlevel 1 (
    echo WARNING: nssm remove failed - the registration may need manual removal.
)

:remove_firewall
echo Removing firewall rules...
netsh advfirewall firewall delete rule name="Quack-Proxy" >nul 2>&1
netsh advfirewall firewall delete rule name="Quack-Proxy Admin" >nul 2>&1

echo QuackProxy service removed.
endlocal
