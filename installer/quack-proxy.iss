; Quack-Proxy Windows installer (public go-quack-proxy fork)
;
; Build with: iscc installer/quack-proxy.iss
; Output: windows/quack-proxy-installer-{#MyAppVersion}.exe
;
; Option B: quack-proxy links libduckdb 1.5.5 in-process via CGo
; (duckdb-go/v2) and runs quack_serve() inside its own process. No duckdb.exe
; CLI is shipped or spawned — the engine is compiled into quack-proxy.exe.
;
; The quack-proxy binary is built in place from this repository
; (Z:\go-quack-proxy via the shared-folder mount) by scripts/build-windows.bat
; into windows\quack-proxy.exe. NSSM (nssm.exe) is downloaded separately
; and staged under dist\nssm\.
;
; Requires the mingw-w64 14.2.0 C toolchain (scripts/install-mingw.ps1) —
; the prebuilt duckdb static libraries only link against that exact toolchain
; (docs/windows-toolchain.md).
;
; Network defaults: install-service.bat opens the first shard port (9491)
; and the admin port (9490) in Windows Firewall — a single-shard setup.
; Multi-shard deployments must open additional ports manually.

#define MyAppName "Quack-Proxy"
#define MyAppVersion "0.3.0"
#define MyAppPublisher "Ron OHara"
#define MyAppURL "https://workbench-win.sentuny.com"

[Setup]
AppId={{2572D829-2907-486B-A42F-1FF9F432A8B9}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
DefaultDirName={autopf}\QuackProxy
DefaultGroupName=Quack-Proxy
AllowNoIcons=yes
LicenseFile=..\LICENSE
OutputDir=..\windows
OutputBaseFilename=quack-proxy-installer-{#MyAppVersion}
Compression=lzma2/max
SolidCompression=yes
WizardStyle=modern
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayName={#MyAppName} {#MyAppVersion}
PrivilegesRequired=admin
DisableProgramGroupPage=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Files]
; -- Quack-Proxy binary (built and staged by scripts/build-windows.bat) --
Source: "..\windows\quack-proxy.exe"; DestDir: "{app}\quack-proxy"; Flags: ignoreversion

; -- Proxy config template (installed as quack-proxy.yaml, the DEFAULT config
;    name the proxy resolves next to its own exe — the service runs with NO
;    config path argument, so NSSM cannot mangle it) --
Source: "..\dist\config\quack-proxy.example.yaml"; DestDir: "{app}\quack-proxy"; DestName: "quack-proxy.yaml"; Flags: ignoreversion

; -- Service scripts --
Source: "..\service\install-service.bat"; DestDir: "{app}\service"; Flags: ignoreversion
Source: "..\service\uninstall-service.bat"; DestDir: "{app}\service"; Flags: ignoreversion

; -- NSSM service wrapper (console apps can't run under SCM directly) --
Source: "..\dist\nssm\nssm.exe"; DestDir: "{app}\service"; Flags: ignoreversion

; -- Licence --
Source: "..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[UninstallRun]
; Stop and remove the quack-proxy service before files are deleted.
; shellexec is REQUIRED: CreateProcess cannot launch .bat files.
; waituntilterminated is REQUIRED: the uninstaller must finish stopping the
; service BEFORE deleting files, or the running service locks its exes and
; the uninstall reports "some elements could not be removed".
Filename: "{app}\service\uninstall-service.bat"; Flags: runhidden shellexec waituntilterminated; RunOnceId: "StopQuackProxyService"

[UninstallDelete]
; Runtime debris the uninstaller does not track (created after install):
; - data\: the database files and NSSM stdout/stderr logs
; - quack-proxy\run\: the proxy PID file
Type: filesandordirs; Name: "{app}\data"
Type: filesandordirs; Name: "{app}\quack-proxy\run"

[Run]
; Register quack-proxy as a Windows service with auto-start.
; UNCONDITIONAL (service registration is the deployment model, not an option).
; shellexec is REQUIRED: CreateProcess cannot launch .bat files.
; waituntilterminated: the installer waits for the service to come up before
; reporting completion.
Filename: "{app}\service\install-service.bat"; Flags: runhidden shellexec waituntilterminated
