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
	Port          string
	DBPath        string
	DatabaseURL   string // Postgres DSN. If set, Postgres is used; else SQLite at DBPath.
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
	HealthIntervalSec int
	LogRetentionDays  int // app_logs retention; 0 = keep forever
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
	return &Config{
		Port:            getenv("PORT", "4000"),
		DBPath:          getenv("DB_PATH", "./data/serverhub.db"),
		DatabaseURL:     os.Getenv("DATABASE_URL"),
		JWTSecret:       getenv("JWT_SECRET", "change-me-in-production-min-32-chars"),
		EncryptionKey:   encKey,
		CookieSecure:    secure,
		CookieDomain:    os.Getenv("COOKIE_DOMAIN"),
		FrontendURL:     getenv("FRONTEND_URL", "http://localhost:5173"),
		AdminUser:       getenv("ADMIN_USERNAME", "admin"),
		AdminPass:       getenv("ADMIN_PASSWORD", "changeme"),
		WebhookSecret:   os.Getenv("GITHUB_WEBHOOK_SECRET"),
		ScanRoots:       splitRoots(getenv("SCAN_ROOTS", "/srv/apps")),
		AlertCPU:        getenvFloat("ALERT_CPU_PCT", 85),
		AlertRAM:        getenvFloat("ALERT_RAM_PCT", 90),
		AlertDisk:       getenvFloat("ALERT_DISK_PCT", 80),
		HealthIntervalSec: interval,
		LogRetentionDays:  retention,
	}
}
