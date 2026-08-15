// Package supervisor manages in-process DuckDB Quack servers.
//
// Option B architecture: each shard runs the Quack server INSIDE the
// quack-proxy process, via the CGo-linked libduckdb (duckdb-go/v2).
// No duckdb.exe child process, no stdin keep-alive, no console — the
// CALL quack_serve(...) statement detaches a server that lives as long
// as its *sql.DB handle stays open. Closing the handle stops the server.
// This is platform-independent by construction.
package supervisor

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2" // in-process DuckDB engine (CGo)

	"github.com/alitrack/quack-proxy/internal/config"
	"github.com/alitrack/quack-proxy/internal/health"
	"github.com/alitrack/quack-proxy/internal/logger"
)

const quackBootSQL = `
CALL quack_serve('quack:%s:%d', token = '%s', allow_other_hostname = true);
`

// maxConsecutiveFailures is the give-up threshold: after this many
// consecutive failed restart attempts a shard is marked "stopped" until
// re-armed (SIGHUP reload).
const maxConsecutiveFailures = 10

type Supervisor struct {
	cfg    *config.Config
	shards map[string]*ShardProcess
	mu     sync.RWMutex
	logger *logger.Logger
	cancel context.CancelFunc
}

type ShardProcess struct {
	Config              config.ShardConfig
	db                  *sql.DB // in-process DuckDB instance; Quack serves while this is open
	Status              string  // "starting", "healthy", "unhealthy", "stopped"
	StartTime           time.Time
	Restarts            int
	Error               string // last start failure; empty while the shard is running
	consecutiveFailures int
	nextRetry           time.Time
	lastCheck           time.Time
}

func New(cfg *config.Config, log *logger.Logger) *Supervisor {
	return &Supervisor{
		cfg:    cfg,
		shards: make(map[string]*ShardProcess),
		logger: log,
	}
}

func (s *Supervisor) StartAll(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, s.cancel = context.WithCancel(ctx)

	// Non-fatal start: one bad shard must not take the proxy — or the
	// admin endpoint and the other shards — down with it. Every shard is
	// registered up front; failures are recorded (unhealthy + error) and
	// retried by the health loop.
	for _, shardCfg := range s.cfg.Shards {
		sp := &ShardProcess{Config: shardCfg, Status: "starting", StartTime: time.Now()}
		s.shards[shardCfg.Name] = sp
		if err := s.startShardLocked(sp); err != nil {
			s.logger.Errorf("shard '%s' failed to start: %v", shardCfg.Name, err)
			sp.Status = "unhealthy"
			sp.Error = err.Error()
		}
	}

	ready, failed := 0, 0
	for _, sp := range s.shards {
		if sp.db != nil {
			ready++
		} else {
			failed++
		}
	}
	s.logger.Infof("shards started: %d ready, %d failed", ready, failed)
	return nil
}

// startShardLocked opens the in-process DuckDB engine and starts the Quack
// server for the shard. It updates sp in place so identity and Restarts are
// preserved across attempts. The caller must hold s.mu.
func (s *Supervisor) startShardLocked(sp *ShardProcess) error {
	cfg := sp.Config
	token := cfg.Token
	if token == "" {
		token = randomToken(32)
		cfg.Token = token
		sp.Config = cfg
		s.logger.Verbosef("generated random token for shard '%s'", cfg.Name)
	}

	bootSQL := fmt.Sprintf(quackBootSQL, s.cfg.Listener.BindHost, cfg.Port, token)

	if s.logger.IsDebug() {
		s.logger.Debugf("SQL for shard '%s': %s", cfg.Name, strings.TrimSpace(bootSQL))
	}

	s.logger.Verbosef("starting shard '%s' on port %d, database: %s", cfg.Name, cfg.Port, cfg.Database)

	// The engine creates missing FILES, but not missing DIRECTORIES —
	// ensure the database's parent directory exists first (same guarantee
	// as init-db, so a deleted data\ dir cannot brick restarts).
	if err := os.MkdirAll(filepath.Dir(cfg.Database), 0755); err != nil {
		return fmt.Errorf("create directory for %s: %w", cfg.Database, err)
	}

	// Open the in-process DuckDB instance for this shard. A single
	// pooled connection guarantees one engine per shard.
	db, err := sql.Open("duckdb", cfg.Database)
	if err != nil {
		return fmt.Errorf("open duckdb %s: %w", cfg.Database, err)
	}
	db.SetMaxOpenConns(1)

	// Start the detached Quack server. The statement returns immediately;
	// the server keeps serving while the db handle stays open.
	if _, err := db.Exec(bootSQL); err != nil {
		db.Close()
		return fmt.Errorf("start quack server on port %d: %w", cfg.Port, err)
	}

	s.logger.Verbosef("shard '%s' serving in-process on port %d", cfg.Name, cfg.Port)

	sp.db = db
	sp.Status = "starting"
	sp.StartTime = time.Now()
	return nil
}

func (s *Supervisor) StopAll() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	for name, sp := range s.shards {
		s.stopShardLocked(name, sp)
	}
}

func (s *Supervisor) stopShardLocked(name string, sp *ShardProcess) {
	s.logger.Verbosef("stopping shard '%s'", name)
	if sp.db != nil {
		// Closing the database handle stops the in-process Quack server.
		if err := sp.db.Close(); err != nil {
			s.logger.Warnf("shard '%s' close error: %v", name, err)
		}
		sp.db = nil
	}
	sp.Status = "stopped"
	s.logger.Infof("shard stopped: %s", name)
}

func (s *Supervisor) HealthLoop(ctx context.Context) {
	s.logger.Verbosef("health loop started, interval: %v", s.cfg.Listener.HealthInterval)

	// Initial grace period: give the Quack servers time to start.
	select {
	case <-ctx.Done():
		s.logger.Verbosef("health loop canceled during grace period")
		return
	case <-time.After(15 * time.Second):
		s.logger.Verbosef("grace period completed, starting health checks")
	}

	ticker := time.NewTicker(s.cfg.Listener.HealthInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			s.logger.Verbosef("health loop canceled")
			return
		case <-ticker.C:
			s.checkAll(ctx)
		}
	}
}

func (s *Supervisor) checkAll(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for name, sp := range s.shards {
		if sp.Status == "stopped" {
			continue
		}

		// A shard without a database handle (failed start) cannot be
		// health-checked — retry its start instead.
		if sp.db == nil {
			s.restartShardLocked(name, sp)
			continue
		}

		host := s.cfg.Listener.BindHost
		if host == "0.0.0.0" {
			host = "127.0.0.1"
		}

		if s.logger.IsDebug() {
			s.logger.Debugf("health check for shard '%s': http://%s:%d%s",
				name, host, sp.Config.Port, s.cfg.Listener.HealthPath)
		}

		ok := health.Check(
			host,
			sp.Config.Port,
			s.cfg.Listener.HealthPath,
			2*time.Second,
		)
		sp.lastCheck = time.Now()

		if ok {
			if sp.Status != "healthy" {
				s.logger.Infof("shard '%s' is now healthy", name)
			}
			sp.Status = "healthy"
			sp.Error = ""
		} else {
			s.restartShardLocked(name, sp)
		}
	}
}

// restartShardLocked stops and restarts a shard in place: the existing
// ShardProcess is kept, so Restarts accumulates across attempts, and a
// failed restart leaves the shard "unhealthy" in the retry set (never
// "stopped", which the health loop would skip forever). Retry attempts
// follow exponential backoff (1s → 30s cap), and after maxConsecutiveFailures
// the shard is marked "stopped" with its last error (re-armed by ReArm).
func (s *Supervisor) restartShardLocked(name string, sp *ShardProcess) {
	// Backoff gate: skip attempts until the retry window opens.
	if time.Now().Before(sp.nextRetry) {
		return
	}

	s.logger.Warnf("shard '%s' is unhealthy, restarting (restart count: %d)", name, sp.Restarts+1)
	s.stopShardLocked(name, sp)
	err := s.startShardLocked(sp)
	sp.Restarts++
	if err != nil {
		sp.consecutiveFailures++
		if sp.consecutiveFailures >= maxConsecutiveFailures {
			s.logger.Errorf("shard '%s' failed %d consecutive restarts — giving up: %v", name, sp.consecutiveFailures, err)
			sp.Status = "stopped"
			sp.Error = err.Error()
			return
		}
		s.logger.Errorf("failed to restart shard '%s': %v", name, err)
		sp.Status = "unhealthy"
		sp.Error = err.Error()
		sp.nextRetry = time.Now().Add(retryBackoff(sp.consecutiveFailures))
		return
	}
	sp.consecutiveFailures = 0
	sp.nextRetry = time.Time{}
	sp.Error = ""
}

// ReArm resets shards that gave up back into the retry set. Called on
// SIGHUP config reload.
func (s *Supervisor) ReArm() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for name, sp := range s.shards {
		if sp.Status == "stopped" {
			s.logger.Infof("re-arming shard '%s'", name)
			sp.Status = "unhealthy"
			sp.consecutiveFailures = 0
			sp.nextRetry = time.Now()
		}
	}
}

func (s *Supervisor) Status() []ShardProcess {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ShardProcess, 0, len(s.shards))
	for _, sp := range s.shards {
		result = append(result, *sp)
	}
	return result
}

// ManualSetShard is a test helper to inject shard state without starting a real DuckDB engine.
func (s *Supervisor) ManualSetShard(name string, sp ShardProcess) {
	s.mu.Lock()
	defer s.mu.Unlock()
	spCopy := sp
	s.shards[name] = &spCopy
}

func (s *Supervisor) AttachSQL() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var sql string
	for _, sp := range s.shards {
		if sp.Status == "healthy" {
			sql += fmt.Sprintf("ATTACH 'quack:%s:%d' AS %s;\n",
				s.cfg.Listener.BindHost, sp.Config.Port, sp.Config.Name)
		}
	}
	return sql
}

func randomToken(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
	}
	return string(b)
}

// retryBackoff returns the delay before the next restart attempt after n
// consecutive failed attempts: 1s, 2s, 4s, … capped at 30s.
func retryBackoff(n int) time.Duration {
	delay := time.Second
	for i := 1; i < n; i++ {
		delay *= 2
		if delay >= 30*time.Second {
			return 30 * time.Second
		}
	}
	return delay
}
