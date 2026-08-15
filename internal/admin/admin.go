// Package admin exposes the proxy's internal state over a read-only
// HTTP endpoint. The server shares the supervisor instance with the
// running proxy, so GET /status reports the LIVE shard state — uptime,
// restart count, per-shard status — never a fresh empty snapshot.
//
// Security: read-only by design; no stop/restart/config actions. Token
// masking is mandatory — the response carries only token_set: true/false,
// never the token itself.
package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/alitrack/quack-proxy/internal/logger"
	"github.com/alitrack/quack-proxy/internal/supervisor"
)

type Server struct {
	sup       *supervisor.Supervisor
	version   string
	startedAt time.Time
	log       *logger.Logger
}

type ShardStatus struct {
	Name          string `json:"name"`
	Port          int    `json:"port"`
	Status        string `json:"status"` // starting | healthy | unhealthy | stopped
	StartTime     string `json:"start_time"`
	UptimeSeconds int64  `json:"uptime_seconds"`
	Restarts      int    `json:"restarts"`
	Database      string `json:"database"`
	TokenSet      bool   `json:"token_set"`       // NEVER the token itself
	Error         string `json:"error,omitempty"` // last start failure
}

type StatusResponse struct {
	Version   string        `json:"version"`
	PID       int           `json:"pid"`
	StartedAt string        `json:"started_at"`
	Shards    []ShardStatus `json:"shards"`
	AttachSQL string        `json:"attach_sql"`
}

// New creates an admin server around the given supervisor. version is the
// proxy build version reported in the snapshot.
func New(sup *supervisor.Supervisor, version string, log *logger.Logger) *Server {
	return &Server{
		sup:       sup,
		version:   version,
		startedAt: time.Now(),
		log:       log,
	}
}

// Handler returns the HTTP handler: GET /status, everything else 404.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/status", s.handleStatus)
	return mux
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	now := time.Now()
	shards := make([]ShardStatus, 0)
	for _, sp := range s.sup.Status() {
		var uptime int64
		if !sp.StartTime.IsZero() {
			uptime = int64(now.Sub(sp.StartTime).Seconds())
		}
		shards = append(shards, ShardStatus{
			Name:          sp.Config.Name,
			Port:          sp.Config.Port,
			Status:        sp.Status,
			StartTime:     sp.StartTime.Format(time.RFC3339),
			UptimeSeconds: uptime,
			Restarts:      sp.Restarts,
			Database:      sp.Config.Database,
			TokenSet:      sp.Config.Token != "",
			Error:         sp.Error,
		})
	}

	resp := StatusResponse{
		Version:   s.version,
		PID:       os.Getpid(),
		StartedAt: s.startedAt.Format(time.RFC3339),
		Shards:    shards,
		AttachSQL: s.sup.AttachSQL(),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil && s.log != nil {
		s.log.Errorf("admin /status: encode response: %v", err)
	}
}

// ListenAndServe serves the admin HTTP API on addr until ctx is canceled,
// then shuts down gracefully.
func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Handler()}

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil && s.log != nil {
			s.log.Warnf("admin server shutdown: %v", err)
		}
	}()

	err := srv.ListenAndServe()
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
