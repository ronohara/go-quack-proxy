package admin

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/alitrack/quack-proxy/internal/config"
	"github.com/alitrack/quack-proxy/internal/logger"
	"github.com/alitrack/quack-proxy/internal/supervisor"
)

const testToken = "secret-token-abc123"

// newTestServer builds an admin server over a supervisor with three
// injected shards: one healthy with a token, one unhealthy without a
// token, and one with a zero StartTime (uptime-guard coverage).
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	log, err := logger.New(logger.Config{Level: logger.LevelQuiet})
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		Listener: config.ListenerConfig{BindHost: "127.0.0.1", HealthPath: "/"},
		Shards:   []config.ShardConfig{{Name: "analytics", Port: 9491}},
	}
	sup := supervisor.New(cfg, log)
	sup.ManualSetShard("analytics", supervisor.ShardProcess{
		Config:    config.ShardConfig{Name: "analytics", Port: 9491, Database: `C:\path\workbench.duckdb`, Token: testToken},
		Status:    "healthy",
		StartTime: time.Now().Add(-2 * time.Hour),
		Restarts:  2,
	})
	sup.ManualSetShard("notoken", supervisor.ShardProcess{
		Config: config.ShardConfig{Name: "notoken", Port: 9492, Database: `C:\path\other.duckdb`},
		Status: "unhealthy",
		Error:  "Failed to bind DuckDB Quack RPC server (address in use)",
	})
	sup.ManualSetShard("zerotime", supervisor.ShardProcess{
		Config: config.ShardConfig{Name: "zerotime", Port: 9493, Database: `C:\path\zero.duckdb`, Token: "x"},
		Status: "starting",
	})
	srv := httptest.NewServer(New(sup, "0.3.0-test", log).Handler())
	t.Cleanup(srv.Close)
	return srv
}

func getRaw(t *testing.T, srv *httptest.Server, method, path string) (*http.Response, string) {
	t.Helper()
	var resp *http.Response
	var err error
	if method == http.MethodGet {
		resp, err = http.Get(srv.URL + path)
	} else {
		resp, err = http.Post(srv.URL+path, "application/json", strings.NewReader("{}"))
	}
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp, string(body)
}

func TestStatusShape(t *testing.T) {
	srv := newTestServer(t)
	resp, body := getRaw(t, srv, http.MethodGet, "/status")

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	var out StatusResponse
	if err := json.Unmarshal([]byte(body), &out); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if out.Version != "0.3.0-test" {
		t.Errorf("version = %q, want 0.3.0-test", out.Version)
	}
	if out.PID != os.Getpid() {
		t.Errorf("pid = %d, want %d", out.PID, os.Getpid())
	}
	if _, err := time.Parse(time.RFC3339, out.StartedAt); err != nil {
		t.Errorf("started_at %q is not RFC3339: %v", out.StartedAt, err)
	}
	if len(out.Shards) != 3 {
		t.Fatalf("shards = %d, want 3", len(out.Shards))
	}

	byName := make(map[string]ShardStatus)
	for _, s := range out.Shards {
		byName[s.Name] = s
	}

	a := byName["analytics"]
	if a.Port != 9491 || a.Status != "healthy" || a.Restarts != 2 {
		t.Errorf("analytics = %+v, want port 9491, healthy, restarts 2", a)
	}
	if a.Database != `C:\path\workbench.duckdb` {
		t.Errorf("analytics database = %q", a.Database)
	}
	if !a.TokenSet {
		t.Error("analytics token_set = false, want true (token configured)")
	}
	if a.UptimeSeconds < 7200-60 || a.UptimeSeconds > 7200+60 {
		t.Errorf("analytics uptime_seconds = %d, want ≈7200", a.UptimeSeconds)
	}
	if _, err := time.Parse(time.RFC3339, a.StartTime); err != nil {
		t.Errorf("analytics start_time %q is not RFC3339: %v", a.StartTime, err)
	}

	if n := byName["notoken"]; n.TokenSet {
		t.Error("notoken token_set = true, want false (no token configured)")
	}
	if n := byName["notoken"]; n.Error != "Failed to bind DuckDB Quack RPC server (address in use)" {
		t.Errorf("notoken error = %q, want the injected failure", n.Error)
	}
	if a.Error != "" {
		t.Errorf("analytics error = %q, want empty for a healthy shard", a.Error)
	}
	if z := byName["zerotime"]; z.UptimeSeconds != 0 {
		t.Errorf("zerotime uptime_seconds = %d, want 0 (zero StartTime guard)", z.UptimeSeconds)
	}

	// attach_sql includes only healthy shards.
	if !strings.Contains(out.AttachSQL, "ATTACH 'quack:127.0.0.1:9491' AS analytics;") {
		t.Errorf("attach_sql missing analytics line: %q", out.AttachSQL)
	}
	if strings.Contains(out.AttachSQL, "notoken") || strings.Contains(out.AttachSQL, "zerotime") {
		t.Errorf("attach_sql contains non-healthy shards: %q", out.AttachSQL)
	}
}

func TestTokenNeverInBody(t *testing.T) {
	srv := newTestServer(t)
	_, body := getRaw(t, srv, http.MethodGet, "/status")

	if strings.Contains(body, testToken) {
		t.Fatal("response body contains the raw token")
	}
	if !strings.Contains(body, `"token_set":true`) {
		t.Error("response body missing token_set:true for the configured shard")
	}
}

func TestUnknownPath404(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := getRaw(t, srv, http.MethodGet, "/nope")
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("GET /nope = %d, want 404", resp.StatusCode)
	}
}

func TestPostStatus405(t *testing.T) {
	srv := newTestServer(t)
	resp, _ := getRaw(t, srv, http.MethodPost, "/status")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST /status = %d, want 405", resp.StatusCode)
	}
}
