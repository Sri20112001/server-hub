// Package healthhttp implements the liveness/readiness split.
//
//   - Live: the process is running (always 200). Suitable for container
//     liveness probes and load-balancer health checks that must not flap
//     on downstream outages.
//   - Ready: the app can serve requests (200, or 503 "degraded" when a
//     required dependency is down). Suitable for readiness gates.
//   - /health stays as a backward-compatible readiness alias.
package healthhttp

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Pinger reports whether a required dependency is reachable.
type Pinger func() error

// LiveHandler always returns 200 while the process runs.
func LiveHandler(start time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"status":    "ok",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"uptimeSec": int64(time.Since(start).Seconds()),
		})
	}
}

// ReadyHandler returns 200 when ping succeeds, 503 otherwise. The payload
// shape matches the historic /health response plus a degraded status.
func ReadyHandler(ping Pinger, start time.Time) gin.HandlerFunc {
	return func(c *gin.Context) {
		dbStatus := "up"
		code := http.StatusOK
		status := "ok"
		if err := ping(); err != nil {
			dbStatus = "down"
			code = http.StatusServiceUnavailable
			status = "degraded"
		}
		c.JSON(code, gin.H{
			"status":    status,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
			"uptimeSec": int64(time.Since(start).Seconds()),
			"checks":    gin.H{"db": dbStatus},
		})
	}
}
