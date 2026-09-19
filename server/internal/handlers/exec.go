package handlers

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"serverhub/internal/audit"
	"serverhub/internal/dockerx"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/ops"
)

// ExecHandler opens controlled, interactive shells inside containers:
// Docker exec → WebSocket → xterm. No host shell is ever exposed; every
// session is audit-logged and requires explicit confirmation to start.
type ExecHandler struct {
	DB     *sql.DB
	Docker *dockerx.Client
	Broker *events.Broker
}

type execGrant struct {
	container string
	shell     string
	cols      uint
	rows      uint
	expires   time.Time
	actor     string
}

var (
	grantsMu sync.Mutex
	grants   = map[string]execGrant{}
)

func mintGrant(container, shell string, cols, rows uint, actor string) string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	grantsMu.Lock()
	defer grantsMu.Unlock()
	now := time.Now()
	for t, g := range grants {
		if now.After(g.expires) {
			delete(grants, t)
		}
	}
	grants[token] = execGrant{
		container: container, shell: shell, cols: cols, rows: rows,
		expires: now.Add(60 * time.Second), actor: actor,
	}
	return token
}

func takeGrant(token string) (execGrant, bool) {
	grantsMu.Lock()
	defer grantsMu.Unlock()
	g, ok := grants[token]
	if !ok || time.Now().After(g.expires) {
		delete(grants, token)
		return execGrant{}, false
	}
	delete(grants, token) // single-use
	return g, true
}

// POST /server-hub/api/containers/:id/exec — HIGH RISK, requires confirm.
// Returns a single-use token for the WebSocket session.
func (h *ExecHandler) Create(c *gin.Context) {
	id := c.Param("id")
	u, _ := middleware.CurrentUser(c)
	if !confirmed(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "opening a shell is high-risk: retry with ?confirm=true or body {\"confirm\": true}"})
		return
	}
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable"})
		return
	}
	var body struct {
		Shell string `json:"shell"`
		Cols  uint   `json:"cols"`
		Rows  uint   `json:"rows"`
	}
	_ = c.ShouldBindJSON(&body)
	shell := body.Shell
	if shell != "/bin/bash" && shell != "/bin/sh" && shell != "sh" {
		shell = "/bin/sh"
	}
	cols, rows := body.Cols, body.Rows
	if cols == 0 || cols > 400 {
		cols = 120
	}
	if rows == 0 || rows > 200 {
		rows = 32
	}
	// Verify the container exists before minting.
	ctx, cancel := dockerCtx()
	defer cancel()
	if _, err := h.Docker.InspectContainer(ctx, id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "container not found"})
		return
	}
	op, err := ops.Create(h.DB, "container.exec", "container", id, u, []string{"shell"})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
		return
	}
	token := mintGrant(id, shell, cols, rows, u)
	audit.Write(h.DB, u, "exec-start", "container", id, "ok", "op="+op.ID+" shell="+shell)
	if h.Broker != nil {
		h.Broker.Publish("container.exec", map[string]interface{}{
			"id": id, "operationId": op.ID, "actor": u, "shell": shell,
		})
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "operationId": op.ID, "id": id})
}

var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  32 * 1024,
	WriteBufferSize: 32 * 1024,
	// Auth rides on the single-use token, not cookies, so any origin
	// presenting a valid token may upgrade.
	CheckOrigin: func(r *http.Request) bool { return true },
}

// GET /server-hub/api/exec/:token — WebSocket PTY bridge (no session auth;
// the single-use token IS the credential).
func (h *ExecHandler) Attach(c *gin.Context) {
	g, ok := takeGrant(c.Param("token"))
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
		return
	}
	raw, err := h.Docker.Raw()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	execID, err := func() (string, error) {
		resp, err := raw.ContainerExecCreate(ctx, g.container, container.ExecOptions{
			AttachStdin: true, AttachStdout: true, AttachStderr: true,
			Tty: true, Cmd: []string{g.shell},
			WorkingDir: "/",
			ConsoleSize: &[2]uint{g.rows, g.cols},
		})
		if err != nil {
			return "", err
		}
		return resp.ID, nil
	}()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	hijack, err := raw.ContainerExecAttach(ctx, execID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	defer hijack.Close()

	ws, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer ws.Close()
	_ = ws.SetReadDeadline(time.Now().Add(10 * time.Minute))

	// Resize + input from the browser.
	go func() {
		defer hijack.Close()
		for {
			mt, msg, err := ws.ReadMessage()
			if err != nil {
				return
			}
			if mt == websocket.TextMessage {
				var resize struct {
					Cols *uint `json:"cols"`
					Rows *uint `json:"rows"`
				}
				if err := json.Unmarshal(msg, &resize); err == nil && resize.Cols != nil {
					rctx, rcancel := dockerCtx()
					_ = raw.ContainerExecResize(rctx, execID, container.ResizeOptions{
						Height: *resize.Rows, Width: *resize.Cols,
					})
					rcancel()
					continue
				}
				_, _ = hijack.Conn.Write(msg)
				continue
			}
			_, _ = hijack.Conn.Write(msg)
		}
	}()

	// Container output to the browser.
	buf := make([]byte, 32*1024)
	for {
		n, err := hijack.Reader.Read(buf)
		if n > 0 {
			_ = ws.SetWriteDeadline(time.Now().Add(30 * time.Second))
			if werr := ws.WriteMessage(websocket.BinaryMessage, buf[:n]); werr != nil {
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				_ = ws.WriteMessage(websocket.TextMessage, []byte("\r\n[connection closed]\r\n"))
			}
			return
		}
	}
}
