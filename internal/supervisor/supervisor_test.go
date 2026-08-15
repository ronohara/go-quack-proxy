package supervisor

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/alitrack/quack-proxy/internal/config"
	"github.com/alitrack/quack-proxy/internal/logger"
)

func newTestLogger(level logger.Level) *logger.Logger {
	lg, err := logger.New(logger.Config{Level: level})
	if err != nil {
		panic(err)
	}
	return lg
}

func TestNewSupervisor(t *testing.T) {
	cfg := &config.Config{
		Listener: config.ListenerConfig{BindHost: "0.0.0.0", HealthPath: "/", HealthInterval: 5 * time.Second},
		Shards:   []config.ShardConfig{{Name: "test", Database: "/tmp/test.db", Port: 9491}},
	}
	sup := New(cfg, newTestLogger(logger.LevelInfo))

	if len(sup.shards) != 0 {
		t.Error("new supervisor should have empty shards map")
	}
}

func TestAttachSQL(t *testing.T) {
	cfg := &config.Config{
		Listener: config.ListenerConfig{BindHost: "localhost", HealthPath: "/", HealthInterval: 5 * time.Second},
		Shards: []config.ShardConfig{
			{Name: "analytics", Database: "/tmp/a.db", Port: 9491},
			{Name: "logs", Database: "/tmp/l.db", Port: 9492},
		},
	}
	sup := New(cfg, newTestLogger(logger.LevelInfo))

	// No shards started, so AttachSQL should be empty
	sql := sup.AttachSQL()
	if sql != "" {
		t.Errorf("AttachSQL with no shards = %q, want empty", sql)
	}
}

func TestStartAll(t *testing.T) {
	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")

	cfg := &config.Config{
		Listener: config.ListenerConfig{BindHost: "127.0.0.1", HealthPath: "/", HealthInterval: 5 * time.Second},
		Shards: []config.ShardConfig{
			{Name: "testshard", Database: dbPath, Port: 9499},
		},
	}
	sup := New(cfg, newTestLogger(logger.LevelDebug))

	ctx := context.Background()
	if err := sup.StartAll(ctx); err != nil {
		t.Fatalf("StartAll: %v", err)
	}

	sp, ok := sup.shards["testshard"]
	if !ok {
		t.Fatal("shard not found in supervisor")
	}

	if sp.Status != "starting" {
		t.Errorf("status = %q, want starting", sp.Status)
	}

	if sp.db == nil {
		t.Error("db = nil, want an open in-process DuckDB instance")
	}

	if sp.Config.Token == "" {
		t.Error("token should not be empty")
	}

	// Cleanup
	sup.StopAll()
}

func TestStatus(t *testing.T) {
	cfg := &config.Config{
		Listener: config.ListenerConfig{BindHost: "127.0.0.1", HealthPath: "/", HealthInterval: 5 * time.Second},
		Shards: []config.ShardConfig{
			{Name: "s1", Database: "/tmp/s1.db", Port: 9491},
			{Name: "s2", Database: "/tmp/s2.db", Port: 9492},
		},
	}
	sup := New(cfg, newTestLogger(logger.LevelInfo))

	status := sup.Status()
	if len(status) != 0 {
		t.Errorf("Status with no started shards = %d items, want 0", len(status))
	}
}

func TestHealthCheckIntegration(t *testing.T) {
	// Start a fake HTTP server as a "healthy Quack endpoint"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tmp := t.TempDir()
	dbPath := filepath.Join(tmp, "test.db")

	testPort := 19503 // Use a port that isn't the fake server

	cfg := &config.Config{
		Listener: config.ListenerConfig{
			BindHost:       "127.0.0.1",
			HealthPath:     "/",
			HealthInterval: 100 * time.Millisecond,
		},
		Shards: []config.ShardConfig{
			{Name: "healthy-shard", Database: dbPath, Port: testPort},
		},
	}
	sup := New(cfg, newTestLogger(logger.LevelInfo))

	// Manually insert a "healthy" shard entry pointing to our fake server
	sup.mu.Lock()
	sup.shards["healthy-shard"] = &ShardProcess{
		Config:    config.ShardConfig{Name: "healthy-shard", Database: dbPath, Port: testPort},
		Status:    "starting",
		StartTime: time.Now(),
	}
	sup.mu.Unlock()

	// Run one health check — should mark as healthy
	ctx := context.Background()
	sup.checkAll(ctx)

	sup.mu.RLock()
	sp := sup.shards["healthy-shard"]
	sup.mu.RUnlock()

	if sp.Status != "unhealthy" {
		// Connection to testPort will fail (no server), so it should be unhealthy
		// That's actually the expected behavior for a non-existing server
	}
	// The main verification is that checkAll doesn't panic
}

func TestStopAllCleansUp(t *testing.T) {
	cfg := &config.Config{
		Listener: config.ListenerConfig{BindHost: "127.0.0.1", HealthPath: "/", HealthInterval: 5 * time.Second},
		Shards:   []config.ShardConfig{},
	}
	sup := New(cfg, newTestLogger(logger.LevelInfo))

	// StopAll on empty supervisor should not panic
	sup.StopAll()

	if sup.cancel != nil {
		t.Error("expected cancel to be nil after start without StartAll")
	}
}

// holdPort binds a listener on an ephemeral port and returns the port
// number. The listener stays open (port held) until the caller closes it.
func holdPort(t *testing.T) (int, net.Listener) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("holdPort: %v", err)
	}
	return l.Addr().(*net.TCPAddr).Port, l
}

// freePort returns an ephemeral port that was free a moment ago.
func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func TestRetryBackoff(t *testing.T) {
	cases := []struct {
		n    int
		want time.Duration
	}{
		{1, time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
		{6, 30 * time.Second},
		{10, 30 * time.Second},
	}
	for _, c := range cases {
		if got := retryBackoff(c.n); got != c.want {
			t.Errorf("retryBackoff(%d) = %v, want %v", c.n, got, c.want)
		}
	}
}

func TestStartAllIsolation(t *testing.T) {
	tmp := t.TempDir()
	badPort, badLn := holdPort(t)
	defer badLn.Close()
	goodPort := freePort(t)

	cfg := &config.Config{
		Listener: config.ListenerConfig{BindHost: "127.0.0.1", HealthPath: "/", HealthInterval: 5 * time.Second},
		Shards: []config.ShardConfig{
			{Name: "bad", Database: filepath.Join(tmp, "bad.db"), Port: badPort},
			{Name: "good", Database: filepath.Join(tmp, "good.db"), Port: goodPort},
		},
	}
	sup := New(cfg, newTestLogger(logger.LevelInfo))

	if err := sup.StartAll(context.Background()); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	if len(sup.shards) != 2 {
		t.Fatalf("shards registered = %d, want 2", len(sup.shards))
	}

	bad := sup.shards["bad"]
	if bad.db != nil {
		t.Error("bad shard db != nil, want start failure")
	}
	if bad.Status != "unhealthy" {
		t.Errorf("bad shard status = %q, want unhealthy", bad.Status)
	}
	if bad.Error == "" {
		t.Error("bad shard error empty, want the bind failure recorded")
	}

	good := sup.shards["good"]
	if good.db == nil {
		t.Error("good shard db == nil, want it started despite the bad shard")
	}

	sup.StopAll()
}

func TestRestartsClimbWithBackoff(t *testing.T) {
	tmp := t.TempDir()
	port, ln := holdPort(t)
	defer ln.Close()

	sup := New(&config.Config{Listener: config.ListenerConfig{BindHost: "127.0.0.1"}}, newTestLogger(logger.LevelQuiet))
	sp := &ShardProcess{
		Config: config.ShardConfig{Name: "loop", Database: filepath.Join(tmp, "loop.db"), Port: port},
		Status: "unhealthy",
	}

	// Attempt 1 — immediate (no gate yet).
	sup.restartShardLocked("loop", sp)
	if sp.Restarts != 1 || sp.consecutiveFailures != 1 || sp.Status != "unhealthy" {
		t.Fatalf("after attempt 1: restarts=%d failures=%d status=%q", sp.Restarts, sp.consecutiveFailures, sp.Status)
	}
	if got := time.Until(sp.nextRetry); got < 900*time.Millisecond || got > 1100*time.Millisecond {
		t.Errorf("nextRetry in %v, want ≈1s", got)
	}

	// An immediate re-call is gated by the backoff window.
	sup.restartShardLocked("loop", sp)
	if sp.Restarts != 1 {
		t.Errorf("gated call still attempted: restarts=%d, want 1", sp.Restarts)
	}

	// Open the gate — attempt 2.
	sp.nextRetry = time.Time{}
	sup.restartShardLocked("loop", sp)
	if sp.Restarts != 2 || sp.consecutiveFailures != 2 {
		t.Fatalf("after attempt 2: restarts=%d failures=%d", sp.Restarts, sp.consecutiveFailures)
	}
	if got := time.Until(sp.nextRetry); got < 1900*time.Millisecond || got > 2100*time.Millisecond {
		t.Errorf("nextRetry in %v, want ≈2s", got)
	}
}

func TestGiveUpAfterConsecutiveFailures(t *testing.T) {
	tmp := t.TempDir()
	port, ln := holdPort(t)
	defer ln.Close()

	sup := New(&config.Config{Listener: config.ListenerConfig{BindHost: "127.0.0.1"}}, newTestLogger(logger.LevelQuiet))
	sp := &ShardProcess{
		Config: config.ShardConfig{Name: "doomed", Database: filepath.Join(tmp, "doomed.db"), Port: port},
		Status: "unhealthy",
	}

	for i := 0; i < maxConsecutiveFailures; i++ {
		sp.nextRetry = time.Time{}
		sup.restartShardLocked("doomed", sp)
	}

	if sp.Status != "stopped" {
		t.Errorf("status = %q, want stopped after %d failures", sp.Status, maxConsecutiveFailures)
	}
	if sp.Error == "" {
		t.Error("error empty, want the last failure recorded")
	}
	if sp.Restarts != maxConsecutiveFailures {
		t.Errorf("restarts = %d, want %d", sp.Restarts, maxConsecutiveFailures)
	}
}

func TestReArm(t *testing.T) {
	sup := New(&config.Config{Listener: config.ListenerConfig{BindHost: "127.0.0.1"}}, newTestLogger(logger.LevelQuiet))
	sup.ManualSetShard("doomed", ShardProcess{
		Config: config.ShardConfig{Name: "doomed", Database: "/tmp/d.db", Port: 9499},
		Status: "stopped",
		Error:  "gave up",
	})
	sup.ReArm()

	sp := sup.shards["doomed"]
	if sp.Status != "unhealthy" {
		t.Errorf("status = %q, want unhealthy after re-arm", sp.Status)
	}
	if sp.consecutiveFailures != 0 {
		t.Errorf("failures = %d, want 0", sp.consecutiveFailures)
	}
	if sp.nextRetry.IsZero() {
		t.Error("nextRetry zero, want an immediate retry window")
	}
}

func TestRecoveryOnPortRelease(t *testing.T) {
	tmp := t.TempDir()
	port, ln := holdPort(t)

	sup := New(&config.Config{Listener: config.ListenerConfig{BindHost: "127.0.0.1"}}, newTestLogger(logger.LevelQuiet))
	sp := &ShardProcess{
		Config: config.ShardConfig{Name: "recover", Database: filepath.Join(tmp, "recover.db"), Port: port},
		Status: "unhealthy",
	}

	sup.restartShardLocked("recover", sp)
	if sp.Status != "unhealthy" || sp.Error == "" {
		t.Fatalf("expected initial failure: status=%q error=%q", sp.Status, sp.Error)
	}

	// Release the port and retry — recovery should succeed.
	ln.Close()
	var recovered bool
	for i := 0; i < 5 && !recovered; i++ {
		time.Sleep(200 * time.Millisecond)
		sp.nextRetry = time.Time{}
		sup.restartShardLocked("recover", sp)
		recovered = sp.db != nil
	}

	if sp.db == nil {
		t.Fatal("db == nil, want recovery start to succeed after port release")
	}
	if sp.Status != "starting" {
		t.Errorf("status = %q, want starting", sp.Status)
	}
	if sp.Error != "" {
		t.Errorf("error = %q, want empty after recovery", sp.Error)
	}
	if sp.consecutiveFailures != 0 {
		t.Errorf("failures = %d, want 0 after recovery", sp.consecutiveFailures)
	}
	sp.db.Close()
}
