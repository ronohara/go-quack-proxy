# Development Environment Overview

Four systems support all development activity: one physical development machine, one public production server, and two Windows virtual machines hosted on the development machine.

The quack proxy updates flow from the the need to have it operate in the Windows environment.


## The Four Systems

| System | Platform | Role |
|---|---|---|
| `x360` | Linux Mint | Code development and testing — hosts all source repositories, the dev stack, and both Windows VMs |
| `a514` | Linux Mint | Public internet-facing production deployment and web hosting |
| `w11pro` | Windows 11 Pro (VM on x360) | Windows testing — MS Access endpoint, MSSQL, quack-proxy, Go backend builds, installer validation |
| `w11home` | Windows 11 Home (VM on x360) | Windows sample deployment — a clean Home install used to validate deployment on a typical end-user machine |

## x360 — Development

- Repositories under `/home/rono/Documents/`: `workbench-development` (main repo — Next.js 16 frontend + FastAPI backend), `workbench-go` (Go backend re-implementation), `workbench-win` (Windows distribution: installers, NSSM, comparative tests), `workbench-mac` (macOS distribution: installers, services, comparative tests), `go-quack-proxy` (this repo — public quack-proxy fork), `workbench-documents` (websites and docs), `msaccess-endpoint` (jet-server), `msaccess-replicator-ng`
- Backend: FastAPI/uvicorn on `localhost:8000`, PostgreSQL (asyncpg), DuckDB via quack-proxy in client-server mode (systemd service)
- Frontend: Next.js 16, verified with `pnpm build`
- VirtualBox hosts the **W11Pro Clone** and **w11home** VMs; NAT port forwards bridge x360 localhost ports into the VMs

## a514 — Production

- IP `5.133.177.203`, hostname `a514`; access via SSH by full hostname, never by IP
- Web stack: Nginx (80/443, SSL termination) → Apache (8080, static sites) and Next.js (`:3000`)
- Subdomains: [www.sentuny.com](https://www.sentuny.com/), [workbench.sentuny.com](https://workbench.sentuny.com/) (landing page), [workbench-demo.sentuny.com](https://workbench-demo.sentuny.com/) (live app), [workbench-docs.sentuny.com](https://workbench-docs.sentuny.com/), [replicator.sentuny.com](https://replicator.sentuny.com/), [replicator-ng.sentuny.com](https://replicator-ng.sentuny.com/) (Replicator-NG website)
- systemd services `workbench` and `workbench-backend`; code under `/opt/statisticians-workbench`
- Rule: nothing on a514 is modified without explicit, specific user confirmation — read-only checks only unless an action has been requested

## w11pro — Windows Testing VM

- `jet-server` — MS Access endpoint via PostgreSQL wire protocol (Rust), port 9432
- MSSQL Server — test instance on port 1433
- quack-proxy — NSSM-managed service on port 9491
- Workbench Go backend — comparative testing on port 8010
- Build tooling — Inno Setup 7 (`C:\Program Files\Inno Setup 7\iscc.exe`), Python 3.13, MSVC toolchain for Rust/CGo builds
- Shared folder — x360 `/home/rono/Documents` maps to `Z:\`, so x360 edits are visible immediately on w11pro

## w11home — Windows Sample Deployment VM

A clean Windows 11 Home installation with no development tooling. Used to validate that installers and deployment steps work on a vanilla Home system — the same experience a real customer would have. VM installs and service registration are performed by the user; x360-side work uses read-only checks and forwarded ports.

## Network Port Forwards (x360 localhost → VMs)

| x360 port | Forwarded to | Service |
|---|---|---|
| `9432` | w11pro:9432 | jet-server (MS Access PostgreSQL endpoint) |
| `1433` | w11pro:1433 | MSSQL Server |
| `9491` | w11pro:9491 | quack-proxy |
| `8010` | w11pro:8010 | Workbench Go backend (comparative testing) |
| `9422` | w11pro:22 | SSH (`ssh -p 9422 ronoh@127.0.0.1`) |

## Repositories

| Repository | Purpose |
|---|---|
| `workbench-development` | Main application — Next.js frontend, FastAPI backend, translations, licensing |
| `workbench-go` | Go re-implementation of the backend for cross-platform validation |
| `workbench-win` | Windows distribution — installers, services, cloned comparative test suite |
| `workbench-mac` | macOS distribution — installers, services, cloned comparative test suite |
| `go-quack-proxy` | Public quack-proxy fork — v0.2.0 Option B (in-process DuckDB engine) |
| `workbench-documents` | Websites and documentation |
| `msaccess-endpoint` | jet-server — Access databases exposed via the PostgreSQL wire protocol |
| `msaccess-replicator-ng` | Replicator-NG — Access replication engine, installer, and website ([replicator-ng.sentuny.com](https://replicator-ng.sentuny.com/)) |
