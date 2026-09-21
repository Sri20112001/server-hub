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

	"serverhub/internal/applog"
	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/events"
	"serverhub/internal/handlers"
	"serverhub/internal/health"
	"serverhub/internal/middleware"
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
	r.POST("/server-hub/api/auth/login", authH.Login)
	r.POST("/server-hub/api/auth/logout", authH.Logout)
	r.GET("/server-hub/api/auth/me", middleware.AuthRequired(cfg.JWTSecret), authH.Me)
	r.PUT("/server-hub/api/auth/password", middleware.AuthRequired(cfg.JWTSecret), authH.ChangePassword)

	api := r.Group("/server-hub/api", middleware.AuthRequired(cfg.JWTSecret))
	execH := &handlers.ExecHandler{DB: db, Docker: dockerClient, Broker: broker}
	{
		projH := &handlers.ProjectHandler{DB: db}
		api.GET("/projects", projH.List)
		api.POST("/projects", projH.Create)
		api.GET("/projects/:id", projH.Get)
		api.PUT("/projects/:id", projH.Update)
		api.DELETE("/projects/:id", projH.Delete)

		svcH := &handlers.ServiceHandler{DB: db}
		api.GET("/projects/:id/services", svcH.ListByProject)
		api.POST("/projects/:id/services", svcH.Create)
		api.PUT("/services/:id", svcH.Update)
		api.DELETE("/services/:id", svcH.Delete)

		ctrH := &handlers.ContainerHandler{DB: db, Docker: dockerClient, Broker: broker}
		api.GET("/containers", ctrH.List)
		api.GET("/containers/:id", ctrH.Inspect)
		api.GET("/containers/:id/logs", ctrH.Logs)
		api.GET("/containers/:id/stats", ctrH.Stats)
		api.POST("/containers/:id/start", ctrH.Start)
		api.POST("/containers/:id/stop", ctrH.Stop)
		api.POST("/containers/:id/restart", ctrH.Restart)
		api.GET("/images", ctrH.Images)
		api.GET("/volumes", ctrH.Volumes)
		api.GET("/containers/stats", ctrH.StatsAll)

		discH := &handlers.DiscoveryHandler{DB: db, Docker: dockerClient, Cfg: cfg, Broker: broker}
		api.GET("/discovery", discH.Scan)
		api.POST("/discovery/import", discH.Import)

		sysH := &handlers.SystemHandler{DB: db, Docker: dockerClient, Cfg: cfg}
		api.GET("/health", sysH.Health)
		api.GET("/projects/:id/health", sysH.ProjectHealth)
		api.GET("/server", sysH.Server)
		api.GET("/server/detail", sysH.Detail)
		api.GET("/server/storage", sysH.Storage)
		api.GET("/dashboard", sysH.Dashboard)
		api.GET("/audit", sysH.Audit)

		depH := &handlers.DeploymentHandler{DB: db, Broker: broker}
		api.GET("/deployments", depH.ListAll)
		api.DELETE("/deployments", depH.Wipe)
		api.GET("/projects/:id/deployments", depH.ListByProject)
		api.POST("/projects/:id/deployments", depH.Create)
		api.POST("/projects/:id/deploy", depH.Deploy)
		api.POST("/projects/:id/deployments/:depId/rollback", depH.Rollback)

		telH := &handlers.TelemetryHandler{DB: db}
		api.GET("/telemetry", telH.History)
		api.GET("/telemetry/latest", telH.Latest)

		lifeH := &handlers.ProjectLifecycle{DB: db, Broker: broker}
		api.POST("/projects/:id/start", lifeH.Start)
		api.POST("/projects/:id/stop", lifeH.Stop)
		api.POST("/projects/:id/restart", lifeH.Restart)

		secH := &handlers.SecretHandler{DB: db, Cfg: cfg}
		api.GET("/projects/:id/secrets", secH.List)
		api.POST("/projects/:id/secrets", secH.Upsert)
		api.PUT("/secrets/:id", secH.Update)
		api.DELETE("/secrets/:id", secH.Delete)
		api.POST("/secrets/:id/reveal", secH.Reveal)



		api.GET("/events", broker.Stream)

		opH := &handlers.OperationsHandler{DB: db}
		api.GET("/operations", opH.List)
		api.GET("/operations/:id", opH.Get)

		api.POST("/containers/:id/exec", execH.Create)

		backH := &handlers.BackupsHandler{DB: db, Broker: broker, Dir: backupDir()}
		api.GET("/projects/:id/backups", backH.List)
		api.POST("/projects/:id/backups", backH.Create)
		api.GET("/backups/:id", backH.Get)
		api.DELETE("/backups/:id", backH.Delete)
		api.POST("/backups/:id/restore", backH.Restore)

		// Central log store: single place for all activity + future aggregator.
		logsH := &handlers.LogsHandler{GDB: gdb}
		api.GET("/logs", logsH.List)
	}

	// Container exec session (single-use token auth, no session cookie).
	r.GET("/server-hub/api/exec/:token", execH.Attach)

	// Public GitHub webhook (HMAC-signed, no session required).
	whH := &handlers.WebhookHandler{DB: db, Cfg: cfg, Broker: broker}
	r.POST("/server-hub/api/webhooks/github", whH.GitHub)

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
