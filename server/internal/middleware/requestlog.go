package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"serverhub/internal/applog"
)

// RequestLog persists one app_logs row per HTTP request (source=api).
// Health probes and SSE streams are skipped to avoid log spam.
func RequestLog(db *gorm.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		if path == "/health" || path == "/server-hub/api/events" {
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
