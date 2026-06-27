# quack-proxy Changelog

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
