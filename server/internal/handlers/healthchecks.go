package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
)

type HealthChecksHandler struct {
	DB *database.DB
}

func (h *HealthChecksHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`
		SELECT id,name,type,target,interval,timeout,expected_status,enabled,status,
		       response_time_ms,last_checked_at,server_id,created_at,updated_at
		FROM health_checks ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		m := scanHealthCheck(rows)
		if m != nil {
			out = append(out, m)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *HealthChecksHandler) Create(c *gin.Context) {
	var body struct {
		Name           string `json:"name"`
		Type           string `json:"type"`
		Target         string `json:"target"`
		Interval       int    `json:"interval"`
		Timeout        int    `json:"timeout"`
		ExpectedStatus int    `json:"expectedStatus"`
		ServerID       *uint  `json:"serverId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" || body.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and target are required"})
		return
	}
	if body.Type == "" {
		body.Type = "http"
	}
	if body.Interval <= 0 {
		body.Interval = 60
	}
	if body.Timeout <= 0 {
		body.Timeout = 10
	}
	if body.ExpectedStatus == 0 {
		body.ExpectedStatus = 200
	}
	id, err := h.DB.InsertID(`
		INSERT INTO health_checks (name,type,target,interval,timeout,expected_status,enabled,status,server_id,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,true,'UNKNOWN',$7,NOW(),NOW())`,
		body.Name, body.Type, body.Target, body.Interval, body.Timeout, body.ExpectedStatus, body.ServerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create", "health_check", strconv.FormatInt(id, 10), "ok", body.Name)
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": body.Name})
}

func (h *HealthChecksHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Name           *string `json:"name"`
		Target         *string `json:"target"`
		Interval       *int    `json:"interval"`
		Timeout        *int    `json:"timeout"`
		ExpectedStatus *int    `json:"expectedStatus"`
		Enabled        *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err = h.DB.Exec(`
		UPDATE health_checks SET
		  name=COALESCE($1,name),
		  target=COALESCE($2,target),
		  interval=COALESCE($3,interval),
		  timeout=COALESCE($4,timeout),
		  expected_status=COALESCE($5,expected_status),
		  enabled=COALESCE($6,enabled),
		  updated_at=NOW()
		WHERE id=$7`,
		body.Name, body.Target, body.Interval, body.Timeout, body.ExpectedStatus, body.Enabled, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HealthChecksHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	_, _ = h.DB.Exec(`DELETE FROM health_check_results WHERE health_check_id=$1`, id)
	_, _ = h.DB.Exec(`DELETE FROM health_checks WHERE id=$1`, id)
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "health_check", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *HealthChecksHandler) Results(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rows, err := h.DB.Query(`
		SELECT id,health_check_id,timestamp,status,response_time_ms,error
		FROM health_check_results WHERE health_check_id=$1
		ORDER BY timestamp DESC LIMIT 100`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var rid, hcid uint
		var ts time.Time
		var status, errMsg string
		var ms int64
		if err := rows.Scan(&rid, &hcid, &ts, &status, &ms, &errMsg); err == nil {
			out = append(out, gin.H{
				"id": rid, "healthCheckId": hcid, "timestamp": ts,
				"status": status, "responseTimeMs": ms, "error": errMsg,
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

type hcRow interface {
	Scan(...any) error
}

func scanHealthCheck(rows hcRow) gin.H {
	var id uint
	var name, typ, target, status string
	var interval, timeout, expectedStatus int
	var enabled bool
	var rtMs *int64
	var lastChecked *time.Time
	var serverID *uint
	var createdAt, updatedAt time.Time
	if err := rows.Scan(&id, &name, &typ, &target, &interval, &timeout, &expectedStatus,
		&enabled, &status, &rtMs, &lastChecked, &serverID, &createdAt, &updatedAt); err != nil {
		return nil
	}
	return gin.H{
		"id": id, "name": name, "type": typ, "target": target,
		"interval": interval, "timeout": timeout, "expectedStatus": expectedStatus,
		"enabled": enabled, "status": status, "responseTimeMs": rtMs,
		"lastCheckedAt": lastChecked, "serverId": serverID,
		"createdAt": createdAt, "updatedAt": updatedAt,
	}
}
