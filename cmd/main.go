package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	_ "github.com/duckdb/duckdb-go/v2" // in-process DuckDB engine (CGo)

	"github.com/alitrack/quack-proxy/internal/admin"
	"github.com/alitrack/quack-proxy/internal/config"
	"github.com/alitrack/quack-proxy/internal/logger"
	"github.com/alitrack/quack-proxy/internal/proxy"
	"github.com/alitrack/quack-proxy/internal/supervisor"
)

var (
	configPath string
	verbose    bool
	debug      bool
	quiet      bool
	logFile    string
	logJSON    bool
	// configExplicit records whether -c was given on the command line.
	// The status command uses it: an explicitly-given missing config is
	// an error, while a missing DEFAULT config falls back to the built-in
	// admin defaults.
	configExplicit bool
)

// version is the single source of truth for the build version, reported
// by the version subcommand, the startup log, and the admin /status snapshot.
const version = "0.3.0"

func init() {
	flag.StringVar(&configPath, "c", "quack-proxy.yaml", "config file path")
	flag.BoolVar(&verbose, "verbose", false, "verbose logging")
	flag.BoolVar(&debug, "debug", false, "debug logging (includes SQL)")
	flag.BoolVar(&quiet, "quiet", false, "quiet mode (errors only)")
	flag.StringVar(&logFile, "log-file", "", "write logs to file")
	flag.BoolVar(&logJSON, "log-json", false, "JSON log format")
}

func main() {
	flag.Usage = usage
	flag.Parse()

	flag.Visit(func(f *flag.Flag) {
		if f.Name == "c" {
			configExplicit = true
		}
	})

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	// Determine log level
	logLevel := logger.LevelInfo
	if quiet {
		logLevel = logger.LevelQuiet
	} else if verbose {
		logLevel = logger.LevelVerbose
	} else if debug {
		logLevel = logger.LevelDebug
	}

	// Create logger
	logCfg := logger.Config{
		Level:   logLevel,
		LogFile: logFile,
		JSON:    logJSON,
	}

	log, err := logger.New(logCfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v\n", err)
		os.Exit(1)
	}

	// Get base directory
	baseDir := getBaseDir(log)

	cfgPath, foundConfig := findConfigArg(args, baseDir)
	if foundConfig {
		configExplicit = true
	}

	switch args[0] {
	case "start":
		runStart(cfgPath, baseDir, log)
	case "stop":
		runStop(cfgPath, log)
	case "status":
		runStatus(cfgPath, log)
	case "reload":
		runReload(cfgPath, log)
	case "gen-proxy":
		runGenProxy(cfgPath, log)
	case "init-db":
		runInitDB(cfgPath, baseDir, log)
	case "version":
		fmt.Println("quack-proxy v" + version)
	default:
		log.Errorf("unknown command: %s", args[0])
		usage()
		os.Exit(1)
	}
}

func getBaseDir(log *logger.Logger) string {
	exe, err := os.Executable()
	if err != nil {
		log.Warnf("failed to get executable path: %v, using current directory", err)
		return "."
	}
	dir := filepath.Dir(exe)
	log.Verbosef("base directory: %s", dir)
	return dir
}

func findConfigArg(args []string, baseDir string) (string, bool) {
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			return resolvePath(args[i+1], baseDir), true
		}
		if strings.HasPrefix(a, "-c=") {
			return resolvePath(strings.TrimPrefix(a, "-c="), baseDir), true
		}
	}
	return resolvePath("quack-proxy.yaml", baseDir), false
}

func resolvePath(path, baseDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

func usage() {
	fmt.Fprint(os.Stderr, `quack-proxy — DuckDB Quack cluster manager

Usage:
  quack-proxy start [-c config.yaml]     Start daemon
  quack-proxy stop [-c config.yaml]       Stop daemon
  quack-proxy status [-c config.yaml]     Show shard status
  quack-proxy reload [-c config.yaml]     Hot-reload configuration
  quack-proxy gen-proxy [-c config.yaml]  Generate HAProxy config
  quack-proxy init-db [-c config.yaml]    Create/verify shard database files
  quack-proxy version                     Print version

Options:
  -c FILE          Config file path (default: quack-proxy.yaml)
  --verbose        Enable verbose logging
  --debug          Enable debug logging (includes SQL)
  --quiet          Quiet mode (errors only)
  --log-file FILE  Write logs to file
  --log-json       JSON log format
`)
}

// runInitDB creates any missing shard database files using the in-process
// DuckDB engine and verifies existing ones are valid DuckDB databases.
// This replaces the duckdb.exe CLI bootstrap — Option B has no CLI.
func runInitDB(cfgPath string, baseDir string, log *logger.Logger) {
	cfg, err := config.Load(cfgPath, baseDir, log)
	if err != nil {
		log.Errorf("failed to load config: %v", err)
		os.Exit(1)
	}

	for _, s := range cfg.Shards {
		existed := true
		if _, err := os.Stat(s.Database); os.IsNotExist(err) {
			existed = false
		}

		// The engine creates missing FILES, but not missing DIRECTORIES —
		// ensure the database's parent directory exists first.
		if err := os.MkdirAll(filepath.Dir(s.Database), 0755); err != nil {
			log.Errorf("shard '%s': create directory for %s: %v", s.Name, s.Database, err)
			os.Exit(1)
		}

		db, err := sql.Open("duckdb", s.Database)
		if err != nil {
			log.Errorf("shard '%s': open %s: %v", s.Name, s.Database, err)
			os.Exit(1)
		}
		db.SetMaxOpenConns(1)
		// Force real initialization — sql.Open is lazy and the file is
		// only created/validated on first use.
		if _, err := db.Exec("SELECT 1"); err != nil {
			db.Close()
			log.Errorf("shard '%s': %s is not a valid DuckDB database: %v", s.Name, s.Database, err)
			os.Exit(1)
		}
		db.Close()

		if existed {
			log.Infof("shard '%s': verified %s", s.Name, s.Database)
		} else {
			log.Infof("shard '%s': created %s", s.Name, s.Database)
		}
	}
	log.Infof("database initialisation complete")
}

func runStart(cfgPath string, baseDir string, log *logger.Logger) {
	log.Infof("Starting quack-proxy v%s", version)
	log.Verbosef("config file: %s", cfgPath)

	cfg, err := config.Load(cfgPath, baseDir, log)
	if err != nil {
		log.Errorf("failed to load config: %v", err)
		os.Exit(1)
	}

	// Validate database files exist
	if err := cfg.ValidateDatabases(log); err != nil {
		log.Errorf("database validation failed: %v", err)
		os.Exit(1)
	}

	// Ensure PID directory exists
	pidDir := filepath.Dir(cfg.Global.PIDFile)
	if err := os.MkdirAll(pidDir, 0755); err != nil {
		log.Errorf("failed to create PID directory: %v", err)
		os.Exit(1)
	}

	writePID(cfg.Global.PIDFile)
	sup := supervisor.New(cfg, log)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sup.StartAll(ctx); err != nil {
		log.Errorf("failed to start shards: %v", err)
		os.Exit(1)
	}

	go sup.HealthLoop(ctx)

	// Start the read-only admin HTTP endpoint in the background. It shares
	// the supervisor instance, so GET /status reports the LIVE shard state.
	// It serves until the process context is canceled (graceful shutdown).
	if cfg.Admin.Enabled != nil && *cfg.Admin.Enabled {
		adminSrv := admin.New(sup, version, log)
		adminAddr := fmt.Sprintf("%s:%d", cfg.Admin.BindHost, cfg.Admin.Port)
		go func() {
			log.Infof("admin endpoint listening on http://%s/status", adminAddr)
			if err := adminSrv.ListenAndServe(ctx, adminAddr); err != nil {
				log.Errorf("admin server: %v", err)
			}
		}()
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for sig := range sigCh {
		switch sig {
		case syscall.SIGHUP:
			log.Infof("reloading config")
			newCfg, err := config.Load(cfgPath, baseDir, log)
			if err != nil {
				log.Errorf("reload failed: %v", err)
				continue
			}
			log.Infof("config reloaded, shards: %d", len(newCfg.Shards))
			sup.ReArm()
		default:
			log.Infof("shutting down, signal: %v", sig)
			cancel()
			sup.StopAll()
			os.Remove(cfg.Global.PIDFile)
			log.Infof("shutdown complete")
			return
		}
	}
}

func runStop(cfgPath string, log *logger.Logger) {
	pidFile := getPIDFile(cfgPath)
	data, err := os.ReadFile(pidFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "quack-proxy is not running (no PID file)")
		os.Exit(1)
	}
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	proc, _ := os.FindProcess(pid)
	if proc != nil {
		proc.Signal(syscall.SIGTERM)
	}
	fmt.Println("sent SIGTERM to quack-proxy")
}

func runStatus(cfgPath string, log *logger.Logger) {
	// status only needs the admin address. With an explicit -c, a missing
	// or broken config is an error. Without one, a missing default config
	// falls back to the built-in admin defaults — the running proxy listens
	// there unless its own config overrides it.
	adminAddr := fmt.Sprintf("%s:%d", config.DefaultAdminBindHost, config.DefaultAdminPort)
	cfg, err := config.Load(cfgPath, "", log)
	if err == nil {
		host := cfg.Admin.BindHost
		if host == "0.0.0.0" || host == "" {
			// The server bound to all interfaces accepts loopback; probing
			// 0.0.0.0 directly as a client is unreliable (fails on Windows).
			host = "127.0.0.1"
		}
		adminAddr = fmt.Sprintf("%s:%d", host, cfg.Admin.Port)
	} else if configExplicit {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	// status is an HTTP client: it probes the proxy's admin endpoint so
	// the data comes from the LIVE process, not a fresh local supervisor.
	url := fmt.Sprintf("http://%s/status", adminAddr)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, "quack-proxy is not running")
		os.Exit(1)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		fmt.Fprintln(os.Stderr, "this proxy build does not expose /status")
		os.Exit(1)
	}
	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "unexpected status response: HTTP %d\n", resp.StatusCode)
		os.Exit(1)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read status response: %v\n", err)
		os.Exit(1)
	}

	// --json passes the remote payload through verbatim.
	for _, arg := range flag.Args() {
		if arg == "--json" {
			fmt.Println(string(body))
			return
		}
	}

	var snapshot admin.StatusResponse
	if err := json.Unmarshal(body, &snapshot); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse status response: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("%-16s %-6s %-10s %-10s %-8s %s\n", "NAME", "PORT", "STATUS", "UPTIME", "RESTARTS", "DATABASE")
	for _, s := range snapshot.Shards {
		uptime := (time.Duration(s.UptimeSeconds) * time.Second).String()
		fmt.Printf("%-16s %-6d %-10s %-10s %-8d %s\n",
			s.Name, s.Port, s.Status, uptime, s.Restarts, s.Database)
	}
}

func runReload(cfgPath string, log *logger.Logger) {
	pidFile := getPIDFile(cfgPath)
	data, _ := os.ReadFile(pidFile)
	var pid int
	fmt.Sscanf(string(data), "%d", &pid)
	proc, _ := os.FindProcess(pid)
	if proc != nil {
		proc.Signal(syscall.SIGHUP)
	}
	fmt.Println("sent SIGHUP to quack-proxy")
}

func runGenProxy(cfgPath string, log *logger.Logger) {
	cfg, err := config.Load(cfgPath, "", log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	if cfg.Proxy == nil || cfg.Proxy.Output == "" {
		fmt.Fprintln(os.Stderr, "proxy.output not configured")
		os.Exit(1)
	}
	sup := supervisor.New(cfg, log)
	if err := proxy.GenerateHAProxy(cfg, sup, cfg.Proxy.Output); err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("HAProxy config written to %s\n", cfg.Proxy.Output)
}

func writePID(path string) {
	dir := filepath.Dir(path)
	os.MkdirAll(dir, 0755)
	os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
}

func getPIDFile(cfgPath string) string {
	// Try to load config to get PID file path
	cfg, _ := config.Load(cfgPath, "", nil)
	if cfg != nil && cfg.Global.PIDFile != "" {
		return cfg.Global.PIDFile
	}
	return "/tmp/quack-proxy/quack-proxy.pid"
}
