package handlers

import (
	"database/sql"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/config"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/models"
	"serverhub/internal/ops"
)

type GatewayHandler struct {
	DB     *sql.DB
	Cfg    *config.Config
	Broker *events.Broker
}

func (h *GatewayHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`SELECT id,project_id,host,path_prefix,target,enabled,created_at,updated_at FROM gateway_routes ORDER BY path_prefix`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []models.GatewayRoute{}
	for rows.Next() {
		var r models.GatewayRoute
		var pid sql.NullInt64
		var enabled int
		if err := rows.Scan(&r.ID, &pid, &r.Host, &r.PathPrefix, &r.Target, &enabled, &r.CreatedAt, &r.UpdatedAt); err == nil {
			if pid.Valid {
				v := pid.Int64
				r.ProjectID = &v
			}
			r.Enabled = enabled == 1
			out = append(out, r)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *GatewayHandler) Create(c *gin.Context) {
	var r models.GatewayRoute
	if err := c.ShouldBindJSON(&r); err != nil || r.PathPrefix == "" || r.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pathPrefix and target are required"})
		return
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	var pid interface{}
	if r.ProjectID != nil {
		pid = *r.ProjectID
	}
	res, err := h.DB.Exec(`INSERT INTO gateway_routes (project_id,host,path_prefix,target,enabled) VALUES (?,?,?,?,?)`,
		pid, r.Host, r.PathPrefix, r.Target, enabled)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create", "gateway_route", strconv.FormatInt(id, 10), "ok", r.PathPrefix+" -> "+r.Target)
	h.getByID(c, id)
}

func (h *GatewayHandler) getByID(c *gin.Context, id int64) {
	var r models.GatewayRoute
	var pid sql.NullInt64
	var enabled int
	err := h.DB.QueryRow(`SELECT id,project_id,host,path_prefix,target,enabled,created_at,updated_at FROM gateway_routes WHERE id=?`, id).
		Scan(&r.ID, &pid, &r.Host, &r.PathPrefix, &r.Target, &enabled, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}
	if pid.Valid {
		v := pid.Int64
		r.ProjectID = &v
	}
	r.Enabled = enabled == 1
	c.JSON(http.StatusOK, r)
}

func (h *GatewayHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var r models.GatewayRoute
	if err := c.ShouldBindJSON(&r); err != nil || r.PathPrefix == "" || r.Target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pathPrefix and target are required"})
		return
	}
	enabled := 0
	if r.Enabled {
		enabled = 1
	}
	var pid interface{}
	if r.ProjectID != nil {
		pid = *r.ProjectID
	}
	_, err = h.DB.Exec(`UPDATE gateway_routes SET project_id=?,host=?,path_prefix=?,target=?,enabled=?,updated_at=datetime('now') WHERE id=?`,
		pid, r.Host, r.PathPrefix, r.Target, enabled, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "update", "gateway_route", strconv.FormatInt(id, 10), "ok", r.PathPrefix)
	h.getByID(c, id)
}

func (h *GatewayHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM gateway_routes WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "route not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "gateway_route", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Validate runs `caddy validate` if the binary exists, otherwise does a
// basic sanity check of the stored routes.
func (h *GatewayHandler) Validate(c *gin.Context) {
	if _, err := exec.LookPath("caddy"); err == nil {
		out, err := exec.Command("caddy", "validate", "--config", h.Cfg.CaddyfilePath, "--adapter", "caddyfile").CombinedOutput()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"valid": false, "output": string(out)})
			return
		}
		c.JSON(http.StatusOK, gin.H{"valid": true, "output": string(out)})
		return
	}
	// Fallback sanity check.
	rows, _ := h.DB.Query(`SELECT path_prefix,target FROM gateway_routes WHERE enabled=1`)
	defer func() {
		if rows != nil {
			rows.Close()
		}
	}()
	seen := map[string]bool{}
	for rows != nil && rows.Next() {
		var prefix, target string
		_ = rows.Scan(&prefix, &target)
		if !strings.HasPrefix(prefix, "/") || target == "" {
			c.JSON(http.StatusBadRequest, gin.H{"valid": false, "output": "invalid route: " + prefix})
			return
		}
		if seen[prefix] {
			c.JSON(http.StatusBadRequest, gin.H{"valid": false, "output": "duplicate prefix: " + prefix})
			return
		}
		seen[prefix] = true
	}
	c.JSON(http.StatusOK, gin.H{"valid": true, "output": "basic validation passed (caddy binary not installed, full validation skipped)"})
}

// Reload runs `caddy reload`. Treated as high-risk: audit-logged.
// Never binds port 80 itself.
func (h *GatewayHandler) Reload(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	op, err := ops.Create(h.DB, "gateway.reload", "gateway", "caddy", u, []string{"reload"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
		return
	}
	emit := func(ok bool, detail string) {
		if h.Broker != nil {
			h.Broker.Publish("gateway.reloaded", map[string]interface{}{
				"operationId": op.ID, "ok": ok, "detail": detail,
			})
		}
	}
	done := func(result, detail string, code int, body gin.H) {
		if result == "ok" {
			_ = ops.Finish(h.DB, op.ID, "SUCCESS", "")
		} else {
			_ = ops.Finish(h.DB, op.ID, "FAILED", detail)
		}
		audit.Write(h.DB, u, "reload", "gateway", "", result, detail)
		emit(result == "ok", detail)
		body["operationId"] = op.ID
		c.JSON(code, body)
	}
	_ = ops.Start(h.DB, op.ID)
	_ = ops.SetStage(h.DB, op.ID, 0, false, "")
	if _, err := exec.LookPath("caddy"); err != nil {
		done("failed", "caddy binary not found", http.StatusServiceUnavailable,
			gin.H{"error": "caddy binary not installed on this host"})
		return
	}
	out, err := exec.Command("caddy", "reload", "--config", h.Cfg.CaddyfilePath, "--adapter", "caddyfile").CombinedOutput()
	if err != nil {
		// Check the Caddyfile exists to give a better error.
		if _, serr := os.Stat(h.Cfg.CaddyfilePath); serr != nil {
			done("failed", "caddyfile missing", http.StatusBadRequest,
				gin.H{"error": "Caddyfile not found at " + h.Cfg.CaddyfilePath, "output": string(out)})
			return
		}
		done("failed", string(out), http.StatusInternalServerError,
			gin.H{"error": "caddy reload failed", "output": string(out)})
		return
	}
	done("ok", "", http.StatusOK, gin.H{"ok": true, "output": string(out)})
}
