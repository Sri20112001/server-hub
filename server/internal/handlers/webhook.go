package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/ops"
)

// WebhookHandler receives GitHub push webhooks (spec §37 Phase 9).
// Public endpoint — authenticated via HMAC-SHA256 signature, NOT session.
// Requires GITHUB_WEBHOOK_SECRET to be configured, otherwise 503.
type WebhookHandler struct {
	DB     *database.DB
	Cfg    *config.Config
	Broker *events.Broker
}

func verifySignature(secret string, body []byte, header string) bool {
	const prefix = "sha256="
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	sig, err := hex.DecodeString(strings.TrimPrefix(header, prefix))
	if err != nil {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hmac.Equal(mac.Sum(nil), sig)
}

// SignBody is exported for tests.
func SignBody(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

type pushPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
}

func (h *WebhookHandler) GitHub(c *gin.Context) {
	if h.Cfg.WebhookSecret == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "webhooks not configured (GITHUB_WEBHOOK_SECRET unset)"})
		return
	}
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read body"})
		return
	}
	if !verifySignature(h.Cfg.WebhookSecret, body, c.GetHeader("X-Hub-Signature-256")) {
		audit.Write(h.DB, "webhook", "github-push", "webhook", "", "failed", "bad signature")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid signature"})
		return
	}
	event := c.GetHeader("X-GitHub-Event")
	if event == "ping" {
		c.JSON(http.StatusOK, gin.H{"ok": true, "msg": "pong"})
		return
	}
	if event != "push" {
		c.JSON(http.StatusOK, gin.H{"ok": true, "ignored": event})
		return
	}
	var p pushPayload
	if err := json.Unmarshal(body, &p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	branch := strings.TrimPrefix(p.Ref, "refs/heads/")
	if branch == "" || p.After == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing ref/after"})
		return
	}
	// Match projects by repository (full_name substring or exact html_url)
	// and branch.
	rows, err := h.DB.Query(`SELECT id, name, repository, branch, deployment_path, compose_file, health_url, auto_deploy FROM projects`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	type match struct {
		id          int64
		name        string
		deployPath  string
		composeFile string
		healthURL   string
		autoDeploy  bool
	}
	var matched []match
	for rows.Next() {
		var m match
		var repo, trackedBranch string
		var auto int
		if err := rows.Scan(&m.id, &m.name, &repo, &trackedBranch, &m.deployPath, &m.composeFile, &m.healthURL, &auto); err != nil {
			continue
		}
		if trackedBranch != branch {
			continue
		}
		repoMatch := repo != "" && (strings.Contains(repo, p.Repository.FullName) ||
			strings.TrimSuffix(repo, ".git") == strings.TrimSuffix(p.Repository.HTMLURL, ".git") ||
			strings.TrimSuffix(repo, ".git") == strings.TrimSuffix("https://github.com/"+p.Repository.FullName, ".git"))
		if !repoMatch {
			continue
		}
		m.autoDeploy = auto == 1
		matched = append(matched, m)
	}
	results := []gin.H{}
	for _, m := range matched {
		if m.autoDeploy {
			deployID, _ := h.DB.InsertID(`INSERT INTO deployments (project_id,commit_sha,branch,trigger,status,started_at) VALUES (?,?,?,?, 'RUNNING', CURRENT_TIMESTAMP)`,
				m.id, p.After, branch, "github-webhook")
			op, _ := ops.Create(h.DB, "deploy", "project", strconv.FormatInt(m.id, 10), "webhook", deployStages)
			opID := ""
			if op != nil {
				opID = op.ID
			}
			go executeDeploy(h.DB, h.Broker, opID, deployID, m.id, m.name, m.deployPath, m.composeFile, "webhook", p.After, m.healthURL)
			audit.Write(h.DB, "webhook", "deploy", "project", strconv.FormatInt(m.id, 10), "started", p.After)
			results = append(results, gin.H{"project": m.name, "deployed": true, "commit": p.After, "operationId": opID})
		} else {
			// Record for visibility without running anything.
			_, _ = h.DB.Exec(`INSERT INTO deployments (project_id,commit_sha,branch,trigger,status,started_at,completed_at,logs) VALUES (?,?,?,?, 'PENDING', CURRENT_TIMESTAMP, NULL, 'Auto-deploy disabled; manual deploy required.')`,
				m.id, p.After, branch, "github-webhook")
			audit.Write(h.DB, "webhook", "record-deployment", "project", strconv.FormatInt(m.id, 10), "ok", p.After)
			results = append(results, gin.H{"project": m.name, "deployed": false, "reason": "autoDeploy disabled", "commit": p.After})
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true, "matched": len(matched), "results": results})
}
