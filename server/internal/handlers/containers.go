package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/docker/docker/api/types/container"
	"github.com/gin-gonic/gin"

	"serverhub/internal/applog"
	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/ops"
)

type ContainerHandler struct {
	DB     *database.DB
	Docker *dockerx.Client
	Broker *events.Broker
}

func dockerCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 10*time.Second)
}

func (h *ContainerHandler) List(c *gin.Context) {
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable", "containers": []interface{}{}})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	list, err := h.Docker.ListContainers(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	// Enrich with project mapping via compose project label.
	type out struct {
		ID      string   `json:"id"`
		Names   []string `json:"names"`
		Image   string   `json:"image"`
		State   string   `json:"state"`
		Status  string   `json:"status"`
		Project string   `json:"composeProject,omitempty"`
		Service string   `json:"composeService,omitempty"`
	}
	res := make([]out, 0, len(list))
	for _, ctr := range list {
		o := out{ID: ctr.ID, Names: ctr.Names, Image: ctr.Image, State: ctr.State, Status: ctr.Status}
		if ctr.Labels != nil {
			o.Project = ctr.Labels["com.docker.compose.project"]
			o.Service = ctr.Labels["com.docker.compose.service"]
		}
		res = append(res, o)
	}
	c.JSON(http.StatusOK, res)
}

func (h *ContainerHandler) Inspect(c *gin.Context) {
	id := c.Param("id")
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable"})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	info, err := h.Docker.InspectContainer(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, info)
}

func (h *ContainerHandler) Logs(c *gin.Context) {
	id := c.Param("id")
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable"})
		return
	}
	tail := c.DefaultQuery("tail", "200")
	cli, err := h.rawClient()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	opts := container.LogsOptions{ShowStdout: true, ShowStderr: true, Tail: tail, Timestamps: false}
	if c.Query("since") != "" {
		opts.Since = c.Query("since")
	}
	if c.Query("follow") == "1" {
		// SSE live tail. The subscription itself is recorded in the
		// central DB log store; the streamed bytes remain ephemeral.
		if h.DB != nil && h.DB.GDB != nil {
			if u, _ := middleware.CurrentUser(c); true {
				applog.Write(h.DB.GDB, applog.Entry{
					Level: "INFO", Source: "container", Actor: u,
					Action: "tail-logs", Resource: "container", ResourceID: id,
					Message:  fmt.Sprintf("container %s log tail started (tail=%s)", id, tail),
					Method:   "GET",
					Path:     c.Request.URL.Path,
					Metadata: applog.Truncate("since="+c.Query("since"), 500),
				})
			}
		}
		// SSE live tail
		opts.Follow = true
		reader, err := cli.ContainerLogs(ctx, id, opts)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		defer reader.Close()
		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		buf := make([]byte, 4096)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				// Docker multiplexes logs with 8-byte headers; strip them best-effort.
				payload := stripDockerHeader(buf[:n])
				_, _ = c.Writer.Write([]byte("data: " + string(payload) + "\n\n"))
				c.Writer.Flush()
			}
			if err != nil {
				break
			}
		}
		return
	}
	reader, err := cli.ContainerLogs(ctx, id, opts)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	defer reader.Close()
	b, _ := io.ReadAll(reader)
	text := string(stripDockerHeader(b))
	// Persist the fetched snapshot to the central DB log store so container
	// output is not only ephemeral Docker-daemon state. The tail sample is
	// truncated to bound row size; the fetch itself is always recorded.
	if h.DB != nil && h.DB.GDB != nil {
		u, _ := middleware.CurrentUser(c)
		applog.Write(h.DB.GDB, applog.Entry{
			Level: "INFO", Source: "container", Actor: u,
			Action: "fetch-logs", Resource: "container", ResourceID: id,
			Message:  fmt.Sprintf("container %s logs fetched (%d bytes, tail=%s)", id, len(text), tail),
			Method:   "GET",
			Path:     c.Request.URL.Path,
			Metadata: applog.Truncate(text, 16000),
		})
	}
	c.JSON(http.StatusOK, gin.H{"logs": text})
}

func stripDockerHeader(b []byte) []byte {
	// Docker log frames: 8-byte header {type,0,0,0,size...}. Remove headers heuristically.
	out := make([]byte, 0, len(b))
	i := 0
	for i < len(b) {
		if len(b)-i >= 8 && (b[i] == 1 || b[i] == 2) && b[i+1] == 0 && b[i+2] == 0 && b[i+3] == 0 {
			size := int(b[i+4])<<24 | int(b[i+5])<<16 | int(b[i+6])<<8 | int(b[i+7])
			i += 8
			if size <= 0 || i+size > len(b) {
				out = append(out, b[i:]...)
				break
			}
			out = append(out, b[i:i+size]...)
			i += size
		} else {
			out = append(out, b[i:]...)
			break
		}
	}
	return out
}

func (h *ContainerHandler) Stats(c *gin.Context) {
	id := c.Param("id")
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable"})
		return
	}
	cli, err := h.rawClient()
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	resp, err := cli.ContainerStats(ctx, id, false)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	c.Data(http.StatusOK, "application/json", b)
}

func (h *ContainerHandler) Images(c *gin.Context) {
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable", "images": []interface{}{}})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	imgs, err := h.Docker.ListImages(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, imgs)
}

func (h *ContainerHandler) Volumes(c *gin.Context) {
	if !h.Docker.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable", "volumes": []interface{}{}})
		return
	}
	ctx, cancel := dockerCtx()
	defer cancel()
	vols, err := h.Docker.ListVolumes(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, vols)
}

// confirmed checks the high-risk confirmation gate: either
// ?confirm=true or a JSON body {"confirm": true}.
func confirmed(c *gin.Context) bool {
	if c.Query("confirm") == "true" {
		return true
	}
	var body struct {
		Confirm *bool `json:"confirm"`
	}
	if err := c.ShouldBindJSON(&body); err == nil && body.Confirm != nil {
		return *body.Confirm
	}
	return false
}

// lifecycle runs a container action as a tracked operation and broadcasts
// a container.<action> event on success.
func (h *ContainerHandler) lifecycle(c *gin.Context, action string, needConfirm bool,
	run func(ctx context.Context, id string) error) {
	id := c.Param("id")
	u, _ := middleware.CurrentUser(c)
	if needConfirm && !confirmed(c) {
		c.JSON(http.StatusBadRequest, gin.H{"error": action + "ing a container is high-risk: retry with ?confirm=true or body {\"confirm\": true}"})
		return
	}
	op, err := ops.Create(h.DB, "container."+action, "container", id, u, []string{action})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not track operation"})
		return
	}
	emit := func(t string, ok bool, detail string) {
		if h.Broker != nil {
			h.Broker.Publish(t, map[string]interface{}{
				"id": id, "action": action, "operationId": op.ID,
				"ok": ok, "detail": detail,
			})
		}
	}
	if !h.Docker.Available() {
		_ = ops.Finish(h.DB, op.ID, "FAILED", "docker unavailable")
		audit.Write(h.DB, u, action, "container", id, "failed", "docker unavailable")
		emit("container."+action, false, "docker unavailable")
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "docker unavailable", "operationId": op.ID})
		return
	}
	_ = ops.Start(h.DB, op.ID)
	_ = ops.SetStage(h.DB, op.ID, 0, false, "")
	ctx, cancel := dockerCtx()
	defer cancel()
	if err := run(ctx, id); err != nil {
		_ = ops.SetStage(h.DB, op.ID, 0, true, "")
		_ = ops.Finish(h.DB, op.ID, "FAILED", err.Error())
		audit.Write(h.DB, u, action, "container", id, "failed", err.Error())
		emit("container."+action, false, err.Error())
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error(), "operationId": op.ID})
		return
	}
	_ = ops.Finish(h.DB, op.ID, "SUCCESS", "")
	audit.Write(h.DB, u, action, "container", id, "ok", "op="+op.ID)
	emit("container."+action, true, "op="+op.ID)
	c.JSON(http.StatusOK, gin.H{"ok": true, "id": id, "action": action, "operationId": op.ID})
}

// POST /api/containers/:id/start — medium risk, no confirmation needed.
func (h *ContainerHandler) Start(c *gin.Context) {
	h.lifecycle(c, "start", false, func(ctx context.Context, id string) error {
		return h.Docker.StartContainer(ctx, id)
	})
}

// POST /api/containers/:id/stop — HIGH RISK, requires confirm=true.
func (h *ContainerHandler) Stop(c *gin.Context) {
	h.lifecycle(c, "stop", true, func(ctx context.Context, id string) error {
		return h.Docker.StopContainer(ctx, id, 10)
	})
}

// POST /api/containers/:id/restart — HIGH RISK, requires confirm=true.
func (h *ContainerHandler) Restart(c *gin.Context) {
	h.lifecycle(c, "restart", true, func(ctx context.Context, id string) error {
		return h.Docker.RestartContainer(ctx, id, 10)
	})
}
