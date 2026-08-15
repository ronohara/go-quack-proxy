# quack-proxy Changelog

> **Fork provenance.** This repository is a fork of
> [alitrack/quack-proxy](https://github.com/alitrack/quack-proxy) (MIT).
> Upstream is frozen: a one-shot upload on 2026-05-29, final upstream commit
> `e05cc4f`, zero commits since. Everything after that point is this fork's
> own work. Divergence from upstream:
>
> - v0.1.1 (2026-06-27): logging levels, base-directory awareness, path
>   resolution
> - v0.2.0 (2026-08-15): Option B — in-process DuckDB engine via CGo, new
>   `init-db` subcommand (see the v0.2.0 section below)
> - NSSM service deployment pattern: the service runs with **no config path
>   argument** so NSSM cannot mangle it; the proxy resolves `quack-proxy.yaml`
>   next to its own executable
> - Status enhancement (admin HTTP endpoint + remote-capable CLI `status`) —
>   implemented (v0.3.0; verification pending — see the plan's Items 9–13)

## v0.3.0 – Status enhancement (implementation landed; verification pending)

### Added
- Admin HTTP endpoint (`GET /status`, default `127.0.0.1:9490`) exposing a live
  JSON snapshot of the running proxy: `version`, `pid`, `started_at`,
  per-shard status/uptime/restarts with token masking, and `attach_sql`
  (new `internal/admin` package; the token is never serialised — `token_set` only)
- CLI `status` reworked as an HTTP client against the admin endpoint — table
  rendering unchanged, `--json` passes the remote payload through verbatim;
  error cases: `quack-proxy is not running` (connection refused, exit 1) and
  `this proxy build does not expose /status` (HTTP 404 from an older build)
- Zero-config `status`: with no `-c` given and no default config file next to
  the executable, the CLI probes the built-in admin defaults
  (`127.0.0.1:9490`); an explicitly-given missing config is still an error
- `admin:` config section (`enabled`, `bind_host`, `port`) with defaults
  (true / `127.0.0.1` / `9490`), wired in `internal/config` and documented in
  `quack-proxy.example.yaml` (unauthenticated endpoint — widening the bind
  is dev-only)
- Admin package tests: JSON shape, token masking, 404 on unknown paths, 405
  on non-GET (4 tests)

### Known Issues
- Loopback port-squatter edge: a specific-address listener on a shard's port
  (e.g. `127.0.0.1:9491`) makes restarts *succeed* while health checks keep
  failing — `restarts` climbs indefinitely and the give-up threshold (keyed
  to failed restarts) never fires. Observed on Windows, where SO_REUSEADDR
  permits the wildcard bind alongside the specific one. The climbing
  `restarts` value remains the visible crash-loop signal.

## v0.2.0 – 2026-08-15 — Option B: in-process DuckDB engine

### Added
- `init-db` subcommand: creates missing shard database files and verifies
  existing ones are valid DuckDB databases, using the in-process engine
  (replaces the duckdb.exe CLI bootstrap — Option B has no CLI)
- In-process DuckDB engine via `github.com/duckdb/duckdb-go/v2` v2.10505.0
  (CGo-linked libduckdb)
- Automatic creation of missing parent directories for shard database files

### Changed
- Supervisor architecture: each shard now runs the Quack server INSIDE the
  quack-proxy process via `CALL quack_serve(...)` — no `duckdb` CLI child
  process, no bash pipe, no stdin keep-alive
- Shard lifecycle: one `*sql.DB` handle per shard; closing the handle stops
  the Quack server (replaces SIGTERM/SIGKILL process management and PID
  tracking)
- Supervisor tests updated for the in-process engine (internal logger, `sp.db`
  instead of PID, no pre-created empty database files)
- `go.mod`/`go.sum`: added duckdb-go/v2 plus indirect dependencies
  (arrow-go, per-platform duckdb bindings)

### Removed
- `internal/supervisor/attr_unix.go` and `attr_windows.go` — child-process
  group attributes no longer needed
- All child-process management code (`exec.Cmd`, shell pipeline, process
  groups, PID tracking)
- Tracked runtime database files `config/quack_data.duckdb` and
  `config/quack_data.duckdb.wal` — untracked from version control; both
  (plus `AGENTS.md`) added to `.gitignore`

### Known Issues
- Test files in `cmd`, `internal/config`, and `internal/proxy` do not compile
  against the current APIs (pre-existing upstream drift at `ea3b0eb`;
  production code builds cleanly)

---

## v0.1.1 – 2026-06-27

### Added
- Base directory detection using `os.Executable()` – the binary now finds its own installation location
- Path resolution using Go's `filepath` package for cross-platform compatibility (Linux, macOS, Windows)
- Database validation on startup – clear error message if database file doesn't exist
- `--verbose` flag for detailed logging (shows base directory, config loading, shard details)
- `--debug` flag for debug logging (includes SQL queries and health check details)
- `--quiet` flag for minimal logging (errors only)
- `--log-file` flag to write logs to a file
- `--log-json` flag for JSON formatted logs
- `internal/logger` package with hierarchical logging levels
- Debug logging for SQL queries executed by DuckDB
- Verbose logging for process start/stop and health checks
- Health check debug logging showing URL and status

### Changed
- Removed `INSTALL quack FROM core_nightly` and `LOAD quack` from `quackBootSQL` – assumes DuckDB 1.5.3+ with built-in Quack
- Updated `quack-start.sh` script to remove `LOAD quack` and translate Chinese comments to English
- Improved error messages for missing database files
- Expanded logging to include timestamps on all messages
- Upgraded logging from `slog` to custom logger with level control
- Replaced `print()` statements with structured logging

### Fixed
- Flag ordering issue – documented that flags must be placed before subcommands
- SQL query logging now works correctly when `--debug` is enabled
- Health check logging now shows the full URL being checked
- Process start/stop logging now shows PID and status

### Removed
- `INSTALL quack FROM core_nightly` – no longer needed with DuckDB 1.5.3+
- `LOAD quack` – no longer needed with DuckDB 1.5.3+
- Dependency on nightly repository for Quack extension

### Documentation
- Updated README with new flags and usage examples
- Documented flag ordering requirement
- Added DuckDB version requirement (>= 1.5.3)

---

## v0.1.0 – 2026-06-24

### Added
- Initial release from upstream
- Process supervision for DuckDB Quack servers
- YAML configuration file support
- Health checking with automatic restart
- HAProxy configuration generation
- Signal-based reload (SIGHUP)
- Cross-platform support (Linux, macOS, Windows)
- Basic logging with `slog`

### Fixed
- `exec.CommandContext` child process killing issue

### Known Issues
- Flag ordering: flags must be placed before subcommands
- Logging was basic and lacked levels
