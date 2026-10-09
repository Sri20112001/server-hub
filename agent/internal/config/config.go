// Package config loads agent settings from the process environment,
// optionally supplemented by a `.env` file. Lookup order:
//
//  1. Real environment variables (always win).
//  2. `.env` next to the agent executable (installer layout).
//  3. `.env` in the working directory (dev layout).
//
// Supported keys (same names as the legacy Node agent):
//
//	AGENT_TOKEN        required — token from ServerHub web UI
//	SERVERHUB_URL      required — base URL of the API, e.g. http://host:4000
//	METRICS_INTERVAL   optional — seconds between metric reports (default 60)
//	HEARTBEAT_INTERVAL optional — seconds between heartbeats (default 30)
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds everything the agent needs to run.
type Config struct {
	Token             string
	BaseURL           string
	MetricsInterval   time.Duration
	HeartbeatInterval time.Duration
}

// Load reads configuration, returning an error if a required value is missing.
func Load() (Config, error) {
	loadDotEnvFile(besideExecutable())
	loadDotEnvFile(inWorkingDir())

	cfg := Config{
		Token:             strings.TrimSpace(os.Getenv("AGENT_TOKEN")),
		BaseURL:           strings.TrimRight(strings.TrimSpace(os.Getenv("SERVERHUB_URL")), "/"),
		MetricsInterval:   envSeconds("METRICS_INTERVAL", 60),
		HeartbeatInterval: envSeconds("HEARTBEAT_INTERVAL", 30),
	}
	if cfg.Token == "" || cfg.BaseURL == "" {
		return Config{}, fmt.Errorf("AGENT_TOKEN and SERVERHUB_URL are required (env or .env)")
	}
	env := strings.ToLower(strings.TrimSpace(os.Getenv("ENVIRONMENT")))
	allowInsecure := strings.EqualFold(strings.TrimSpace(os.Getenv("ALLOW_INSECURE_HTTP")), "true")
	if env == "production" && !allowInsecure && !strings.HasPrefix(cfg.BaseURL, "https://") {
		return Config{}, fmt.Errorf("production environment requires HTTPS for SERVERHUB_URL (set ALLOW_INSECURE_HTTP=true to override for testing)")
	}
	if cfg.MetricsInterval <= 0 || cfg.HeartbeatInterval <= 0 {
		return Config{}, fmt.Errorf("METRICS_INTERVAL and HEARTBEAT_INTERVAL must be positive seconds")
	}
	return cfg, nil
}

func envSeconds(key string, def int) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return time.Duration(def) * time.Second
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return time.Duration(def) * time.Second
	}
	return time.Duration(n) * time.Second
}

// besideExecutable returns <exe-dir>/.env, or "" if unresolvable.
func besideExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Join(filepath.Dir(exe), ".env")
}

// inWorkingDir returns <cwd>/.env, or "" if unresolvable.
func inWorkingDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return filepath.Join(cwd, ".env")
}

// loadDotEnvFile parses a KEY=VALUE file and sets each key that is not
// already present in the environment. Missing files are silently ignored.
func loadDotEnvFile(path string) {
	if path == "" {
		return
	}
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Allow a leading `export ` for shell-compat files.
		line = strings.TrimPrefix(line, "export ")
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		val = strings.Trim(val, `"'`)
		if key == "" || val == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, val)
		}
	}
}
