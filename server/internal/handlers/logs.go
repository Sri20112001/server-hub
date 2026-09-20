package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"serverhub/internal/applog"
)

// LogsHandler serves the central app_logs store — the single place to check
// all activity today, and the future log-aggregator's read API.
type LogsHandler struct {
	GDB *gorm.DB
}

// GET /server-hub/api/logs?level=ERROR&source=api&search=deploy&projectId=3&limit=100&offset=0
func (h *LogsHandler) List(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	var pid *uint
	if v := c.Query("projectId"); v != "" {
		if n, err := strconv.ParseUint(v, 10, 32); err == nil {
			u := uint(n)
			pid = &u
		}
	}
	rows, err := applog.Query(h.GDB, applog.Filter{
		Level:     c.Query("level"),
		Source:    c.Query("source"),
		Search:    c.Query("search"),
		ProjectID: pid,
		Limit:     limit,
		Offset:    offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, rows)
}
