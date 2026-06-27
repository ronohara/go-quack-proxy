// Package config parses and validates quack-proxy.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/alitrack/quack-proxy/internal/logger"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Global   GlobalConfig    `yaml:"global"`
	Listener ListenerConfig  `yaml:"listener"`
	Shards   []ShardConfig   `yaml:"shards"`
	Proxy    *ProxyConfig     `yaml:"proxy"`
}

type GlobalConfig struct {
	LogLevel   string `yaml:"log_level"`
	PIDFile    string `yaml:"pid_file"`
	StatusFile string `yaml:"status_file"`
}

type ListenerConfig struct {
	BindHost       string        `yaml:"bind_host"`
	PortStart      int           `yaml:"port_start"`
	HealthPath     string        `yaml:"health_path"`
	HealthInterval time.Duration `yaml:"health_interval"`
}

type ShardConfig struct {
	Name     string `yaml:"name"`
	Database string `yaml:"database"`
	Port     int    `yaml:"port"`
	Token    string `yaml:"token"`
	ReadOnly bool   `yaml:"readonly"`
}

type ProxyConfig struct {
	Enabled   bool       `yaml:"enabled"`
	Output    string     `yaml:"output"`
	BindPort  int        `yaml:"bind_port"`
	Mode      string     `yaml:"mode"`
	SSL       *SSLConfig `yaml:"ssl"`
}

type SSLConfig struct {
	Enabled bool   `yaml:"enabled"`
	Cert    string `yaml:"cert"`
}

// Load reads and parses the config file, resolving paths relative to baseDir.
func Load(path string, baseDir string, log *logger.Logger) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	cfg.applyDefaults(baseDir)
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("validate config: %w", err)
	}

	if log != nil {
		log.Verbosef("config loaded: %d shard(s)", len(cfg.Shards))
	}

	return cfg, nil
}

func (c *Config) applyDefaults(baseDir string) {
	if c.Global.LogLevel == "" {
		c.Global.LogLevel = "info"
	}
	if c.Global.PIDFile == "" {
		c.Global.PIDFile = filepath.Join(baseDir, "run", "quack-proxy.pid")
	}
	if c.Global.StatusFile == "" {
		c.Global.StatusFile = filepath.Join(baseDir, "run", "status.json")
	}
	if c.Listener.BindHost == "" {
		c.Listener.BindHost = "0.0.0.0"
	}
	if c.Listener.PortStart == 0 {
		c.Listener.PortStart = 9491
	}
	if c.Listener.HealthPath == "" {
		c.Listener.HealthPath = "/"
	}
	if c.Listener.HealthInterval == 0 {
		c.Listener.HealthInterval = 5 * time.Second
	}

	// Resolve paths relative to baseDir
	c.Global.PIDFile = resolvePath(c.Global.PIDFile, baseDir)
	c.Global.StatusFile = resolvePath(c.Global.StatusFile, baseDir)

	for i := range c.Shards {
		if c.Shards[i].Port == 0 {
			c.Shards[i].Port = c.Listener.PortStart + i
		}
		c.Shards[i].Database = resolvePath(c.Shards[i].Database, baseDir)
	}

	if c.Proxy != nil && c.Proxy.Mode == "" {
		c.Proxy.Mode = "roundrobin"
	}
	if c.Proxy != nil && c.Proxy.Output != "" {
		c.Proxy.Output = resolvePath(c.Proxy.Output, baseDir)
	}
}

func resolvePath(path, baseDir string) string {
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(baseDir, path)
}

func (c *Config) Validate() error {
	if len(c.Shards) == 0 {
		return fmt.Errorf("at least one shard is required")
	}

	ports := make(map[int]bool)
	for i, s := range c.Shards {
		if s.Name == "" {
			return fmt.Errorf("shard[%d]: name is required", i)
		}
		if s.Database == "" {
			return fmt.Errorf("shard[%d] (%s): database path is required", i, s.Name)
		}
		if s.Port <= 0 || s.Port > 65535 {
			return fmt.Errorf("shard[%d] (%s): invalid port %d", i, s.Name, s.Port)
		}
		if ports[s.Port] {
			return fmt.Errorf("shard[%d] (%s): duplicate port %d", i, s.Name, s.Port)
		}
		ports[s.Port] = true
	}

	return nil
}

// ValidateDatabases checks that all database files exist.
func (c *Config) ValidateDatabases(log *logger.Logger) error {
	for _, s := range c.Shards {
		if log != nil {
			log.Verbosef("checking database: %s", s.Database)
		}
		if _, err := os.Stat(s.Database); os.IsNotExist(err) {
			return fmt.Errorf("database file not found: %s", s.Database)
		}
	}
	if log != nil {
		log.Verbosef("all databases validated")
	}
	return nil
}