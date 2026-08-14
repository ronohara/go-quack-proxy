# Windows Toolchain for the quack-proxy Build

How the correct C toolchain for the Windows build of quack-proxy was discovered, why every other option failed, and what the build environment requires.

## Why a specific toolchain matters

Option B quack-proxy links **libduckdb in-process** via CGo (`duckdb-go/v2`). The Go module ships **prebuilt** static archives in `duckdb-go-bindings/lib/windows-amd64` — the Windows build never compiles DuckDB C++; it only *links* those archives.

That is the trap: prebuilt archives carry the expectations of the exact compiler that produced them. Get the toolchain wrong and the final CGo link fails with unexplained symbol errors.

## The experiments, in order

### 1. w64devkit (gcc 16.2) — failed

- Candidate considered alongside winlibs; w64devkit was chosen first.
- Staged from `w64devkit-x64-2.9.1.7z.exe` into `build\toolchain\w64devkit`.
- Immediate detour: gcc's own-prefix resolution broke on the mapped-drive path (`Z:\`), so a bare smoke test failed even though `as.exe` and the linker tools were present. Prepend the toolchain `bin` to PATH (the real build condition) and it progressed — but only to the next failure.
- Final CGo link failed: `link.exe: running gcc failed: exit status 1`.
- Root cause: w64devkit is **MSVCRT-based with a patched CRT/runtime**. It fails on the UCRT symbols in the prebuilt archives (`__stdio_common_vsnprintf_s`) plus the same libstdc++ internals that would later doom MSYS2.

### 2. winlibs — considered, not pursued

- Mentioned as the alternative to w64devkit; dropped once MSYS2 looked like the upstream-tested path (below).

### 3. MSYS2 ucrt64 gcc 16.2 — failed

- Rationale looked solid: the duckdb bindings' own CI (`run_tests.yml`) links these exact `.a` files on GitHub's `windows-latest` runners, which ship **MSYS2 with a ucrt64 gcc** — an upstream-tested configuration.
- Install: MSYS2 GUI installer → `pacman -Syu` → `pacman -S mingw-w64-ucrt-x86_64-gcc` → gcc at `C:\msys64\ucrt64\bin`.
- Result: links **everything except two libstdc++ internals**:
  - `__emutls_v._ZSt11__once_call`
  - `__emutls_v._ZSt15__once_callable`
- Decisive finding: those symbols exist **nowhere** in the installed `libstdc++.a` or `libgcc.a`. Modern libstdc++ removed `std::__once_call` / `__once_callable` entirely (call_once was reimplemented). DuckDB's prebuilt archives were compiled against an older libstdc++ and carry *inlined* code that reads those TLS variables — so **no modern toolchain can ever link the prebuilt archives**.
- Moment of truth: "toolchain shopping is exhausted — recompiling duckdb is the only path." (Recompiling duckdb v1.5.5 from source is a legitimate but heavy route: ~1 GB clone, 30–90+ minute unity build, exact build-recipe reproduction.)

### 4. The breakthrough — Chocolatey mingw 14.2.0: SUCCESS

- Before committing to the recompile, the archive provenance was checked: duckdb's CI (`BundleStaticLibs.yml`) builds the windows-amd64 archives with:
  - `choco install mingw --version 14.2.0`
  - niXman mingw-builds, `x86_64-14.2.0-release-posix-seh-ucrt` (UCRT runtime, posix threads, seh exceptions)
- gcc 14.x's libstdc++ **still carries the `__once_call` symbols** — it is old enough to satisfy the archives and new enough to match their UCRT build.
- Conclusion: no recompile needed. Install the **same toolchain the CI used**.

## The final toolchain

- gcc location: `C:\ProgramData\mingw64\mingw64\bin\gcc.exe` — gcc 14.2.0, `x86_64-posix-seh-rev0`
- Installer: `scripts/install-mingw.ps1` (elevated PowerShell; installs Chocolatey if missing, then `choco install mingw --version 14.2.0`)
- Build controls (`build-quack-proxy.bat` and the backend equivalent) prepend that directory to PATH **for the session only** — no permanent environment change.

## Lessons and requirements

- **Exact version required.** No other compiler links the prebuilt archives: MSYS2 ucrt64 gcc 16.2 and w64devkit both fail at the final link for the reasons above.
- **ASCII + CRLF for all `.bat`/`.ps1` files.** PowerShell 5.1 mis-parses LF-only scripts with chained operators (install-mingw.ps1 hit this: `Unexpected token 'Path", "User")`), and cmd displays UTF-8 dashes as mojibake.
- **Mapped-drive caveat.** GCC prefix resolution can misbehave on network-mapped paths (`Z:\`); the chosen toolchain lives on `C:\ProgramData`, away from that problem.
- **Verified 2026-08-13:** quack-proxy.exe built (81.9 MB, libduckdb 1.5.5 in-process) with this toolchain; `quack-proxy version` responds; the Experiment 1 installer compiles clean.
