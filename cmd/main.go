package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

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
)

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

	cfgPath := findConfigArg(args, baseDir)

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
	case "version":
		fmt.Println("quack-proxy v0.1.0")
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

func findConfigArg(args []string, baseDir string) string {
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			return resolvePath(args[i+1], baseDir)
		}
		if strings.HasPrefix(a, "-c=") {
			return resolvePath(strings.TrimPrefix(a, "-c="), baseDir)
		}
	}
	return resolvePath("quack-proxy.yaml", baseDir)
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

func runStart(cfgPath string, baseDir string, log *logger.Logger) {
	log.Infof("Starting quack-proxy v0.1.0")
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
	cfg, err := config.Load(cfgPath, "", log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	sup := supervisor.New(cfg, log)
	shards := sup.Status()

	for _, arg := range flag.Args() {
		if arg == "--json" {
			output := struct {
				Shards               []supervisor.ShardProcess `json:"shards"`
				CoordinatorAttachSQL string                    `json:"coordinator_attach_sql"`
			}{shards, sup.AttachSQL()}
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			enc.Encode(output)
			return
		}
	}

	fmt.Printf("%-16s %-6s %-10s %-10s %-8s %s\n", "NAME", "PORT", "STATUS", "UPTIME", "RESTARTS", "DATABASE")
	for _, s := range shards {
		uptime := time.Since(s.StartTime).Round(time.Second)
		fmt.Printf("%-16s %-6d %-10s %-10s %-8d %s\n",
			s.Config.Name, s.Config.Port, s.Status, uptime, s.Restarts, s.Config.Database)
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