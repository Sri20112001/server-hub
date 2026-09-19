package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"serverhub/internal/ops"
)

type OperationsHandler struct {
	DB *sql.DB
}

// GET /server-hub/api/operations?limit=50
func (h *OperationsHandler) List(c *gin.Context) {
	limit := 50
	if v, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil {
		limit = v
	}
	list, err := ops.List(h.DB, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, list)
}

// GET /server-hub/api/operations/:id
func (h *OperationsHandler) Get(c *gin.Context) {
	op, err := ops.Get(h.DB, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "operation not found"})
		return
	}
	c.JSON(http.StatusOK, op)
}
