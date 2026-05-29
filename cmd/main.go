package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alitrack/quack-proxy/internal/config"
	"github.com/alitrack/quack-proxy/internal/proxy"
	"github.com/alitrack/quack-proxy/internal/supervisor"
)

func main() {
	flag.Usage = usage
	flag.Parse()

	args := flag.Args()
	if len(args) < 1 {
		usage()
		os.Exit(1)
	}

	cfgPath := findConfigArg(args)

	switch args[0] {
	case "start":
		runStart(cfgPath)
	case "stop":
		runStop(cfgPath)
	case "status":
		runStatus(cfgPath)
	case "reload":
		runReload(cfgPath)
	case "gen-proxy":
		runGenProxy(cfgPath)
	case "version":
		fmt.Println("quack-proxy v0.1.0")
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		usage()
		os.Exit(1)
	}
}

func findConfigArg(args []string) string {
	for i, a := range args {
		if a == "-c" && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(a, "-c=") {
			return strings.TrimPrefix(a, "-c=")
		}
	}
	return "quack-proxy.yaml"
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
`)
}

func runStart(cfgPath string) {
	logger := newLogger()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}
	writePID(cfg.Global.PIDFile)
	sup := supervisor.New(cfg, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := sup.StartAll(ctx); err != nil {
		logger.Error("failed to start shards", "error", err)
		os.Exit(1)
	}
	go sup.HealthLoop(ctx)
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for sig := range sigCh {
		switch sig {
		case syscall.SIGHUP:
			logger.Info("reloading config")
			newCfg, err := config.Load(cfgPath)
			if err != nil {
				logger.Error("reload failed", "error", err)
				continue
			}
			logger.Info("config reloaded", "shards", len(newCfg.Shards))
		default:
			logger.Info("shutting down", "signal", sig)
			cancel()
			sup.StopAll()
			os.Remove(cfg.Global.PIDFile)
			return
		}
	}
}

func runStop(cfgPath string) {
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

func runStatus(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	sup := supervisor.New(cfg, newLogger())
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

func runReload(cfgPath string) {
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

func runGenProxy(cfgPath string) {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}
	if cfg.Proxy == nil || cfg.Proxy.Output == "" {
		fmt.Fprintln(os.Stderr, "proxy.output not configured")
		os.Exit(1)
	}
	sup := supervisor.New(cfg, newLogger())
	if err := proxy.GenerateHAProxy(cfg, sup, cfg.Proxy.Output); err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("HAProxy config written to %s\n", cfg.Proxy.Output)
}

func writePID(path string) {
	dir := path[:len(path)-len("quack-proxy.pid")]
	os.MkdirAll(dir, 0755)
	os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644)
}

func getPIDFile(cfgPath string) string {
	cfg, _ := config.Load(cfgPath)
	if cfg != nil {
		return cfg.Global.PIDFile
	}
	return "/var/run/quack-proxy/quack-proxy.pid"
}

func newLogger() *slog.Logger {
	var w io.Writer = os.Stdout
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo}))
}
