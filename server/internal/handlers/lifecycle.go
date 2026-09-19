package handlers

import (
	"bytes"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/ops"
)

// ProjectLifecycle runs docker-compose operations for a whole project.
// Start is medium risk; stop/restart are HIGH RISK and require confirm=true.
type ProjectLifecycle struct {
	DB     *sql.DB
	Broker *events.Broker
}

func (h *ProjectLifecycle) Start(c *gin.Context) {
	h.run(c, "start", false, []string{"up", "-d", "--remove-orphans"})
}

func (h *ProjectLifecycle) Stop(c *gin.Context) {
	h.run(c, "stop", true, []string{"stop"})
}

func (h *ProjectLifecycle) Restart(c *gin.Context) {
	h.run(c, "restart", true, []string{"restart"})
}

func (h *ProjectLifecycle) run(c *gin.Context, action string, needConfirm bool, composeArgs []string) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	if needConfirm && !confirmed(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%sing a project is high-risk: retry with ?confirm=true or body {\"confirm\": true}", action)})
		return
	}
	var name, deployPath string
	if err := h.DB.QueryRow(`SELECT name, deployment_path FROM projects WHERE id=?`, pid).Scan(&name, &deployPath); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if deployPath == "" {
		audit.Write(h.DB, u, action, "project", strconv.FormatInt(pid, 10), "failed", "no deployment_path configured")
		c.JSON(http.StatusBadRequest, gin.H{"error": "no deployment_path configured for this project"})
		return
	}
	if _, err := os.Stat(filepath.Join(deployPath, "docker-compose.yml")); err != nil {
		// Fall back to whatever compose file the project declares.
		var composeFile string
		_ = h.DB.QueryRow(`SELECT compose_file FROM projects WHERE id=?`, pid).Scan(&composeFile)
		if composeFile == "" {
			composeFile = "docker-compose.yml"
		}
		if _, err := os.Stat(filepath.Join(deployPath, composeFile)); err != nil {
			audit.Write(h.DB, u, action, "project", strconv.FormatInt(pid, 10), "failed", "compose file missing")
			c.JSON(http.StatusBadRequest, gin.H{"error": "compose file not found in deployment_path"})
			return
		}
	}
	var buf bytes.Buffer
	op, err := ops.Create(h.DB, "project."+action, "project", strconv.FormatInt(pid, 10), u, []string{action})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
		return
	}
	emit := func(ok bool, detail string) {
		if h.Broker != nil {
			h.Broker.Publish("project."+action, map[string]interface{}{
				"projectId": pid, "project": name, "action": action,
				"operationId": op.ID, "ok": ok, "detail": detail,
			})
		}
	}
	fail := func(msg string, logs string) {
		_ = ops.Finish(h.DB, op.ID, "FAILED", msg)
		audit.Write(h.DB, u, action, "project", strconv.FormatInt(pid, 10), "failed", msg)
		emit(false, msg)
		c.JSON(http.StatusBadGateway, gin.H{"error": msg, "logs": logs, "operationId": op.ID})
	}
	_ = ops.Start(h.DB, op.ID)
	_ = ops.SetStage(h.DB, op.ID, 0, false, "")
	cmd := exec.Command("docker", append([]string{"compose"}, composeArgs...)...)
	cmd.Dir = deployPath
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	fmt.Fprintf(&buf, "$ docker compose %v (project %s)\n", composeArgs, name)
	if err := cmd.Run(); err != nil {
		// Legacy fallback: docker-compose binary.
		legacy := exec.Command("docker-compose", composeArgs...)
		legacy.Dir = deployPath
		legacy.Stdout = &buf
		legacy.Stderr = &buf
		if err2 := legacy.Run(); err2 != nil {
			_ = ops.SetStage(h.DB, op.ID, 0, true, "")
			fail(fmt.Sprintf("docker compose %s failed", action), buf.String())
			return
		}
	}
	_ = ops.Finish(h.DB, op.ID, "SUCCESS", "")
	audit.Write(h.DB, u, action, "project", strconv.FormatInt(pid, 10), "ok", "op="+op.ID)
	emit(true, "op="+op.ID)
	c.JSON(http.StatusOK, gin.H{"ok": true, "projectId": pid, "action": action, "logs": buf.String(), "operationId": op.ID})
}
