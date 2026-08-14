# quack-proxy PRD

> Product Requirements Document | v0.1 | 2026-05-29
> Author: alitrack

---

## 1. Problem Statement

### Background

DuckDB is a single-process architecture by default: only one process can write to a `.duckdb` file at a time. The Quack protocol shipped in DuckDB v1.5.2 natively solves multi-client concurrent writes, but DuckDB offers no built-in process management or service orchestration.

Today, anyone who wants to run a production-grade "Quack server cluster" (multiple DuckDB files, each exposing its own Quack port) has to hand-write systemd units, shell scripts, health checks, and load-balancer configuration. There is no turnkey tool.

### Core Pain Points

| Pain point | Status quo | Goal |
|------|------|------|
| Starting a Quack server | Hand-written shell script `duckdb -c "CALL quack_serve(...)"` | One YAML config, one command |
| Managing N DuckDB files | N systemd units | One daemon manages all processes |
| Health checks | Hand-written curl + cron | Built-in HTTP health check + auto-restart |
| Load balancing | Hand-written HAProxy/Nginx config | Auto-generated config + reload signal |
| Failure recovery | Manual restart | Crash auto-restart + exponential backoff |
| Cross-machine federation | Manually ATTACH each machine | Coordinator DuckDB one-command federation |

### Target Users

- Data analytics teams: partitioned DuckDB data across multiple machines that need a unified query entry point
- Small-scale SaaS: using DuckDB for embedded OLAP, needing multi-client concurrent writes
- duckdb_fdw users: needing Quack to solve concurrent-write problems across multiple PG backends
- Individual developers: wanting convenient management of Quack server processes during local development

---

## 2. Product Vision

**Turn any number of DuckDB database files into a highly-available Quack service cluster with one YAML file and one command.**

```
quack-proxy start  ← one command
    ↓
┌─────────────────────────────────────────┐
│  quack-proxy daemon                      │
│                                           │
│  ├── Process Supervisor                  │
│  │   ├── DuckDB #1 → Quack :9491        │
│  │   ├── DuckDB #2 → Quack :9492        │
│  │   └── DuckDB #N → Quack :949N        │
│  │                                       │
│  ├── Health Checker                      │
│  │   └── HTTP GET / → every 5s per endpoint │
│  │                                       │
│  ├── Config Generator                    │
│  │   └── → HAProxy/Nginx config file    │
│  │                                       │
│  └── Optional: Coordinator DuckDB        │
│      └── ATTACH all Quack endpoints      │
│          → unified query entry (VIP)     │
└─────────────────────────────────────────┘
```

---

## 3. Architecture Design

### 3.1 Logical Architecture

```
quack-proxy (Go binary)
    │
    ├── quack-proxy start     ← start the daemon
    ├── quack-proxy stop      ← stop the daemon
    ├── quack-proxy status    ← show status
    ├── quack-proxy reload    ← hot-reload configuration
    └── quack-proxy gen-proxy ← generate HAProxy config
```

### 3.2 Process Model

```
quack-proxy (main process, PID 1)
    ├── signal handler (SIGTERM, SIGHUP)
    ├── config watcher (inotify on config file)
    ├── health check loop (polls all endpoints every N seconds)
    │
    └── child processes (one goroutine per DuckDB instance)
        ├── duckdb -- Quack on :9491 ← /data/shard_1.db
        ├── duckdb -- Quack on :9492 ← /data/shard_2.db
        └── duckdb -- Quack on :9493 ← /data/shard_3.db
```

### 3.3 Lifecycle

```
START:
  1. Parse the YAML configuration
  2. Validate that all .duckdb files exist
  3. Start a duckdb child process for each shard
     - Generate a random token (if not specified)
     - duckdb -c "INSTALL quack; LOAD quack; CALL quack_serve('quack:0.0.0.0:PORT', token='TOKEN');"
  4. Wait for all processes to be ready (HTTP health check passes)
  5. Enter the run loop

RUNNING:
  1. HTTP GET all endpoints every N (configurable) seconds
  2. Unhealthy endpoint: kill child process → restart → exponential backoff
  3. Write status file /var/run/quack-proxy/status.json

STOP:
  1. SIGTERM → all child processes → wait for graceful shutdown
  2. SIGKILL timeout timer (default 10s)

RELOAD:
  1. SIGHUP → re-parse configuration → incrementally update child processes
     - New shard → start a new process
     - Removed shard → graceful shutdown
     - Unchanged shard → keep running
```

---

## 4. Configuration Specification

### 4.1 quack-proxy.yaml

```yaml
# quack-proxy v0.1 config

global:
  log_level: info              # debug | info | warn | error
  pid_file: /var/run/quack-proxy/quack-proxy.pid
  status_file: /var/run/quack-proxy/status.json

listener:
  bind_host: 0.0.0.0            # Quack endpoint listen address
  port_start: 9491              # port start (allocated incrementally)
  health_path: /                # health check path (Quack HTTP)
  health_interval: 5s           # health check interval

shards:
  - name: orders_2024           # logical name
    database: /data/orders_2024.db
    port: 9491                  # optional, auto-assigned
    token: ""                   # optional, auto-generated when empty
    readonly: false             # optional

  - name: orders_2025
    database: /data/orders_2025.db
    # port auto-assigned as 9492

  - name: customers
    database: /data/customers.db
    # port auto-assigned as 9493

proxy:
  enabled: true                 # enable auto-generated HAProxy config
  output: /etc/haproxy/quack-proxy.cfg
  bind_port: 9490               # HAProxy frontend port
  mode: roundrobin              # roundrobin | leastconn
  ssl:
    enabled: false
    cert: /etc/ssl/quack.pem
```

### 4.2 Token Security Model

- Token not specified → a 32-character random token is auto-generated at startup and written to `status.json`
- Token specified → the specified value is used
- Production recommendation: Nginx reverse proxy for SSL termination + fixed token

---

## 5. CLI Interface

```
quack-proxy start [-c config.yaml]     # start the daemon
quack-proxy stop                        # stop the daemon
quack-proxy status [--json]            # show all shard statuses
quack-proxy reload                     # hot-reload configuration
quack-proxy gen-proxy [-c config.yaml] # generate HAProxy/Nginx config
quack-proxy version                    # print version
```

### 5.1 Example status Output

```
$ quack-proxy status

 NAME           PORT   STATUS    UPTIME    RESTARTS  DATABASE
 orders_2024   9491   healthy   2h 15m    0         /data/orders_2024.db
 orders_2025   9492   healthy   2h 15m    0         /data/orders_2025.db
 customers     9493   healthy   2h 14m    1         /data/customers.db

$ quack-proxy status --json
{
  "shards": [
    {
      "name": "orders_2024",
      "port": 9491,
      "status": "healthy",
      "pid": 12345,
      "uptime": "2h15m",
      "restarts": 0,
      "token": "a8f3...b9e1",
      "database": "/data/orders_2024.db",
      "last_health_check": "2026-05-29T12:00:00Z"
    }
  ],
  "coordinator_attach_sql": "ATTACH 'quack:localhost:9491' AS orders_2024;\nATTACH 'quack:localhost:9492' AS orders_2025;\n..."
}
```

### 5.2 coordinator_attach_sql

The `status --json` output includes a `coordinator_attach_sql` field — a block of SQL that can be copied directly into a Coordinator DuckDB to ATTACH all currently healthy Quack endpoints. It is the bridge between the cluster managed by quack-proxy and the Coordinator DuckDB.

---

## 6. Integration with duckdb_fdw

Once quack-proxy is managing the Quack endpoints, duckdb_fdw users can connect directly:

```sql
-- Configure duckdb_fdw in PG
CREATE SERVER quack_cluster FOREIGN DATA WRAPPER duckdb_fdw
OPTIONS (quack_host 'localhost:9490');  -- HAProxy VIP

CREATE USER MAPPING FOR current_user SERVER quack_cluster
OPTIONS (quack_token 'a8f3...b9e1');

IMPORT FOREIGN SCHEMA "remote" FROM SERVER quack_cluster INTO public;
```

Or connect to a single endpoint directly:
```sql
CREATE SERVER orders_2024_srv FOREIGN DATA WRAPPER duckdb_fdw
OPTIONS (quack_host '192.168.1.10:9491');

CREATE USER MAPPING FOR current_user SERVER orders_2024_srv
OPTIONS (quack_token 'token_from_status_json');
```

---

## 7. Non-Functional Requirements

| Requirement | Target |
|------|------|
| Startup time | <5s (including all DuckDB process starts + extension loading) |
| Health check interval | Configurable, default 5s |
| Failure recovery | Crash auto-restart with exponential backoff (1s→2s→4s→…→30s max) |
| Memory footprint | <50MB (excluding DuckDB child processes) |
| Binary size | <15MB (static Go build) |
| Platforms | Linux + macOS + Windows |
| DuckDB version | >= 1.5.2 (first Quack-capable version) |
| Concurrent shards | 1-1000 |

---

## 8. What We Won't Do

| Won't do | Reason |
|------|------|
| Distributed transaction coordination | Out of scope — a DuckDB transaction can only write one ATTACHed database |
| Automatic partitioning / sharding | Users decide their own data-partitioning strategy |
| Web UI / Dashboard | v0.1 is CLI-only |
| Implementing the Quack protocol | Use DuckDB's own Quack client (ATTACH mode) |
| Cross-machine Quack endpoint discovery | v0.1 focuses on single-machine multi-file management |
| Persistent Coordinator | The v0.1 Coordinator is stateless; users start DuckDB + ATTACH themselves |

---

## 9. Milestones

### M1: Minimum Viable Product (1-2 days)

- [ ] YAML configuration parsing
- [ ] Start N DuckDB+Quack child processes
- [ ] `start` / `stop` / `status` commands
- [ ] Basic health checks + auto-restart
- [ ] `status --json` with `coordinator_attach_sql` output

### M2: Operations Capabilities (1 day)

- [ ] SIGHUP hot reload (incremental child-process updates)
- [ ] `gen-proxy` HAProxy config generation
- [ ] Exponential-backoff restart policy
- [ ] PID file + graceful shutdown

### M3: Production-Ready (1 day)

- [ ] systemd unit file template
- [ ] Structured log output
- [ ] README + quick-start guide
- [ ] Binary releases (GitHub Releases)

---

## 10. Technology Choices

| Choice | Decision | Rationale |
|------|------|------|
| Language | Go | goroutines map naturally to child-process management, net/http works out of the box, fast compilation |
| Config format | YAML | Human-readable, common in the DuckDB ecosystem |
| Process management | os/exec | Standard library, no third-party dependency |
| HTTP client | net/http | Sufficient for health checks |
| Logging | slog | Go 1.21+ standard library |
| CLI framework | None (flag + hand-written) | Few commands, not worth pulling in cobra |
| Cross-platform | Linux + macOS + Windows | The Quack extension supports all three platforms |

---

*Document version v0.1 | Next step: go mod init + scaffolding code*
