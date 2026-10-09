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
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/deploypath"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/models"
	"serverhub/internal/notify"
	"serverhub/internal/ops"
	"serverhub/internal/safehttp"
)

type DeploymentHandler struct {
	DB     *database.DB
	Broker *events.Broker
	// DeployRoots bounds deployment_path (see deploypath).
	DeployRoots []string
	// StrictEgress applies strict SSRF policy to deploy health polling.
	StrictEgress bool
}

// ExecPolicy bundles the trust-boundary inputs for compose execution.
type ExecPolicy struct {
	Roots        []string
	StrictEgress bool
}

func (h *DeploymentHandler) policy() ExecPolicy {
	return ExecPolicy{Roots: h.DeployRoots, StrictEgress: h.StrictEgress}
}

var deployStages = []string{"Pull images", "Start services", "Health check"}

func (h *DeploymentHandler) ListAll(c *gin.Context) {
	limit := 50
	if v, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	rows, err := h.DB.Query(`SELECT id,project_id,commit_sha,branch,trigger,status,started_at,completed_at,duration_sec,logs FROM deployments ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list deployments"})
		return
	}
	defer rows.Close()
	c.JSON(http.StatusOK, scanDeployments(rows))
}

func (h *DeploymentHandler) ListByProject(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	rows, err := h.DB.Query(`SELECT id,project_id,commit_sha,branch,trigger,status,started_at,completed_at,duration_sec,logs FROM deployments WHERE project_id=? ORDER BY id DESC LIMIT 100`, pid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list deployments"})
		return
	}
	defer rows.Close()
	c.JSON(http.StatusOK, scanDeployments(rows))
}

func scanDeployments(rows *sql.Rows) []models.Deployment {
	out := []models.Deployment{}
	for rows.Next() {
		var d models.Deployment
		var dur sql.NullInt64
		var started, completed sql.NullString
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.Trigger, &d.Status, &started, &completed, &dur, &d.Logs); err == nil {
			d.StartedAt, d.CompletedAt = nullStr(started), nullStr(completed)
			if dur.Valid {
				v := dur.Int64
				d.DurationSec = &v
			}
			out = append(out, d)
		}
	}
	return out
}

func scanDeploymentRow(row interface {
	Scan(dest ...interface{}) error
}, d *models.Deployment) {
	var dur sql.NullInt64
	var started, completed sql.NullString
	if err := row.Scan(&d.ID, &d.ProjectID, &d.CommitSHA, &d.Branch, &d.Trigger, &d.Status, &started, &completed, &dur, &d.Logs); err == nil {
		d.StartedAt, d.CompletedAt = nullStr(started), nullStr(completed)
		if dur.Valid {
			v := dur.Int64
			d.DurationSec = &v
		}
	}
}

// Wipe is disabled: deployment history contains log records and the
// deployments table is delete-protected (DB trigger rejects DELETE).
// The attempt itself is audit-logged (append-only) and a 410 is returned
// so old clients fail closed instead of silently deleting history.
func (h *DeploymentHandler) Wipe(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "wipe-blocked", "deployments", "", "blocked",
		"deployment history is append-only and cannot be wiped")
	c.JSON(http.StatusGone, gin.H{"error": "deployment history is append-only and cannot be wiped"})
}

func (h *DeploymentHandler) Create(c *gin.Context) {
	// Manual record: POST /api/projects/:id/deployments
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	var body struct {
		CommitSHA string `json:"commitSha"`
		Branch    string `json:"branch"`
		Trigger   string `json:"trigger"`
		Status    string `json:"status"`
		Logs      string `json:"logs"`
	}
	_ = c.ShouldBindJSON(&body)
	if body.Trigger == "" {
		body.Trigger = "manual"
	}
	if body.Status == "" {
		body.Status = "SUCCESS"
	}
	id, err := h.DB.InsertID(`INSERT INTO deployments (project_id,commit_sha,branch,trigger,status,started_at,completed_at,logs) VALUES (?,?,?,?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,?)`,
		pid, body.CommitSHA, body.Branch, body.Trigger, body.Status, body.Logs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "record-deployment", "deployment", strconv.FormatInt(id, 10), "ok", body.CommitSHA)
	var d models.Deployment
	scanDeploymentRow(h.DB.QueryRow(`SELECT id,project_id,commit_sha,branch,trigger,status,started_at,completed_at,duration_sec,logs FROM deployments WHERE id=?`, id), &d)
	c.JSON(http.StatusCreated, d)
}

// Deploy triggers a staged `docker compose pull && up -d` in the project's
// deployment path and returns the deployment row plus an operation id the
// frontend polls for stage progress.
func (h *DeploymentHandler) Deploy(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	var body struct {
		CommitSHA string `json:"commitSha"`
		Branch    string `json:"branch"`
	}
	_ = c.ShouldBindJSON(&body)

	var name, deployPath, composeFile, branch, healthURL string
	err = h.DB.QueryRow(`SELECT name, deployment_path, compose_file, branch, health_url FROM projects WHERE id=?`, pid).
		Scan(&name, &deployPath, &composeFile, &branch, &healthURL)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
		return
	}
	if body.Branch != "" {
		branch = body.Branch
	}
	u, _ := middleware.CurrentUser(c)

	if deployID, err := h.DB.InsertID(`INSERT INTO deployments (project_id,commit_sha,branch,trigger,status,started_at) VALUES (?,?,?,?, 'RUNNING', CURRENT_TIMESTAMP)`,
		pid, body.CommitSHA, branch, "manual@serverhub"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create deployment"})
		return
	} else {
		audit.Write(h.DB, u, "deploy", "project", strconv.FormatInt(pid, 10), "started", body.CommitSHA)

		op, err := ops.Create(h.DB, "deploy", "project", strconv.FormatInt(pid, 10), u, deployStages)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
			return
		}

		go h.runDeploy(op.ID, deployID, pid, name, deployPath, composeFile, u, body.CommitSHA, healthURL)

		var d models.Deployment
		scanDeploymentRow(h.DB.QueryRow(`SELECT id,project_id,commit_sha,branch,trigger,status,started_at,completed_at,duration_sec,logs FROM deployments WHERE id=?`, deployID), &d)
		c.JSON(http.StatusAccepted, gin.H{"deployment": d, "operationId": op.ID})
	}
}

func (h *DeploymentHandler) runDeploy(opID string, deployID, projectID int64, projectName, deployPath, composeFile, actor, commit, healthURL string) {
	executeDeploy(h.DB, h.Broker, h.policy(), opID, deployID, projectID, projectName, deployPath, composeFile, actor, commit, healthURL)
}

// Rollback re-runs `docker compose up` for a previous deployment record:
// POST /server-hub/api/projects/:id/deployments/:depId/rollback
func (h *DeploymentHandler) Rollback(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	depID, err := strconv.ParseInt(c.Param("depId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid deployment id"})
		return
	}
	var commit, branch, name, deployPath, composeFile, healthURL string
	err = h.DB.QueryRow(`SELECT d.commit_sha, d.branch, p.name, p.deployment_path, p.compose_file, p.health_url
		FROM deployments d JOIN projects p ON p.id = d.project_id
		WHERE d.id=? AND d.project_id=?`, depID, pid).
		Scan(&commit, &branch, &name, &deployPath, &composeFile, &healthURL)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "deployment not found for this project"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	newID, err := h.DB.InsertID(`INSERT INTO deployments (project_id,commit_sha,branch,trigger,status,started_at)
		VALUES (?,?,?,?, 'RUNNING', CURRENT_TIMESTAMP)`,
		pid, commit, branch, "rollback:"+strconv.FormatInt(depID, 10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	audit.Write(h.DB, u, "rollback", "deployment", strconv.FormatInt(newID, 10), "started",
		"target="+strconv.FormatInt(depID, 10)+" commit="+commit)
	op, err := ops.Create(h.DB, "rollback", "project", strconv.FormatInt(pid, 10), u, deployStages)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
		return
	}
	go h.runDeploy(op.ID, newID, pid, name, deployPath, composeFile, u, commit, healthURL)

	var d models.Deployment
	scanDeploymentRow(h.DB.QueryRow(`SELECT id,project_id,commit_sha,branch,trigger,status,started_at,completed_at,duration_sec,logs FROM deployments WHERE id=?`, newID), &d)
	// Honest semantics: this re-runs `compose up` against the project's
	// CURRENT files — it does not check out the recorded commit. The note
	// keeps the API honest without renaming the endpoint.
	c.JSON(http.StatusAccepted, gin.H{"deployment": d, "operationId": op.ID,
		"note": "rollback reapplies the project's current compose files (redeploy); it does not check out the recorded commit"})
}

// validateDeployInputs is kept for backward compatibility and delegates
// to the deploypath package (containment-aware).
func validateDeployInputs(deployPath, composeFile string) error {
	if _, err := deploypath.ValidateWrite(deployPath, nil); err != nil {
		return err
	}
	return deploypath.ValidateComposeFile(composeFile)
}

// warnComposeRefs surfaces compose-file references resolving outside the
// deployment roots into the deploy log + audit trail. See deploypath for
// why this warns instead of failing.
func warnComposeRefs(db *database.DB, buf *bytes.Buffer, actor string, deployID, projectID int64, composePath string, roots []string) {
	refs, err := deploypath.CheckComposeRefs(composePath, roots)
	if err != nil {
		fmt.Fprintf(buf, "compose scan: %v\n", err)
		return
	}
	if len(refs) == 0 {
		return
	}
	fmt.Fprintf(buf, "compose references outside deployment roots (review):\n")
	detail := make([]string, 0, len(refs))
	for _, r := range refs {
		fmt.Fprintf(buf, "  - %s: %s\n", r.Field, r.Value)
		detail = append(detail, r.Field+"="+r.Value)
	}
	audit.Write(db, actor, "deploy-refs", "deployment", strconv.FormatInt(deployID, 10), "warn",
		"project="+strconv.FormatInt(projectID, 10)+" "+strings.Join(detail, " "))
}

// executeDeploy runs the staged pipeline (pull → up → health check),
// updating the operation stages and broadcasting events throughout.
// deployPath is resolved (symlinks + root containment) before ANY command
// runs, so a path planted or swapped after project save cannot escape.
func executeDeploy(db *database.DB, broker *events.Broker, policy ExecPolicy, opID string, deployID, projectID int64, projectName, deployPath, composeFile, actor, commit, healthURL string) {
	start := time.Now()
	emit := func(t string, data interface{}) {
		if broker != nil {
			broker.Publish(t, data)
		}
	}
	base := map[string]interface{}{
		"deploymentId": deployID, "projectId": projectID,
		"project": projectName, "commit": commit, "operationId": opID,
	}
	fail := func(logs string) {
		dur := int64(time.Since(start).Seconds())
		_, _ = db.Exec(`UPDATE deployments SET status='FAILED', completed_at=CURRENT_TIMESTAMP, duration_sec=?, logs=? WHERE id=?`,
			dur, logs, deployID)
		_ = ops.Finish(db, opID, "FAILED", logs)
		audit.Write(db, actor, "deploy", "deployment", strconv.FormatInt(deployID, 10), "FAILED", commit)
		base["durationSec"] = dur
		emit("deployment.failed", base)
		notify.Send(db, notify.EventDeployFailed,
			fmt.Sprintf("Deploy failed: %s @ %s", projectName, commit), logs)
	}
	_ = ops.Start(db, opID)
	emit("deployment.started", base)

	resolvedPath, err := deploypath.ResolveExec(deployPath, policy.Roots)
	if err != nil {
		fail(err.Error())
		return
	}
	if err := deploypath.ValidateComposeFile(composeFile); err != nil {
		fail(err.Error())
		return
	}
	if healthURL != "" {
		if err := safehttp.ValidateURL(healthURL); err != nil {
			fail(fmt.Sprintf("health_url: %v", err))
			return
		}
	}
	deployPath = resolvedPath
	composePath := filepath.Join(deployPath, composeFile)
	if _, err := os.Stat(composePath); err != nil {
		fail(fmt.Sprintf("Compose file not found: %s", composePath))
		return
	}
	var buf bytes.Buffer
	// Indirect references (include/env_file/bind mounts outside the roots)
	// are legitimate in real stacks (e.g. docker.sock), so they warn
	// instead of failing — but they are always surfaced for review.
	warnComposeRefs(db, &buf, actor, deployID, projectID, composePath, policy.Roots)

	run := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		cmd.Dir = deployPath
		cmd.Stdout = &buf
		cmd.Stderr = &buf
		fmt.Fprintf(&buf, "$ %s %v\n", name, args)
		return cmd.Run()
	}

	_ = ops.SetStage(db, opID, 0, false, "")
	if err := run("docker", "compose", "pull"); err != nil {
		fmt.Fprintf(&buf, "pull warning: %v\n", err)
	}

	_ = ops.SetStage(db, opID, 1, false, "")
	if err := run("docker", "compose", "up", "-d", "--remove-orphans"); err != nil {
		if err2 := run("docker-compose", "pull"); err2 != nil {
			_ = ops.SetStage(db, opID, 1, true, "")
			fail(buf.String() + fmt.Sprintf("\ndeploy failed: %v", err))
			return
		}
		if err2 := run("docker-compose", "up", "-d", "--remove-orphans"); err2 != nil {
			_ = ops.SetStage(db, opID, 1, true, "")
			fail(buf.String() + fmt.Sprintf("\ndeploy failed: %v", err))
			return
		}
	}

	_ = ops.SetStage(db, opID, 2, false, "")
	if healthURL != "" {
		fmt.Fprintf(&buf, "health check: polling %s for 200 (30s budget)\n", healthURL)
		if ok, reason := safehttp.Get(healthURL, 5*time.Second, 30*time.Second, policy.StrictEgress); !ok {
			_ = ops.SetStage(db, opID, 2, true, "")
			if reason != "" {
				fmt.Fprintf(&buf, "health check: %s\n", reason)
			}
			fail(buf.String() + "\nhealth check failed: endpoint did not return 2xx in time")
			return
		}
		fmt.Fprintf(&buf, "health check: healthy\n")
	}

	dur := int64(time.Since(start).Seconds())
	_, _ = db.Exec(`UPDATE deployments SET status='SUCCESS', completed_at=CURRENT_TIMESTAMP, duration_sec=?, logs=? WHERE id=?`,
		dur, buf.String(), deployID)
	_ = ops.Finish(db, opID, "SUCCESS", "")
	audit.Write(db, actor, "deploy", "deployment", strconv.FormatInt(deployID, 10), "SUCCESS", commit)
	base["durationSec"] = dur
	emit("deployment.completed", base)
}


