package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port        string
	DatabaseURL string // Required Postgres DSN, e.g. postgres://serverhub:changeme@localhost:5432/serverhub?sslmode=disable.
	JWTSecret     string
	EncryptionKey string // hex-encoded 32 bytes
	CookieSecure  bool
	CookieDomain  string
	FrontendURL   string
	AdminUser     string
	AdminPass     string
	WebhookSecret string
	ScanRoots     []string
	AlertCPU      float64
	AlertRAM      float64
	AlertDisk     float64
	// OfflineTimeoutSec marks a managed server OFFLINE/DISCONNECTED when
	// no heartbeat was received within this many seconds.
	OfflineTimeoutSec int
	HealthIntervalSec int
	// LogRetentionDays is deprecated and ignored: all log tables are
	// append-only and retained forever (DB triggers reject DELETE/UPDATE).
	LogRetentionDays int
	// Prometheus / Alertmanager integration (optional).
	PrometheusURL              string
	PrometheusTimeoutSec       int
	AlertmanagerURL            string
	AlertmanagerTimeoutSec     int
	AlertmanagerWebhookSecret  string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitRoots(s string) []string {
	var out []string
	for _, r := range strings.Split(s, ",") {
		if r = strings.TrimSpace(r); r != "" {
			out = append(out, r)
		}
	}
	return out
}

func getenvFloat(key string, def float64) float64 {
	if v := os.Getenv(key); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil && f > 0 {
			return f
		}
	}
	return def
}

func Load() *Config {
	encKey := os.Getenv("SERVERHUB_ENCRYPTION_KEY")
	if encKey == "" {
		// Generate ephemeral key (secrets won't survive restart unless env is set).
		// Operator is warned via log in main.
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		encKey = hex.EncodeToString(b)
	}
	interval, _ := strconv.Atoi(getenv("HEALTH_CHECK_INTERVAL_SEC", "60"))
	if interval <= 0 {
		interval = 60
	}
	secure := getenv("COOKIE_SECURE", "false") == "true"
	retention, _ := strconv.Atoi(getenv("LOG_RETENTION_DAYS", "30"))

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		fmt.Fprintln(os.Stderr, "FATAL: JWT_SECRET environment variable is required")
		os.Exit(1)
	}
	if len(jwtSecret) < 32 {
		fmt.Fprintln(os.Stderr, "FATAL: JWT_SECRET must be at least 32 characters")
		os.Exit(1)
	}

	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminPass == "" {
		fmt.Fprintln(os.Stderr, "FATAL: ADMIN_PASSWORD environment variable is required")
		os.Exit(1)
	}

	promTimeout, _ := strconv.Atoi(getenv("PROMETHEUS_TIMEOUT_SEC", "10"))
	if promTimeout <= 0 {
		promTimeout = 10
	}
	amTimeout, _ := strconv.Atoi(getenv("ALERTMANAGER_TIMEOUT_SEC", "10"))
	if amTimeout <= 0 {
		amTimeout = 10
	}
	offlineTimeout, _ := strconv.Atoi(getenv("SERVER_OFFLINE_TIMEOUT_SEC", "180"))
	if offlineTimeout < 30 {
		offlineTimeout = 180
	}
	return &Config{
		Port:        getenv("PORT", "4000"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		JWTSecret:       jwtSecret,
		EncryptionKey:   encKey,
		CookieSecure:    secure,
		CookieDomain:    os.Getenv("COOKIE_DOMAIN"),
		FrontendURL:     getenv("FRONTEND_URL", "http://localhost:5173"),
		AdminUser:       getenv("ADMIN_USERNAME", "admin"),
		AdminPass:       adminPass,
		WebhookSecret:   os.Getenv("GITHUB_WEBHOOK_SECRET"),
		ScanRoots:       splitRoots(getenv("SCAN_ROOTS", "/srv/apps")),
		AlertCPU:        getenvFloat("ALERT_CPU_PCT", 85),
		AlertRAM:        getenvFloat("ALERT_RAM_PCT", 90),
		AlertDisk:       getenvFloat("ALERT_DISK_PCT", 80),
		OfflineTimeoutSec: offlineTimeout,
		HealthIntervalSec: interval,
		LogRetentionDays:  retention,
		PrometheusURL:             os.Getenv("PROMETHEUS_URL"),
		PrometheusTimeoutSec:      promTimeout,
		AlertmanagerURL:           os.Getenv("ALERTMANAGER_URL"),
		AlertmanagerTimeoutSec:    amTimeout,
		AlertmanagerWebhookSecret: os.Getenv("ALERTMANAGER_WEBHOOK_SECRET"),
	}
}
