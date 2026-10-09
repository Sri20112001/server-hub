package config

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
)

type Config struct {
	Port        string
	DatabaseURL string // Required Postgres DSN, e.g. postgres://serverhub:changeme@localhost:5432/serverhub?sslmode=disable.
	// Env is "development" by default; set SERVERHUB_ENV=production (or
	// APP_ENV / GO_ENV) to enable production fail-fast checks.
	Env           string
	IsProduction  bool
	JWTSecret     string
	EncryptionKey string // hex-encoded 32 bytes
	CookieSecure  bool
	CookieDomain  string
	FrontendURL   string
	// FrontendOrigins holds normalized CORS origins (scheme + host + port,
	// never a path) derived from FRONTEND_ORIGIN, falling back to
	// FRONTEND_URL for backward compatibility.
	FrontendOrigins []string
	AdminUser     string
	AdminPass     string
	WebhookSecret string
	ScanRoots     []string
	// DeployRoots bounds operator-controlled deployment_path values:
	// deploy, rollback, lifecycle and restore resolve the configured path
	// (symlinks included) and refuse anything outside these roots.
	// DEPLOY_ROOTS defaults to SCAN_ROOTS so production always has roots.
	DeployRoots []string
	// EgressStrict applies strict SSRF policy (deny loopback/private/
	// link-local destinations) to health polling. Default false: metadata-IP
	// + no-redirect guardrails are always on, but private targets stay
	// allowed because monitoring internal services is the feature's job.
	EgressStrict bool
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
	PrometheusEnabled          bool
	AlertmanagerURL            string
	AlertmanagerTimeoutSec     int
	AlertmanagerEnabled        bool
	AlertmanagerWebhookSecret  string
	// NotificationRulesEnabled gates the Phase 2 rules engine. Default
	// false: with no (or disabled) rules the legacy notification pipeline
	// runs exactly as before.
	NotificationRulesEnabled bool
	// MetricsRetentionDays bounds server_metrics history. Default 30 (the
	// largest UI/API range); clamped to 1..365, invalid values fall back
	// to the default so a bad value can never wipe everything.
	MetricsRetentionDays int
	// AppLogRetentionDays bounds operational app_logs history (default 90).
	// Must be >= database.AppLogsPruneFloorDays: the DB trigger rejects
	// deletes of newer rows regardless of this setting. audit_logs is
	// never pruned (fully immutable, no exception).
	AppLogRetentionDays int
	// HealthResultRetentionDays bounds health_check_results history.
	// Default 30; raw probe data, not audit evidence.
	HealthResultRetentionDays int
	// DeliveryRetentionDays bounds terminal delivery outbox rows, attempts
	// and flushed digests (default 90). Firing incident states are never
	// pruned; audit_logs are never touched.
	DeliveryRetentionDays int
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

// SplitOrigins parses a comma-separated origin list into valid CORS origins.
// A valid origin is scheme + host + optional port only: entries carrying a
// URL path, query, or fragment (e.g. "https://example.com/server-hub") are
// rejected — browsers never send paths in Origin headers, so such values can
// never match and only create a false sense of configuration. A lone trailing
// slash is stripped. Returns the accepted origins and the rejected inputs.
func SplitOrigins(s string) (valid, rejected []string) {
	for _, part := range strings.Split(s, ",") {
		raw := strings.TrimSpace(part)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			rejected = append(rejected, raw)
			continue
		}
		if u.RawQuery != "" || u.Fragment != "" || (u.EscapedPath() != "" && u.EscapedPath() != "/") {
			rejected = append(rejected, raw)
			continue
		}
		origin := u.Scheme + "://" + u.Host
		if !slices.Contains(valid, origin) {
			valid = append(valid, origin)
		}
	}
	return valid, rejected
}

// frontendOriginsRaw prefers FRONTEND_ORIGIN, falling back to FRONTEND_URL
// so existing deployments keep working.
func frontendOriginsRaw() (raw string, strict bool) {
	if v := strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")); v != "" {
		return v, true
	}
	return os.Getenv("FRONTEND_URL"), false
}

// resolveFrontendOrigins computes the CORS allow-list. Explicit
// FRONTEND_ORIGIN values are validated strictly (paths rejected). The
// legacy FRONTEND_URL fallback is normalized leniently — a value like
// "https://imagesoft.in/server-hub" yields "https://imagesoft.in" — with a
// warning recommending FRONTEND_ORIGIN, so existing production deployments
// keep (and actually gain) working CORS instead of silently failing it.
func resolveFrontendOrigins() []string {
	raw, strict := frontendOriginsRaw()
	if strict {
		return mustOrigins(raw)
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		u, err := url.Parse(p)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			fmt.Fprintf(os.Stderr, "WARNING: ignoring invalid CORS origin %q (origins must be scheme+host, no path)\n", p)
			continue
		}
		if u.RawQuery != "" || u.Fragment != "" || (u.EscapedPath() != "" && u.EscapedPath() != "/") {
			fmt.Fprintf(os.Stderr, "WARNING: FRONTEND_URL %q contains a path; using origin %q — set FRONTEND_ORIGIN instead\n",
				p, u.Scheme+"://"+u.Host)
		}
		origin := u.Scheme + "://" + u.Host
		if !slices.Contains(out, origin) {
			out = append(out, origin)
		}
	}
	return out
}

// mustOrigins normalizes configured origins, warning about rejected values.
// Empty configuration yields no origins (callers add localhost defaults).
func mustOrigins(raw string) []string {
	valid, rejected := SplitOrigins(raw)
	for _, r := range rejected {
		fmt.Fprintf(os.Stderr, "WARNING: ignoring invalid CORS origin %q (origins must be scheme+host, no path)\n", r)
	}
	return valid
}

// resolveEnv returns the runtime environment name (lower-cased) and whether
// production hardening applies. SERVERHUB_ENV wins, then APP_ENV, then GO_ENV.
func resolveEnv() (string, bool) {
	env := getenv("SERVERHUB_ENV", getenv("APP_ENV", getenv("GO_ENV", "development")))
	env = strings.TrimSpace(strings.ToLower(env))
	if env == "" {
		env = "development"
	}
	return env, env == "production" || env == "prod"
}

// validEncryptionKey reports whether s is 64 hex chars (32 bytes for AES-256-GCM).
func validEncryptionKey(s string) bool {
	if len(s) != 64 {
		return false
	}
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32
}

func Load() *Config {
	env, isProd := resolveEnv()

	encKey := strings.TrimSpace(os.Getenv("SERVERHUB_ENCRYPTION_KEY"))
	if encKey == "" {
		if isProd {
			fmt.Fprintln(os.Stderr, "FATAL: SERVERHUB_ENCRYPTION_KEY is required in production (stable 64-char hex); refusing to start with an ephemeral key")
			os.Exit(1)
		}
		// Development convenience only: ephemeral key (secrets won't
		// survive restart). Operator is warned via log in main.
		b := make([]byte, 32)
		_, _ = rand.Read(b)
		encKey = hex.EncodeToString(b)
	} else if !validEncryptionKey(encKey) {
		fmt.Fprintln(os.Stderr, "FATAL: SERVERHUB_ENCRYPTION_KEY must be 64 hex characters (32 bytes)")
		os.Exit(1)
	}
	interval, _ := strconv.Atoi(getenv("HEALTH_CHECK_INTERVAL_SEC", "60"))
	if interval <= 0 {
		interval = 60
	}
	secure := getenv("COOKIE_SECURE", "false") == "true"
	if isProd && !secure {
		fmt.Fprintln(os.Stderr, "FATAL: COOKIE_SECURE must be true in production (API is served over HTTPS via reverse proxy)")
		os.Exit(1)
	}
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
	// Optional integrations stay exactly as before unless explicitly
	// disabled: a configured URL means enabled.
	promEnabled := getenv("PROMETHEUS_ENABLED", "true")
	amEnabled := getenv("ALERTMANAGER_ENABLED", "true")
	rulesEnabled := getenv("NOTIFICATION_RULES_ENABLED", "false")
	retentionDays, _ := strconv.Atoi(getenv("SERVER_METRICS_RETENTION_DAYS", "30"))
	if retentionDays < 1 || retentionDays > 365 {
		retentionDays = 30
	}
	appLogDays, _ := strconv.Atoi(getenv("APPLOG_RETENTION_DAYS", "90"))
	if appLogDays < 30 || appLogDays > 3650 {
		appLogDays = 90
	}
	healthResDays, _ := strconv.Atoi(getenv("HEALTHCHECK_RESULTS_RETENTION_DAYS", "30"))
	if healthResDays < 1 || healthResDays > 3650 {
		healthResDays = 30
	}
	deliveryDays, _ := strconv.Atoi(getenv("DELIVERY_RETENTION_DAYS", "90"))
	if deliveryDays < 1 || deliveryDays > 3650 {
		deliveryDays = 90
	}
	offlineTimeout, _ := strconv.Atoi(getenv("SERVER_OFFLINE_TIMEOUT_SEC", "180"))
	if offlineTimeout < 30 {
		offlineTimeout = 180
	}
	scanRoots := splitRoots(getenv("SCAN_ROOTS", "/srv/apps"))
	deployRoots := splitRoots(getenv("DEPLOY_ROOTS", getenv("SCAN_ROOTS", "/srv/apps")))
	strictEgress := getenv("HEALTHCHECK_EGRESS_STRICT", "false")
	return &Config{
		Port:        getenv("PORT", "4000"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		Env:           env,
		IsProduction:  isProd,
		JWTSecret:       jwtSecret,
		EncryptionKey:   encKey,
		CookieSecure:    secure,
		CookieDomain:    os.Getenv("COOKIE_DOMAIN"),
		FrontendURL:     getenv("FRONTEND_URL", "http://localhost:5173"),
		FrontendOrigins: resolveFrontendOrigins(),
		AdminUser:       getenv("ADMIN_USERNAME", "admin"),
		AdminPass:       adminPass,
		WebhookSecret:   os.Getenv("GITHUB_WEBHOOK_SECRET"),
		ScanRoots:       scanRoots,
		DeployRoots:     deployRoots,
		EgressStrict:    strictEgress == "true" || strictEgress == "1",
		AlertCPU:        getenvFloat("ALERT_CPU_PCT", 85),
		AlertRAM:        getenvFloat("ALERT_RAM_PCT", 90),
		AlertDisk:       getenvFloat("ALERT_DISK_PCT", 80),
		OfflineTimeoutSec: offlineTimeout,
		HealthIntervalSec: interval,
		LogRetentionDays:  retention,
		PrometheusURL:             os.Getenv("PROMETHEUS_URL"),
		PrometheusTimeoutSec:      promTimeout,
		PrometheusEnabled:         promEnabled != "false" && promEnabled != "0",
		AlertmanagerURL:           os.Getenv("ALERTMANAGER_URL"),
		AlertmanagerTimeoutSec:    amTimeout,
		AlertmanagerEnabled:       amEnabled != "false" && amEnabled != "0",
		NotificationRulesEnabled: rulesEnabled == "true" || rulesEnabled == "1",
		MetricsRetentionDays:    retentionDays,
		AppLogRetentionDays:     appLogDays,
		HealthResultRetentionDays: healthResDays,
		DeliveryRetentionDays:   deliveryDays,
		AlertmanagerWebhookSecret: os.Getenv("ALERTMANAGER_WEBHOOK_SECRET"),
	}
}
