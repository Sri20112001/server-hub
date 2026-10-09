package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"serverhub/internal/applog"
)

// skipRequestLog reports whether path must not produce a per-hit app_logs
// row: health probes, the SSE event stream, and single-use exec capability
// URLs. Exec tokens are secrets in the URL path; persisting them would leak
// live session credentials to every viewer able to read /api/logs.
func skipRequestLog(path string) bool {
	if path == "/health" || path == "/health/live" || path == "/health/ready" ||
		path == "/server-hub/api/events" {
		return true
	}
	return strings.HasPrefix(path, "/server-hub/api/exec/")
}

// RequestLog persists one app_logs row per HTTP request (source=api) into
// the append-only central log store. Health probes (/health, /health/live,
// /health/ready) and long-lived SSE streams (/server-hub/api/events) are
// intentionally not stored per-hit (they would flood the immutable store);
// their state transitions remain fully logged via audit -> app_logs mirroring.
func RequestLog(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if skipRequestLog(path) {
			c.Next()
			return
		}
		c.Next()
		latency := time.Since(start).Milliseconds()
		status := c.Writer.Status()
		level := "INFO"
		if status >= 500 {
			level = "ERROR"
		} else if status >= 400 {
			level = "WARN"
		}
		user, _ := CurrentUser(c)
		applog.Write(db, applog.Entry{
			Level:      level,
			Source:     "api",
			Actor:      user,
			Action:     c.Request.Method,
			Resource:   "http",
			Message:    c.Request.Method + " " + path + " -> " + httpStatusText(status),
			Method:     c.Request.Method,
			Path:       path,
			StatusCode: status,
			LatencyMs:  latency,
		})
	}
}

func httpStatusText(code int) string {
	switch {
	case code >= 500:
		return "server error"
	case code >= 400:
		return "client error"
	case code >= 300:
		return "redirect"
	default:
		return "ok"
	}
}
