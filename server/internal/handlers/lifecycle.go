package handlers

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"serverhub/internal/applog"
	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/notify"
	"serverhub/internal/ops"
)

// ProjectLifecycle runs docker-compose operations for a whole project.
// Start is medium risk; stop/restart are HIGH RISK and require confirm=true.
type ProjectLifecycle struct {
	DB     *database.DB
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
	code, body := h.runOne(u, pid, action, composeArgs)
	c.JSON(code, body)
}

// runOne executes one lifecycle action. Shared by the single-project routes
// and the bulk endpoint; responses keep the single-route shape.
func (h *ProjectLifecycle) runOne(u string, pid int64, action string, composeArgs []string) (int, gin.H) {
	var name, deployPath string
	if err := h.DB.QueryRow(`SELECT name, deployment_path FROM projects WHERE id=?`, pid).Scan(&name, &deployPath); err != nil {
		return http.StatusNotFound, gin.H{"error": "project not found"}
	}
	if deployPath == "" {
		audit.Write(h.DB, u, action, "project", strconv.FormatInt(pid, 10), "failed", "no deployment_path configured")
		return http.StatusBadRequest, gin.H{"error": "no deployment_path configured for this project"}
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
			return http.StatusBadRequest, gin.H{"error": "compose file not found in deployment_path"}
		}
	}
	var buf bytes.Buffer
	op, err := ops.Create(h.DB, "project."+action, "project", strconv.FormatInt(pid, 10), u, []string{action})
	if err != nil {
		return http.StatusInternalServerError, gin.H{"error": "could not track operation"}
	}
	emit := func(ok bool, detail string) {
		if h.Broker != nil {
			h.Broker.Publish("project."+action, map[string]interface{}{
				"projectId": pid, "project": name, "action": action,
				"operationId": op.ID, "ok": ok, "detail": detail,
			})
		}
	}
	projectID := uint(pid)
	fail := func(msg string, logs string) (int, gin.H) {
		_ = ops.Finish(h.DB, op.ID, "FAILED", msg)
		audit.Write(h.DB, u, action, "project", strconv.FormatInt(pid, 10), "failed", msg)
		// Persist the full compose output to the central DB log store
		// (previously it only went back in the HTTP response).
		if h.DB != nil && h.DB.GDB != nil {
			applog.Write(h.DB.GDB, applog.Entry{
				Level: "ERROR", Source: "project", Actor: u, Action: action,
				Resource: "project", ResourceID: strconv.FormatInt(pid, 10),
				ProjectID: &projectID,
				Message:   fmt.Sprintf("project %s %s failed: %s", name, action, msg),
				Metadata:  applog.Truncate(logs, 16000),
			})
		}
		emit(false, msg)
		notify.Send(h.DB, notify.EventProjectFailed,
			fmt.Sprintf("Project %s failed: %s", name, action), msg)
		return http.StatusBadGateway, gin.H{"error": msg, "logs": logs, "operationId": op.ID}
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
			return fail(fmt.Sprintf("docker compose %s failed", action), buf.String())
		}
	}
	_ = ops.Finish(h.DB, op.ID, "SUCCESS", "")
	audit.Write(h.DB, u, action, "project", strconv.FormatInt(pid, 10), "ok", "op="+op.ID)
	// Persist the compose output to the central DB log store (append-only).
	if h.DB != nil && h.DB.GDB != nil {
		applog.Write(h.DB.GDB, applog.Entry{
			Level: "INFO", Source: "project", Actor: u, Action: action,
			Resource: "project", ResourceID: strconv.FormatInt(pid, 10),
			ProjectID: &projectID,
			Message:   fmt.Sprintf("project %s %s ok (op=%s)", name, action, op.ID),
			Metadata:  applog.Truncate(buf.String(), 16000),
		})
	}
	emit(true, "op="+op.ID)
	return http.StatusOK, gin.H{"ok": true, "projectId": pid, "action": action, "logs": buf.String(), "operationId": op.ID}
}

var bulkLifecycleActions = map[string]struct {
	needConfirm bool
	args        []string
}{
	"start":   {false, []string{"up", "-d", "--remove-orphans"}},
	"stop":    {true, []string{"stop"}},
	"restart": {true, []string{"restart"}},
}

// POST /server-hub/api/projects/bulk-lifecycle — run one lifecycle action
// across many projects: {ids: [...], action: start|stop|restart, confirm}.
// stop/restart are high-risk and require confirm=true, mirroring the single
// routes. Runs sequentially and reports per-project results.
func (h *ProjectLifecycle) Bulk(c *gin.Context) {
	var body struct {
		IDs     []int64 `json:"ids"`
		Action  string  `json:"action"`
		Confirm bool    `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || len(body.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids (non-empty) and action are required"})
		return
	}
	spec, ok := bulkLifecycleActions[body.Action]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown action (want start, stop or restart)"})
		return
	}
	if spec.needConfirm && !body.Confirm {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("%sing projects is high-risk: retry with {\"confirm\": true}", body.Action)})
		return
	}
	u, _ := middleware.CurrentUser(c)
	results := []gin.H{}
	succeeded := 0
	seen := map[int64]bool{}
	for _, pid := range body.IDs {
		if seen[pid] {
			continue
		}
		seen[pid] = true
		var name string
		_ = h.DB.QueryRow(`SELECT name FROM projects WHERE id=?`, pid).Scan(&name)
		code, resp := h.runOne(u, pid, body.Action, spec.args)
		entry := gin.H{"id": pid, "name": name, "ok": code == http.StatusOK}
		if code == http.StatusOK {
			succeeded++
			if op, ok := resp["operationId"].(string); ok {
				entry["operationId"] = op
			}
		} else {
			if msg, ok := resp["error"].(string); ok {
				entry["error"] = msg
			}
		}
		results = append(results, entry)
	}
	audit.Write(h.DB, u, "bulk-"+body.Action, "project", "", "ok",
		"succeeded="+strconv.Itoa(succeeded)+"/"+strconv.Itoa(len(results)))
	c.JSON(http.StatusOK, gin.H{
		"ok": true, "action": body.Action,
		"succeeded": succeeded, "failed": len(results) - succeeded,
		"results": results,
	})
}
