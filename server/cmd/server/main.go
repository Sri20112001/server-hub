package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"serverhub/internal/alerting"
	"serverhub/internal/applog"
	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/events"
	"serverhub/internal/handlers"
	"serverhub/internal/health"
	"serverhub/internal/healthcheck"
	"serverhub/internal/middleware"
	"serverhub/internal/monitoring"
	"serverhub/internal/notify"
	"serverhub/internal/telemetry"
)

func main() {
	// Load server/.env for local dev (optional — Docker Compose injects
	// real environment variables instead, which take precedence).
	if err := godotenv.Load(); err != nil {
		log.Println("no .env file loaded, using process environment")
	}
	cfg := config.Load()

	// Startup warnings go to stdout now; DB mirrors are written after the
	// database opens below (see bootWarningsToDB) so every warning is also
	// stored in app_logs.
	if os.Getenv("SERVERHUB_ENCRYPTION_KEY") == "" {
		log.Println("WARNING: SERVERHUB_ENCRYPTION_KEY not set — using ephemeral key. Secrets will NOT survive restarts. Set a stable 64-char hex key in production.")
	}
	if len(cfg.JWTSecret) < 32 {
		log.Println("WARNING: JWT_SECRET is short — use at least 32 characters in production.")
	}

	// Postgres is the only store. DATABASE_URL is required; the server
	// fails fast without it.
	db, err := database.OpenDatabase(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	gdb := db.GDB
	log.Printf("connected to postgres")
	// Log tables are append-only and retained forever: no pruning, no
	// wipe, no delete. LOG_RETENTION_DAYS is ignored (see config).
	if cfg.LogRetentionDays > 0 {
		log.Printf("log retention: append-only mode, ignoring LOG_RETENTION_DAYS=%d (logs kept forever)", cfg.LogRetentionDays)
		applog.Warn(gdb, "system", "append-only mode: ignoring LOG_RETENTION_DAYS, logs kept forever")
	}
	if err := ensureAdmin(db, cfg.AdminUser, cfg.AdminPass); err != nil {
		log.Fatalf("seed admin: %v", err)
	}
	// Migrate a legacy single-recipient email setup into the default
	// notification group (idempotent; no-op when already migrated).
	notify.EnsureDefaultGroup(db)
	// Mirror boot + startup warnings to the central DB log store
	// (stdout alone is not enough — all logs must live in the DB).
	bootWarningsToDB(gdb)
	applog.Info(gdb, "system", "serverhub boot (postgres)")

	dockerClient := dockerx.New()
	broker := events.NewBroker()
	health.StartLoop(db, broker, cfg.HealthIntervalSec)
	telemetry.StartLoop(db, broker, telemetry.Thresholds{
		CPU: cfg.AlertCPU, RAM: cfg.AlertRAM, Disk: cfg.AlertDisk,
	})
	alerting.StartLoop(db, broker, alerting.Thresholds{
		CPU: cfg.AlertCPU, RAM: cfg.AlertRAM, Disk: cfg.AlertDisk,
		OfflineAfter: time.Duration(cfg.OfflineTimeoutSec) * time.Second,
	})
	log.Printf("server offline timeout: %ds", cfg.OfflineTimeoutSec)
	healthcheck.StartLoop(db, broker)

	promClient := monitoring.NewPrometheusClient(cfg.PrometheusURL, cfg.PrometheusTimeoutSec)
	amClient := monitoring.NewAlertmanagerClient(cfg.AlertmanagerURL, cfg.AlertmanagerTimeoutSec)
	if promClient.Available() {
		log.Printf("Prometheus configured at %s", cfg.PrometheusURL)
		applog.Info(gdb, "system", "prometheus configured at "+cfg.PrometheusURL)
	}
	if amClient.Available() {
		log.Printf("Alertmanager configured at %s", cfg.AlertmanagerURL)
		applog.Info(gdb, "system", "alertmanager configured at "+cfg.AlertmanagerURL)
	}

	r := gin.Default()
	// FRONTEND_URL may hold a comma-separated list of allowed origins
	// (e.g. "http://localhost:6540,http://localhost:5173").
	origins := []string{"http://localhost:5173", "http://localhost:3000"}
	for _, o := range strings.Split(cfg.FrontendURL, ",") {
		if o = strings.TrimSpace(o); o != "" && !slices.Contains(origins, o) {
			origins = append(origins, o)
		}
	}
	r.Use(cors.New(cors.Config{
		AllowOrigins:     origins,
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		AllowCredentials: true,
	}))
	r.Use(middleware.RequestLog(gdb))

	startTime := time.Now()
	r.GET("/health", func(c *gin.Context) {
		dbStatus := "up"
		if err := db.Ping(); err != nil {
			dbStatus = "down"
		}
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"uptimeSec": int64(time.Since(startTime).Seconds()),
			"checks":    gin.H{"db": dbStatus},
		})
	})

	authH := &handlers.AuthHandler{DB: db, Cfg: cfg}
	loginLimiter := middleware.NewLoginLimiter()
	r.POST("/server-hub/api/auth/login", middleware.LoginRateLimit(gdb, loginLimiter), authH.Login)
	r.POST("/server-hub/api/auth/token", middleware.LoginRateLimit(gdb, loginLimiter), authH.Token)
	r.POST("/server-hub/api/auth/refresh", authH.Refresh)
	r.POST("/server-hub/api/auth/logout", middleware.AuthRequired(cfg.JWTSecret), authH.Logout)
	r.GET("/server-hub/api/auth/me", middleware.AuthRequired(cfg.JWTSecret), authH.Me)
	r.PUT("/server-hub/api/auth/password", middleware.AuthRequired(cfg.JWTSecret), authH.ChangePassword)
	r.POST("/server-hub/api/auth/logout-all", middleware.AuthRequired(cfg.JWTSecret), authH.LogoutAll)

	// Role-based route groups (see middleware/roles.go for the matrix):
	// viewer = read-only, operator = +routine mutations,
	// admin = +destructive & privileged operations.
	authd := middleware.AuthRequired(cfg.JWTSecret)
	viewer := r.Group("/server-hub/api", authd, middleware.RequireRole(middleware.RoleViewer))
	operator := r.Group("/server-hub/api", authd, middleware.RequireRole(middleware.RoleOperator))
	admin := r.Group("/server-hub/api", authd, middleware.RequireRole(middleware.RoleAdmin))
	execH := &handlers.ExecHandler{DB: db, Docker: dockerClient, Broker: broker}
	handlers.SetWSAllowedOrigins(origins)
	{
		projH := &handlers.ProjectHandler{DB: db}
		viewer.GET("/projects", projH.List)
		operator.POST("/projects", projH.Create)
		viewer.GET("/projects/:id", projH.Get)
		operator.PUT("/projects/:id", projH.Update)
		operator.DELETE("/projects/:id", projH.Delete)

		svcH := &handlers.ServiceHandler{DB: db}
		viewer.GET("/projects/:id/services", svcH.ListByProject)
		operator.POST("/projects/:id/services", svcH.Create)
		operator.PUT("/services/:id", svcH.Update)
		operator.DELETE("/services/:id", svcH.Delete)

		ctrH := &handlers.ContainerHandler{DB: db, Docker: dockerClient, Broker: broker}
		viewer.GET("/containers", ctrH.List)
		viewer.GET("/containers/:id", ctrH.Inspect)
		viewer.GET("/containers/:id/logs", ctrH.Logs)
		viewer.GET("/containers/:id/stats", ctrH.Stats)
		operator.POST("/containers/:id/start", ctrH.Start)
		admin.POST("/containers/:id/stop", ctrH.Stop)
		admin.POST("/containers/:id/restart", ctrH.Restart)
		viewer.GET("/images", ctrH.Images)
		viewer.GET("/volumes", ctrH.Volumes)
		viewer.GET("/containers/stats", ctrH.StatsAll)

		discH := &handlers.DiscoveryHandler{DB: db, Docker: dockerClient, Cfg: cfg, Broker: broker}
		viewer.GET("/discovery", discH.Scan)
		operator.POST("/discovery/import", discH.Import)
		operator.POST("/discovery/import-all", discH.ImportAll)

		sysH := &handlers.SystemHandler{DB: db, Docker: dockerClient, Cfg: cfg}
		viewer.GET("/health", sysH.Health)
		viewer.GET("/projects/:id/health", sysH.ProjectHealth)
		viewer.GET("/server", sysH.Server)
		viewer.GET("/server/detail", sysH.Detail)
		viewer.GET("/server/storage", sysH.Storage)
		viewer.GET("/dashboard", sysH.Dashboard)
		viewer.GET("/audit", sysH.Audit)

		depH := &handlers.DeploymentHandler{DB: db, Broker: broker}
		viewer.GET("/deployments", depH.ListAll)
		admin.DELETE("/deployments", depH.Wipe)
		viewer.GET("/projects/:id/deployments", depH.ListByProject)
		operator.POST("/projects/:id/deployments", depH.Create)
		operator.POST("/projects/:id/deploy", depH.Deploy)
		operator.POST("/projects/:id/deployments/:depId/rollback", depH.Rollback)

		telH := &handlers.TelemetryHandler{DB: db}
		viewer.GET("/telemetry", telH.History)
		viewer.GET("/telemetry/latest", telH.Latest)

		lifeH := &handlers.ProjectLifecycle{DB: db, Broker: broker}
		operator.POST("/projects/:id/start", lifeH.Start)
		admin.POST("/projects/:id/stop", lifeH.Stop)
		admin.POST("/projects/:id/restart", lifeH.Restart)
		admin.POST("/projects/bulk-lifecycle", lifeH.Bulk)

		secH := &handlers.SecretHandler{DB: db, Cfg: cfg}
		viewer.GET("/projects/:id/secrets", secH.List)
		operator.POST("/projects/:id/secrets", secH.Upsert)
		admin.PUT("/secrets/:id", secH.Update)
		admin.DELETE("/secrets/:id", secH.Delete)
		admin.POST("/secrets/:id/reveal", secH.Reveal)



		viewer.GET("/events", broker.Stream)

		opH := &handlers.OperationsHandler{DB: db}
		viewer.GET("/operations", opH.List)
		viewer.GET("/operations/:id", opH.Get)

		// Container shells are admin-only: the single-use grant token keeps
		// the WebSocket itself capability-gated, but minting requires admin.
		admin.POST("/containers/:id/exec", execH.Create)

		backH := &handlers.BackupsHandler{DB: db, Broker: broker, Dir: backupDir()}
		viewer.GET("/projects/:id/backups", backH.List)
		operator.POST("/projects/:id/backups", backH.Create)
		viewer.GET("/backups/:id", backH.Get)
		viewer.GET("/backups/:id/download", backH.Download)
		operator.DELETE("/backups/:id", backH.Delete)
		operator.POST("/backups/:id/restore", backH.Restore)

		// Central log store: single place for all activity + future aggregator.
		logsH := &handlers.LogsHandler{GDB: gdb}
		viewer.GET("/logs", logsH.List)

		// Databases: auto-detect servers, browse, save connections, register.
		dbH := &handlers.DatabasesHandler{DB: db, Docker: dockerClient, Cfg: cfg}
		viewer.GET("/databases/servers", dbH.Servers)
		operator.POST("/databases/browse", dbH.Browse)
		operator.POST("/databases/connect", dbH.Connect)
		operator.POST("/databases/register", dbH.Register)

		// Notification channels (Telegram / email) for failure + pressure signals.
		ntH := &handlers.NotificationsHandler{DB: db}
		viewer.GET("/settings/notifications", ntH.Get)
		admin.PUT("/settings/notifications", ntH.Update)
		admin.POST("/settings/notifications/test", ntH.Test)

		// User + role management (admin only; last-admin guards in handler).
		usrH := &handlers.UserHandler{DB: db}
		admin.GET("/users", usrH.List)
		admin.POST("/users", usrH.Create)
		admin.PUT("/users/:username/role", usrH.SetRole)
		admin.DELETE("/users/:username", usrH.Delete)

		// ── Managed servers (Phase 1-9) ──────────────────────────────────
		srvH := &handlers.ServersHandler{DB: db, Broker: broker, Cfg: cfg}
		viewer.GET("/servers", srvH.List)
		operator.POST("/servers", srvH.Create)
		viewer.GET("/servers/:id", srvH.Get)
		operator.PATCH("/servers/:id", srvH.Update)
		admin.DELETE("/servers/:id", srvH.Delete)
		viewer.GET("/servers/:id/metrics", srvH.Metrics)
		viewer.GET("/servers/:id/metrics/latest", srvH.LatestMetrics)
		admin.GET("/servers/:id/tokens", srvH.ListTokens)
		admin.POST("/servers/:id/tokens", srvH.CreateToken)
		admin.DELETE("/servers/:id/tokens/:tokenId", srvH.RevokeToken)

		// Server groups
		grpH := &handlers.ServerGroupsHandler{DB: db}
		viewer.GET("/server-groups", grpH.List)
		operator.POST("/server-groups", grpH.Create)
		admin.DELETE("/server-groups/:id", grpH.Delete)

		// Notification groups (Phase 1: multi-recipient email)
		ngH := &handlers.NotificationGroupsHandler{DB: db}
		viewer.GET("/notification-groups", ngH.List)
		viewer.GET("/notification-groups/:id", ngH.Get)
		operator.POST("/notification-groups", ngH.Create)
		operator.PATCH("/notification-groups/:id", ngH.Update)
		operator.POST("/notification-groups/:id/members", ngH.AddMember)
		operator.DELETE("/notification-groups/:id/members/:memberId", ngH.RemoveMember)
		admin.DELETE("/notification-groups/:id", ngH.Delete)

		// Alerts
		altH := &handlers.AlertsHandler{DB: db}
		viewer.GET("/alerts", altH.List)
		operator.PATCH("/alerts/:id/resolve", altH.Resolve)

		// In-app notifications
		notifH := &handlers.NotificationsInAppHandler{DB: db}
		viewer.GET("/notifications", notifH.List)
		viewer.PATCH("/notifications/:id/read", notifH.MarkRead)
		viewer.POST("/notifications/read-all", notifH.MarkAllRead)

		// Health checks
		hcH := &handlers.HealthChecksHandler{DB: db}
		viewer.GET("/health-checks", hcH.List)
		operator.POST("/health-checks", hcH.Create)
		operator.PATCH("/health-checks/:id", hcH.Update)
		admin.DELETE("/health-checks/:id", hcH.Delete)
		viewer.GET("/health-checks/:id/results", hcH.Results)

		// ── Prometheus + Alertmanager monitoring ─────────────────────────────
		monH := &handlers.MonitoringHandler{
			DB: db, Cfg: cfg, Broker: broker,
			Prometheus: promClient, Alertmgr: amClient,
		}
		// Prometheus proxy (viewer+)
		viewer.GET("/monitoring/prometheus/status", monH.PrometheusStatus)
		viewer.GET("/monitoring/prometheus/targets", monH.PrometheusTargets)
		viewer.GET("/monitoring/prometheus/rules", monH.PrometheusRules)
		// Raw PromQL restricted to admin (arbitrary query = privileged)
		admin.GET("/monitoring/prometheus/query", monH.PrometheusQuery)
		admin.GET("/monitoring/prometheus/query-range", monH.PrometheusQueryRange)
		// High-level monitoring APIs (viewer+)
		viewer.GET("/monitoring/overview", monH.Overview)
		viewer.GET("/monitoring/metrics", monH.Metrics)
		// Alertmanager proxy (viewer+)
		viewer.GET("/monitoring/alertmanager/status", monH.AlertmanagerStatus)
		viewer.GET("/monitoring/alerts", monH.AlertmanagerAlerts)
		viewer.GET("/monitoring/silences", monH.ListSilences)
		operator.POST("/monitoring/silences", monH.CreateSilence)
		operator.DELETE("/monitoring/silences/:id", monH.DeleteSilence)
	}

	// Agent endpoints — authenticated by agent token, not user JWT.
	agentMw := middleware.AgentAuth(db)
	agentGrp := r.Group("/server-hub/api/agent", agentMw)
	{
		agentSrvH := &handlers.ServersHandler{DB: db, Broker: broker, Cfg: cfg}
		agentGrp.POST("/heartbeat", agentSrvH.Heartbeat)
		agentGrp.POST("/metrics", agentSrvH.IngestMetrics)
	}

	// Container exec session (single-use token auth, no session cookie).
	r.GET("/server-hub/api/exec/:token", execH.Attach)

	// Public GitHub webhook (HMAC-signed, no session required).
	whH := &handlers.WebhookHandler{DB: db, Cfg: cfg, Broker: broker}
	r.POST("/server-hub/api/webhooks/github", whH.GitHub)

	// Alertmanager webhook (shared-secret authenticated, no session required).
	monWebhookH := &handlers.MonitoringHandler{
		DB: db, Cfg: cfg, Broker: broker,
		Prometheus: promClient, Alertmgr: amClient,
	}
	r.POST("/server-hub/api/webhooks/alertmanager", monWebhookH.AlertmanagerWebhook)

	// Serve frontend if FRONTEND_DIR is set (e.g. /app/client/dist)
	if frontendDir := os.Getenv("FRONTEND_DIR"); frontendDir != "" {
		log.Printf("Serving static frontend from %s", frontendDir)
		applog.Info(gdb, "system", "serving static frontend from "+frontendDir)
		// Map Vite's base path /server-hub/assets to the physical assets folder
		r.Static("/server-hub/assets", filepath.Join(frontendDir, "assets"))
		r.StaticFile("/server-hub/favicon.png", filepath.Join(frontendDir, "favicon.png"))
		
		// All other routes fallback to index.html (SPA)
		r.NoRoute(func(c *gin.Context) {
			// Only fallback to index.html if it's under /server-hub or / (to prevent shadowing API)
			if strings.HasPrefix(c.Request.URL.Path, "/server-hub/") || c.Request.URL.Path == "/server-hub" || c.Request.URL.Path == "/" {
				c.File(filepath.Join(frontendDir, "index.html"))
			} else {
				c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			}
		})
	} else {
		// Default NoRoute if frontend isn't bundled
		r.NoRoute(func(c *gin.Context) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		})
	}

	log.Printf("ServerHub API listening on :%s (db=postgres docker=%v)", cfg.Port, dockerClient.Available())
	log.Printf("CORS allowed origins: %v", origins)
	applog.Info(gdb, "system", "API listening (db=postgres docker="+boolStr(dockerClient.Available())+")")
	if err := r.Run(":" + cfg.Port); err != nil {
		applog.Error(gdb, "system", "server exited: "+err.Error())
		log.Fatal(err)
	}
}

// bootWarningsToDB mirrors process-environment warnings into app_logs so
// they are stored in the DB, not just stdout.
func bootWarningsToDB(gdb *gorm.DB) {
	if os.Getenv("SERVERHUB_ENCRYPTION_KEY") == "" {
		applog.Warn(gdb, "system", "startup warning: SERVERHUB_ENCRYPTION_KEY not set — using ephemeral key")
	}
	// JWTSecret length is checked by the caller config; mirror generically
	// without leaking the value.
	if v := os.Getenv("JWT_SECRET"); v != "" && len(v) < 32 {
		applog.Warn(gdb, "auth", "startup warning: JWT_SECRET is short — use at least 32 characters in production")
	}
}

func boolStr(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// backupDir resolves BACKUP_DIR or defaults next to the database.
func backupDir() string {
	if v := os.Getenv("BACKUP_DIR"); v != "" {
		return v
	}
	return "./data/backups"
}

func ensureAdmin(db *database.DB, username, password string) error {	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, username).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?,'admin')`, username, string(hash))
	if err == nil {
		log.Printf("Seeded admin user %q (change ADMIN_PASSWORD immediately)", username)
	}
	_ = os.Stdout.Sync()
	return err
}
