// Package supervisor manages DuckDB+Quack child processes.
package supervisor

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/alitrack/quack-proxy/internal/config"
	"github.com/alitrack/quack-proxy/internal/health"
	"github.com/alitrack/quack-proxy/internal/logger"
)

const quackBootSQL = `
CALL quack_serve('quack:%s:%d', token = '%s', allow_other_hostname = true);
`

type Supervisor struct {
	cfg    *config.Config
	shards map[string]*ShardProcess
	mu     sync.RWMutex
	logger *logger.Logger
	cancel context.CancelFunc
}

type ShardProcess struct {
	Config    config.ShardConfig
	cmd       *exec.Cmd
	PID       int
	Status    string // "starting", "healthy", "unhealthy", "stopped"
	StartTime time.Time
	Restarts  int
	lastCheck time.Time
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

	for _, shardCfg := range s.cfg.Shards {
		if err := s.startShardLocked(ctx, shardCfg); err != nil {
			return fmt.Errorf("start %s: %w", shardCfg.Name, err)
		}
	}

	s.logger.Infof("all shards started, count: %d", len(s.shards))
	return nil
}

func (s *Supervisor) startShardLocked(ctx context.Context, cfg config.ShardConfig) error {
	token := cfg.Token
	if token == "" {
		token = randomToken(32)
		s.logger.Verbosef("generated random token for shard '%s'", cfg.Name)
	}

	sql := fmt.Sprintf(quackBootSQL, s.cfg.Listener.BindHost, cfg.Port, token)

	if s.logger.IsDebug() {
		s.logger.Debugf("SQL for shard '%s': %s", cfg.Name, strings.TrimSpace(sql))
	}

	// Use shell pipe trick: pipe init SQL then keep stdin open via
	// tail -f /dev/null so duckdb stays alive serving Quack indefinitely.
	shellCmd := fmt.Sprintf(
		`(echo '%s'; tail -f /dev/null) | duckdb '%s'`,
		strings.ReplaceAll(sql, "'", "'\\''"),
		cfg.Database,
	)

	s.logger.Verbosef("starting shard '%s' on port %d, database: %s", cfg.Name, cfg.Port, cfg.Database)

	cmd := exec.Command("bash", "-c", shellCmd)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = setProcessGroup()
	cmd.Cancel = nil // disable CommandContext's internal context kill goroutine

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start duckdb: %w", err)
	}

	s.logger.Verbosef("shard '%s' started with PID %d", cfg.Name, cmd.Process.Pid)

	sp := &ShardProcess{
		Config:    cfg,
		cmd:       cmd,
		PID:       cmd.Process.Pid,
		Status:    "starting",
		StartTime: time.Now(),
	}
	sp.Config.Token = token
	s.shards[cfg.Name] = sp
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
	s.logger.Verbosef("stopping shard '%s' (PID %d)", name, sp.PID)
	if sp.cmd != nil && sp.cmd.Process != nil {
		sp.cmd.Process.Signal(syscall.SIGTERM)
		done := make(chan error, 1)
		go func() { done <- sp.cmd.Wait() }()
		select {
		case <-done:
			s.logger.Verbosef("shard '%s' stopped gracefully", name)
		case <-time.After(10 * time.Second):
			s.logger.Warnf("shard '%s' did not stop, killing", name)
			sp.cmd.Process.Kill()
		}
	}
	sp.Status = "stopped"
	s.logger.Infof("shard stopped: %s", name)
}

func (s *Supervisor) HealthLoop(ctx context.Context) {
	s.logger.Verbosef("health loop started, interval: %v", s.cfg.Listener.HealthInterval)

	// Initial grace period: give DuckDB time to start Quack
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
		} else {
			sp.Status = "unhealthy"
			s.logger.Warnf("shard '%s' is unhealthy, restarting (restart count: %d)",
				name, sp.Restarts+1)
			s.stopShardLocked(name, sp)
			if err := s.startShardLocked(ctx, sp.Config); err != nil {
				s.logger.Errorf("failed to restart shard '%s': %v", name, err)
			}
			sp.Restarts++
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

// ManualSetShard is a test helper to inject shard state without starting a real DuckDB process.
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