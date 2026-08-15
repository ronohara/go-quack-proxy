# Quack-Proxy `status` Enhancement — Implementation Plan

Item-by-item work plan for the admin HTTP endpoint and remote-capable CLI. Ordered for implementation, with acceptance criteria per item.

> **Source.** Derived from `quack-proxy-enhancement-design.html` (2026-08-14). Every item below maps to a section of that design — §2 architecture, §3 config, §4 NSSM, §5 security, §6 file changes, §7 verification, §8 exclusions.

## Order of Work

Items are sequential where they touch the same code, parallelisable where they don't. **1 → 2 → 3** must precede **4**; **5** and **6** are independent of each other; **7–12** are verification, in the order given. Revision items **13–16** implement and verify design §2.3, then the Windows items **17–18** (added 2026-08-15 after Item 12's live findings).

Status legend: **DONE** / **IN PROGRESS** / **PENDING**

## Implementation Items

### ITEM 0 — Fork provenance — verify fork-changelog.md is current — DONE

The design requires recording the fork point and our divergence before shipping (§"Upstream Status": treat the code as our own; nothing further will arrive from upstream). The `go-quack-proxy` repo already carries `fork-changelog.md` with a v0.2.0 (Option B) section.

- **Files:** `fork-changelog.md` (go-quack-proxy)
- **Accept:** changelog states the fork point (upstream `e05cc4f`), lists Option B, `init-db`, NSSM service pattern, config-path fix — and after this feature lands, the status-enhancement entry.

### ITEM 1 — AdminConfig in the config package — DONE

Add `AdminConfig` to `internal/config/config.go`: `enabled` (default `true`), `bind_host` (default `127.0.0.1`), `port` (default `9490`). Same load/defaults pattern as `Listener`; wire into `Config`; strict parsing of unknown fields unchanged. The `/status` path is fixed, not configurable in v1.

- **Files:** `internal/config/config.go` (+ existing config tests)
- **Accept:** defaults applied when the `admin:` section is absent; explicit values override; unknown admin fields still rejected.

### ITEM 2 — Admin HTTP server — internal/admin package (new) — DONE

New `internal/admin/admin.go`: a small `net/http` server sharing the supervisor instance. `GET /status` builds the JSON snapshot: `version`, `pid`, `started_at`, `shards[]` (`name`, `port`, `status`, `start_time`, `uptime_seconds`, `restarts`, `database`, `token_set`), and `attach_sql`. Graceful shutdown on context cancel.

- **Files:** `internal/admin/admin.go` (new)
- **Accept:** handler returns the documented JSON shape; token appears **never** — only `token_set: true/false`; unknown paths → 404.

### ITEM 3 — Admin package tests (new) — DONE

`internal/admin/admin_test.go`: JSON-shape assertions against a supervisor with injected shard state; assert the raw token is absent from the serialised body; 404 on unknown paths.

- **Files:** `internal/admin/admin_test.go` (new)
- **Accept:** `go test ./internal/admin/` green, including the token-masking assertion.

### ITEM 4 — Wire the admin server into runStart — DONE

`cmd/main.go` — `runStart` constructs the admin server with the supervisor instance and starts it in a goroutine; it shuts down with the process context. No NSSM changes, no firewall changes — the listener lives inside the service process, loopback-bound by default.

- **Files:** `cmd/main.go`
- **Accept:** starting the proxy (Option B in-process) also listens on `127.0.0.1:9490`; stopping the proxy releases the port.

### ITEM 5 — CLI status becomes an HTTP client — DONE

`runStatus` replaces the local-supervisor logic (the always-empty-table bug) with an HTTP GET to `admin_host:admin_port/status`. Render the existing table from the remote JSON; `--json` passes the remote payload through verbatim. Error cases: connection refused → `quack-proxy is not running`, exit 1; HTTP 404 (old proxy build) → `this proxy build does not expose /status`. Zero-config: with no `-c` and no default config file, the CLI probes the built-in admin defaults (`127.0.0.1:9490`); an explicit missing config still errors.

- **Files:** `cmd/main.go`
- **Accept:** table rendering and `--json` shape visually identical to today's output, but populated from the live remote process; both error cases produce the documented messages and exit codes.

### ITEM 6 — Token masking backport to local --json — DONE

**Superseded by Item 5.** The local `--json` serialisation path this item targeted no longer exists — `runStatus` is now an HTTP client, and the only JSON serialiser left in the codebase is the admin endpoint, which already masks. Verified by grep: no `ShardProcess` marshalling, no `StatusFile` writer, no token json tag outside `token_set`.

- **Files:** none (no code change required)
- **Accept:** no path serialises the raw token; admin `/status` emits `token_set` only (already covered by Items 2/3 tests).

### ITEM 7 — Example config — document the admin section — DONE

`quack-proxy.example.yaml` gains the `admin:` section with commented defaults (`enabled: true`, `bind_host: 127.0.0.1`, `port: 9490`) and a note that the endpoint is read-only status (loopback is convention, widening is safe where remote status is wanted).

- **Files:** `quack-proxy.example.yaml`
- **Accept:** fresh-config users can discover the admin section; the loopback default and the status-only note are visible in the file.

### ITEM 8 — Fork-changelog entry — DONE

After Items 1–7 land, record the status enhancement in `fork-changelog.md` (completing Item 0's divergence list: Option B, init-db, NSSM pattern, config-path fix, status enhancement).

- **Files:** `fork-changelog.md` (go-quack-proxy)
- **Accept:** the divergence list mentions the admin endpoint + remote status.

## Verification Items (design §7)

> **Staging — the two test phases are mutually exclusive on the shared ports.** The VBox NAT forwards claim x360 `localhost` ports (9432, 1433, **9490**, 9491, 8010, 9422) whenever the W11Pro Clone VM is booted. Quack-proxy cannot run on both systems at the same time through those ports: with w11pro booted, an x360-local quack-proxy cannot bind 9490 and the x360 tests fail. Therefore:
>
> **Phase A (Items 9–12, x360)** — the w11pro VM must be **shut down** (not suspended — NAT forwards stay bound), leaving x360's localhost ports free for the local proxy. Item 9 (build/vet) is port-independent but runs in the same phase.
>
> **Phase B (Items 17–18, Windows)** — w11pro booted, quack-proxy installed and running as the NSSM service, the 9490 forward active, and **no** local x360 quack-proxy running.

### ITEM 9 — x360 build + vet — DONE

`go build ./...` with CGo (duckdb-go/v2) and `go vet ./...` in the go-quack-proxy repo.

- **Accept:** clean build, clean vet.

### ITEM 10 — Local end-to-end (two shells) — DONE

Start the proxy with the e2e config (a 9492 shard). From a **second** shell: `curl http://127.0.0.1:9490/status` returns JSON with `status: "healthy"`; `quack-proxy status -c config` renders the table — proving the data is remote, not a fresh local supervisor.

- **Stage:** Phase A — w11pro VM shut down so x360 localhost:9490 (and the 9491/8010 forwards) are free.
- **Accept:** curl JSON healthy; CLI table non-empty and matching the curl payload.

### ITEM 11 — Token-masking assertion — DONE

Grep both the admin JSON and the local `--json` output: `token_set` present, raw token absent.

- **Stage:** Phase A — same running local proxy as Item 10.
- **Accept:** zero token occurrences in either output.

### ITEM 12 — Crash-loop scenario — DONE

**Executed 2026-08-15 — exposed a design gap.** With the shard port held, `StartAll` fails fatally and the proxy exits before the admin endpoint comes up (`status` → "not running"). The restart counter also cannot climb (fresh-entry reset on success; stopped-skip on failure). The original acceptance was unachievable as built. Superseded by design §2.3; the revised crash-loop verification is Items 16–18.

- **Stage:** Phase A — executed against the live proxy; environment restored afterwards.
- **Outcome:** finding recorded; behaviour rework tracked by Items 13–15, verification by Items 16–18.

## Revision Items (design §2.3)

> **Source.** Design §2.3 "Crash-Loop Reality and the Restart Model" (2026-08-15), written after Item 12's live verification contradicted the original §2 promise. These items implement and verify the revised model.

### ITEM 13 — Supervisor — non-fatal start + accumulating restarts — DONE

`StartAll` attempts every shard; a failing shard is logged and marked `unhealthy` with its `Error` recorded, while the remaining shards and the admin endpoint still come up. Health-loop restarts update the existing `ShardProcess` **in place** — `Restarts` preserved and incremented, fresh `StartTime` — and a failed restart leaves the shard `unhealthy` in the retry set (never the skip-forever `stopped`).

- **Files:** `internal/supervisor/supervisor.go`
- **Accept:** one held port no longer kills the proxy or admin endpoint; `status` shows the shard `unhealthy`; `restarts` climbs across attempts.

### ITEM 14 — Backoff and give-up threshold — DONE

Per-shard `nextRetry` with exponential backoff (1s → 2s → 4s → … → 30s cap, reset on success); healthy shards keep the 5s health cadence. After 10 consecutive failures the shard is marked `stopped` with its last error and retries cease; SIGHUP reload re-arms it.

- **Files:** `internal/supervisor/supervisor.go`
- **Accept:** backoff visibly lengthens between attempts; give-up fires at N; reload re-arms.

### ITEM 15 — Revision tests (supervisor + admin) — DONE

`supervisor_test.go`: bad-shard isolation, climbing `restarts`, backoff timing, give-up, recovery on port release. `admin.go` exposes the shard `error` field in the snapshot; `admin_test.go` asserts the new shape.

- **Files:** `internal/supervisor/supervisor_test.go`, `internal/admin/admin.go`, `internal/admin/admin_test.go`
- **Accept:** `go test ./internal/supervisor ./internal/admin` green including the new cases; token masking still enforced.

### ITEM 16 — Revised crash-loop verification (design §2.3.7) — DONE

**Executed 2026-08-15 (Phase A).** RUN A passed — held port → proxy + admin up, shard `unhealthy` + `error`, `restarts` climbing, release → `healthy`. RUN B passed — hold past threshold → `stopped` with last error. SIGHUP re-arm verified by unit test (`TestReArm`); the live re-arm check (RUN C) is dropped.

Hold the shard port; start the proxy → proxy + admin stay up, shard `unhealthy` with `error` set. Sample `status` twice → `restarts` climbing, backoff lengthening. Release the port → recovery to `healthy`. Hold past the threshold → `stopped` with last error, no further attempts.

- **Stage:** Phase A — same conditions as Item 10.
- **Accept:** all four §2.3.7 steps observed.

### ITEM 17 — Windows verification through the forward — DONE

Rebuild with the mingw 14.2.0 toolchain (see windows-toolchain.md), reinstall via the Experiment 1 installer on w11pro, forward the admin port, and run `quack-proxy status` **from x360** against the NSSM service — the original requirement.

- **Stage:** Phase B — w11pro booted, NSSM service running, x360→w11pro 9490 forward active, no local x360 proxy running.
- **Accept:** x360 shell shows live shard state of the w11pro NSSM service with no VM login; crash-loop visibility confirmed if re-provoked.

### ITEM 18 — Windows verification with the revised model — DONE

Re-run Item 17's flow, additionally confirming the revised crash-loop signal end-to-end: with the w11pro shard port blocked, the NSSM service's `status` from x360 shows `unhealthy` + `error` and climbing `restarts`, then recovery on release.

- **Stage:** Phase B — w11pro booted, NSSM service running, 9490 forward active, no local x360 proxy.
- **Accept:** remote status shows the revised crash-loop signal end-to-end through the forward.

> **Non-goals (design §8) — deliberately not in this plan:** remote stop/restart/reload (needs auth — future design), per-shard metrics history (logs remain the forensic source), authentication on the admin endpoint (read-only status makes it unnecessary), and any change to `stop` (stays PID-file + SIGTERM, local by design).

---

Quack-Proxy Status Enhancement — Implementation Plan · Copyright © 2026 Ron O'Hara
