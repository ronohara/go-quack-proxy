# Quack-Proxy `status` Enhancement — Design

Making `quack-proxy status` a real remote probe: an admin HTTP endpoint in the proxy plus a CLI that reports live shard state over TCP.

> **Origin.** 2026-08-14, during Windows service testing of the in-process (Option B) quack-proxy. Requirement: from x360, an operator must be able to see the real runtime state of the proxy running as an NSSM service on w11pro — through the forwarded port, not by logging into the VM.

## 1. Problem Statement

Today the CLI subcommands are **local-process tools, not TCP clients**:

| Command | Mechanism | Consequence |
|---|---|---|
| `status` | Loads config file; creates a *fresh* `Supervisor`; prints **its own** (empty) shards map — `sup.Status()` | Always prints an empty table. It has never shown the running service's state — decorative in every test so far. |
| `stop` | Reads PID file, sends SIGTERM to that local PID | Local only. On a remote machine: "not running (no PID file)". |

The only TCP surface quack-proxy exposes today is the **quack server's HTTP endpoint** per shard (`/` health, `/quack` protocol) — used by backend clients and by the supervisor's own health loop (`health.Check` = HTTP GET, 2xx = healthy). Nothing exposes the proxy's *internal* state: uptime, restart count, per-shard status string, config details.

In the Windows environment this gap is acute:

- The proxy runs under **NSSM** — its console output is captured to rotated log files; `sc query`/NSSM show only SCM state (RUNNING/PAUSED), never shard health.
- The crash-loop we debugged (config path mangled by NSSM, restarting every ~6s) was only visible by pulling the rotated log files from the VM. A remote status would have shown `restarts` climbing immediately.
- The blended-environment model (x360 tooling driving w11pro services through port forwards — same pattern as the MS Access 9432 and MSSQL 1433 endpoints) assumes every service is inspectable over TCP from x360.

## Upstream Status — We Are the Windows Implementation

> **Findings, 2026-08-14 (GitHub evidence).** The gaps in this document are not accidents of incomplete testing — they exist because **nobody has ever run quack-proxy on Windows before**. The upstream repository confirms it.

| Evidence | Value | Meaning |
|---|---|---|
| Repo created / last pushed | 2026-05-29 / 2026-05-29 | One-shot upload — zero commits since creation. Effectively frozen upstream. |
| Stars / forks / open issues | 4 / 1 / 0 | No user base, no issue traffic. |
| Windows-related issues ever filed | 0 | No one has attempted a Windows deployment before us. |
| Service-related issues ever filed | 0 | Same for service/daemon use. |
| License | MIT | Free to fork, modify, redistribute — our changes are legally ours. |

**The reassuring architectural fact:** quack-proxy is *not* the quack protocol. The quack server — `quack_serve()` — lives inside DuckDB itself (libduckdb 1.5.5) and is maintained by the DuckDB project. quack-proxy is only a thin orchestration wrapper: config loading, supervisor, health loop, CLI. With Option B, we have already replaced the hardest part of that wrapper — the process-spawn mechanism — with direct in-process CGo calls. Our real dependency is DuckDB upstream, which is active and healthy; the dormant part is precisely the part we now control.

**Consequences:**

- Treat the quack-proxy code as our own — nothing further will arrive from upstream.
- Fix provenance before shipping: record the fork point and our divergence (Option B, init-db, NSSM service pattern, this status enhancement, the config-path fix) in a fork-notes document.
- This design document, together with the Option B architecture, constitutes the Windows implementation of this stack — there is no prior art to fall back on, and no one to ask.

## 2. Proposed Architecture

```
quack-proxy.exe (w11pro, NSSM service)
 │
 ├── supervisor (existing)          ── in-memory shard state: Status/Uptime/Restarts
 │     └── in-process quack servers (libduckdb 1.5.5, Option B)
 │
 ├── admin HTTP listener (NEW)      ── 127.0.0.1:9490 (default)
 │     └── GET /status  →  JSON snapshot of supervisor.Status() + proxy metadata
 │
 └── CLI (any machine: x360, w11pro)
       └── quack-proxy status -c config.yaml
             → HTTP GET http://admin_host:admin_port/status
             → renders the familiar table (or --json passthrough)

 x360 shell ──(port forward 9490)──▶ w11pro admin listener
```

### 2.1 Admin endpoint (inside the proxy)

A small `net/http` server started by `runStart`, sharing the supervisor instance. Read-only by design. Default bind `127.0.0.1` — reachable remotely only via an explicit port forward or an operator-chosen bind override.

**Response — `GET /status`**

```json
{
   "version": "0.1.0",
   "pid": 1234,
   "started_at": "2026-08-14T09:15:00+07:00",
   "shards": [
     {
       "name": "analytics",
       "port": 9491,
       "status": "healthy",          // starting | healthy | unhealthy | stopped
       "start_time": "2026-08-14T09:15:01+07:00",
       "uptime_seconds": 43210,
       "restarts": 0,
       "database": "C:\\Program Files\\Sentuny\\...\\data\\workbench.duckdb",
       "token_set": true              // NEVER the token itself
     }
   ],
   "attach_sql": "ATTACH 'quack:0.0.0.0:9491' AS analytics;\n"
}
```

> **Token masking is mandatory.** The in-process `status --json` today serialises the full `ShardProcess`, *including* `Config.Token`. The admin endpoint must emit only `token_set: true/false`. (The existing `--json` path should get the same treatment while we're in there.)

### 2.2 CLI `status` becomes an HTTP client

| Case | Behavior |
|---|---|
| GET succeeds | Render the existing table format from the remote JSON; `--json` passes the remote payload through verbatim. |
| Connection refused | `quack-proxy is not running` (proxy down, or admin disabled) — exit 1. Distinct from shard-unhealthy, which is visible per-shard in the table. |
| Old proxy build (no admin endpoint) | HTTP 404 → clear error: this proxy build does not expose /status. |
| No `-c` given, no default config file | Probe the built-in admin defaults (`127.0.0.1:9490`). An explicit `-c` pointing at a missing file remains a hard error. |

Table rendering and the `--json` shape stay visually identical to today's output, so existing habits/scripts keep working — but the data now comes from the live process.

## 2.3 Revision — Crash-Loop Reality and the Restart Model (2026-08-15)

> **Live verification (plan Item 12) contradicted the original §2 promise.** With a shard port held: the bind failure is fatal in `StartAll` → the whole proxy process exits → the admin endpoint never comes up, and `status` reports "not running" for the duration. Additionally, the in-process restart counter cannot climb: a successful restart replaces the shard with a fresh entry (`Restarts: 0`), and a failed restart leaves the shard `stopped`, which the health loop skips forever. "Restarts climbing" is therefore unachievable as built. This section revises the model.

### 2.3.1 Per-shard non-fatal start

`StartAll` attempts every shard. A shard that fails to start is logged and marked `unhealthy` with its error recorded; the remaining shards start, and the proxy + admin endpoint always come up. One bad shard can no longer take the admin endpoint (and every other shard) down with it.

### 2.3.2 Restart accounting that accumulates

Restarts are carried across attempts: on a health-loop restart the existing `ShardProcess` is updated **in place** — `Status: "starting"`, fresh `StartTime`, `Restarts` preserved and incremented — instead of being replaced by a zero-valued entry. A failed restart sets `Status: "unhealthy"` (not `stopped`) and leaves the shard in the retry set.

### 2.3.3 Backoff and give-up

Restart attempts per shard follow exponential backoff (1s → 2s → 4s → … → 30s cap, reset on success), tracked per shard with a `nextRetry` timestamp; healthy shards keep the normal 5s health cadence. After `N` consecutive failures (10), the shard is marked `stopped` with its last error and retries cease — visible in status; SIGHUP reload re-arms it.

### 2.3.4 Admin JSON additions

```json
{
   "name": "analytics",
   "port": 9491,
   "status": "unhealthy",
   "error": "Failed to bind DuckDB Quack RPC server to http://127.0.0.1:9491 (address in use)",
   "restarts": 4,
   ...
}
```

New `error` field carries the last start failure; token-masking rules are unchanged. (Optionally `next_retry_at` for observability.)

### 2.3.5 Process-level crash-loop stays the outer net

systemd/NSSM continue to restart the whole process when the proxy itself dies; during that window `status` reports `quack-proxy is not running` — the documented proxy-down signal, distinct from per-shard `unhealthy`.

### 2.3.6 File changes (revision)

| File | Change |
|---|---|
| `internal/supervisor/supervisor.go` | Non-fatal `StartAll`; in-place restart with accumulating `Restarts`; per-shard backoff (`nextRetry`); give-up threshold; `Error` field on `ShardProcess`. |
| `internal/supervisor/supervisor_test.go` | Tests: one bad shard doesn't stop the others or the admin; `restarts` climbs across attempts; backoff timing; give-up after N; recovery on port release. |
| `internal/admin/admin.go` | Expose `error` (last failure) in the shard snapshot. |
| `internal/admin/admin_test.go` | Shape assertions for the new field. |

### 2.3.7 Verification (replaces plan Item 12)

1. Hold the shard port; start the proxy → proxy + admin stay up; `status` shows the shard `unhealthy` with `error` set; other shards healthy.
2. Two successive `status` snapshots show `restarts` climbing and backoff lengthening between attempts.
3. Release the port → the next retry succeeds; the shard returns to `healthy`; `restarts` stops climbing.
4. Leave the port held past the threshold → the shard is marked `stopped` with its last error; no further attempts until re-arm.

## 3. Config Changes

```yaml
# quack-proxy.yaml — new top-level section (all fields optional)
admin:
  enabled: true          # default true
  bind_host: 127.0.0.1   # default 127.0.0.1 — local-only unless overridden
  port: 9490             # default 9490 (quack shards start at 9491)
  path: /status          # fixed; not user-configurable in v1
```

`config.go` gains an `AdminConfig` struct with the same load/defaults pattern as `Listener`. Unknown fields keep the existing strict-parse behavior.

## 4. Windows / NSSM Integration

- **No NSSM changes.** The admin listener runs in the same process as the service; NSSM needs no new parameters.
- **Firewall.** The installer opens 9491 (first shard) and 9490 (admin) in Windows Firewall — the default install network setup is for a **single shard**; additional shards need their ports opened manually. Remote access to the admin port is via the operator's existing forward mechanism (VBox NAT / SSH -L), mirroring 9491/8000/8010.
- **Crash-loop detection becomes instant.** `restarts` climbing while `status` stays `starting`/`unhealthy` is the signature of exactly the failure modes we hit (config-path mangling, stdin refusal era).
- **Bind widening.** An operator may set `admin.bind_host: 0.0.0.0` for direct LAN access — safe because the endpoint serves read-only status only (see §5).

## 5. Security Notes

- **Read-only endpoint.** No stop/restart/config actions over the admin API — remote control is deliberately excluded (a remote stop would need authentication, which is out of scope).
- **No authentication.** The endpoint serves read-only status only — no tokens, no credentials, no control actions — so the loopback default is a convention, not a security control. Widening the bind exposes only shard names, database paths, and the attach SQL.
- **Token masking** is enforced in the admin handler AND backported to the local `--json` output.

## 6. File Changes (quack-proxy repo)

| File | Change |
|---|---|
| `internal/config/config.go` | Add `AdminConfig` (enabled, bind_host, port) + defaults + YAML tags; wire into `Config`. |
| `internal/admin/admin.go` *(new)* | HTTP server; `GET /status` handler building the JSON snapshot from the supervisor; token masking; graceful shutdown on context cancel. |
| `internal/admin/admin_test.go` *(new)* | JSON shape assertions; token never appears in the body; 404 on unknown paths. |
| `cmd/main.go` | `runStart`: construct the admin server with the supervisor instance, start it in a goroutine; `runStatus`: replace local-supervisor logic with the HTTP client described in §2.2. |
| `quack-proxy.example.yaml` | Document the `admin:` section (commented defaults). |

## 7. Verification Plan

1. **x360 build** — `go build ./cmd/...` with CGo (duckdb-go/v2); `go vet ./...`.
2. **Local E2E** — start the proxy with the e2e config (port 9492 shard); `curl http://127.0.0.1:9490/status` returns JSON with `status: "healthy"`; `quack-proxy status -c config` renders the table from a *second* shell (separate process — proving it's remote data).
3. **Token masking** — assert `token_set` present, raw token absent (grep the JSON).
4. **Crash-loop scenario** — kill the in-process shard's database file access or force an unhealthy shard; confirm `restarts` increments across successive status calls while status shows `unhealthy`/`starting`.
5. **Windows** — rebuild with the mingw 14.2.0 toolchain, reinstall via the Experiment 1 installer, run `quack-proxy status` from x360 through the forwarded admin port against the NSSM service.

## 8. Conscious Exclusions

- No remote stop/restart/reload (auth would be required — separate future design if ever needed).
- No per-shard metrics history (restart timestamps, event log) — a snapshot is enough for v1; the existing rotated logs remain the forensic source.
- No auth on the admin endpoint in v1 (loopback bind compensates).
- `stop` remains PID-file + SIGTERM (local by design — remote stop is exactly the capability we chose not to add).

---

Quack-Proxy Status Enhancement — Design · Copyright © 2026 Ron O'Hara
