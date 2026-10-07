package handlers

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/applog"
	"serverhub/internal/audit"
	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/monitoring"
)

// MonitoringHandler handles all Prometheus + Alertmanager proxy endpoints.
type MonitoringHandler struct {
	DB         *database.DB
	Cfg        *config.Config
	Broker     *events.Broker
	Prometheus *monitoring.PrometheusClient
	Alertmgr   *monitoring.AlertmanagerClient
}

func promCtx(cfg *config.Config) (context.Context, context.CancelFunc) {
	t := cfg.PrometheusTimeoutSec
	if t <= 0 {
		t = 10
	}
	return context.WithTimeout(context.Background(), time.Duration(t)*time.Second)
}

func amCtx(cfg *config.Config) (context.Context, context.CancelFunc) {
	t := cfg.AlertmanagerTimeoutSec
	if t <= 0 {
		t = 10
	}
	return context.WithTimeout(context.Background(), time.Duration(t)*time.Second)
}

// ── Prometheus endpoints ──────────────────────────────────────────────────────

// GET /server-hub/api/monitoring/prometheus/status
func (h *MonitoringHandler) PrometheusStatus(c *gin.Context) {
	if !h.Prometheus.Available() {
		c.JSON(http.StatusOK, gin.H{"available": false, "status": "not configured"})
		return
	}
	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	ok, msg := h.Prometheus.Status(ctx)
	c.JSON(http.StatusOK, gin.H{"available": true, "healthy": ok, "status": msg})
}

// GET /server-hub/api/monitoring/prometheus/query?query=<promql>
func (h *MonitoringHandler) PrometheusQuery(c *gin.Context) {
	if !h.Prometheus.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "prometheus not configured"})
		return
	}
	query := strings.TrimSpace(c.Query("query"))
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter required"})
		return
	}
	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	data, err := h.Prometheus.Query(ctx, query)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GET /server-hub/api/monitoring/prometheus/query-range
func (h *MonitoringHandler) PrometheusQueryRange(c *gin.Context) {
	if !h.Prometheus.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "prometheus not configured"})
		return
	}
	query := strings.TrimSpace(c.Query("query"))
	start := c.Query("start")
	end := c.Query("end")
	step := c.DefaultQuery("step", "60")
	if query == "" || start == "" || end == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query, start, end required"})
		return
	}
	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	data, err := h.Prometheus.QueryRange(ctx, query, start, end, step)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GET /server-hub/api/monitoring/prometheus/targets
func (h *MonitoringHandler) PrometheusTargets(c *gin.Context) {
	if !h.Prometheus.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "prometheus not configured"})
		return
	}
	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	data, err := h.Prometheus.Targets(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// GET /server-hub/api/monitoring/prometheus/rules
func (h *MonitoringHandler) PrometheusRules(c *gin.Context) {
	if !h.Prometheus.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "prometheus not configured"})
		return
	}
	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	data, err := h.Prometheus.Rules(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

// ── High-level overview ───────────────────────────────────────────────────────

// GET /server-hub/api/monitoring/overview
// Executes predefined PromQL queries and returns a frontend-friendly summary.
func (h *MonitoringHandler) Overview(c *gin.Context) {
	if !h.Prometheus.Available() {
		c.JSON(http.StatusOK, gin.H{
			"available": false,
			"cpu": nil, "memory": nil, "disk": nil,
			"networkRx": nil, "networkTx": nil,
		})
		return
	}
	ctx, cancel := promCtx(h.Cfg)
	defer cancel()

	fetch := func(q string) float64 {
		data, err := h.Prometheus.Query(ctx, q)
		if err != nil {
			return -1
		}
		v, _ := monitoring.ScalarFloat(data)
		return v
	}

	cpu := fetch(`100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`)
	mem := fetch(`(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100`)
	disk := fetch(`(1 - (node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})) * 100`)
	rxRate := fetch(`sum(rate(node_network_receive_bytes_total[5m]))`)
	txRate := fetch(`sum(rate(node_network_transmit_bytes_total[5m]))`)

	c.JSON(http.StatusOK, gin.H{
		"available": true,
		"cpu":       roundF(cpu, 2),
		"memory":    roundF(mem, 2),
		"disk":      roundF(disk, 2),
		"networkRx": roundF(rxRate, 0),
		"networkTx": roundF(txRate, 0),
	})
}

// GET /server-hub/api/monitoring/metrics?metric=cpu|memory|disk|network&range=1h|6h|24h|7d&step=60
func (h *MonitoringHandler) Metrics(c *gin.Context) {
	if !h.Prometheus.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "prometheus not configured"})
		return
	}

	metric := c.DefaultQuery("metric", "cpu")
	rangeStr := c.DefaultQuery("range", "1h")
	step := c.DefaultQuery("step", "60")

	rangeSeconds := map[string]int64{
		"1h": 3600, "6h": 21600, "24h": 86400, "7d": 604800,
	}
	dur, ok := rangeSeconds[rangeStr]
	if !ok {
		dur = 3600
		rangeStr = "1h"
	}

	queries := map[string]string{
		"cpu":       `100 - (avg(rate(node_cpu_seconds_total{mode="idle"}[5m])) * 100)`,
		"memory":    `(1 - (node_memory_MemAvailable_bytes / node_memory_MemTotal_bytes)) * 100`,
		"disk":      `(1 - (node_filesystem_avail_bytes{mountpoint="/"} / node_filesystem_size_bytes{mountpoint="/"})) * 100`,
		"networkRx": `sum(rate(node_network_receive_bytes_total[5m]))`,
		"networkTx": `sum(rate(node_network_transmit_bytes_total[5m]))`,
		"uptime":    `node_time_seconds - node_boot_time_seconds`,
	}

	query, ok := queries[metric]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown metric; use cpu|memory|disk|networkRx|networkTx|uptime"})
		return
	}

	now := time.Now().Unix()
	start := fmt.Sprintf("%d", now-dur)
	end := fmt.Sprintf("%d", now)

	ctx, cancel := promCtx(h.Cfg)
	defer cancel()
	data, err := h.Prometheus.QueryRange(ctx, query, start, end, step)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"metric": metric, "range": rangeStr, "data": data})
}

// ── Alertmanager endpoints ────────────────────────────────────────────────────

// GET /server-hub/api/monitoring/alertmanager/status
func (h *MonitoringHandler) AlertmanagerStatus(c *gin.Context) {
	if !h.Alertmgr.Available() {
		c.JSON(http.StatusOK, gin.H{"available": false, "status": "not configured"})
		return
	}
	ctx, cancel := amCtx(h.Cfg)
	defer cancel()
	ok, msg := h.Alertmgr.Status(ctx)
	c.JSON(http.StatusOK, gin.H{"available": true, "healthy": ok, "status": msg})
}

// GET /server-hub/api/monitoring/alerts
func (h *MonitoringHandler) AlertmanagerAlerts(c *gin.Context) {
	if !h.Alertmgr.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "alertmanager not configured"})
		return
	}
	ctx, cancel := amCtx(h.Cfg)
	defer cancel()
	data, err := h.Alertmgr.Alerts(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

// GET /server-hub/api/monitoring/silences
func (h *MonitoringHandler) ListSilences(c *gin.Context) {
	if !h.Alertmgr.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "alertmanager not configured"})
		return
	}
	ctx, cancel := amCtx(h.Cfg)
	defer cancel()
	data, err := h.Alertmgr.Silences(ctx)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.Data(http.StatusOK, "application/json", data)
}

// POST /server-hub/api/monitoring/silences
func (h *MonitoringHandler) CreateSilence(c *gin.Context) {
	if !h.Alertmgr.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "alertmanager not configured"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 64*1024))
	if err != nil || len(body) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "request body required"})
		return
	}
	// Validate it's JSON before forwarding.
	if !json.Valid(body) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}
	ctx, cancel := amCtx(h.Cfg)
	defer cancel()
	data, err := h.Alertmgr.CreateSilence(ctx, body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	if h.DB != nil {
		audit.Write(h.DB, u, "create-silence", "alertmanager", "", "ok", "")
	}
	c.Data(http.StatusOK, "application/json", data)
}

// DELETE /server-hub/api/monitoring/silences/:id
func (h *MonitoringHandler) DeleteSilence(c *gin.Context) {
	if !h.Alertmgr.Available() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "alertmanager not configured"})
		return
	}
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "silence id required"})
		return
	}
	ctx, cancel := amCtx(h.Cfg)
	defer cancel()
	if err := h.Alertmgr.DeleteSilence(ctx, id); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	if h.DB != nil {
		audit.Write(h.DB, u, "delete-silence", "alertmanager", id, "ok", "")
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ── Alertmanager webhook ──────────────────────────────────────────────────────

// WebhookAlert is a single Alertmanager alert delivery.
type WebhookAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    string            `json:"startsAt"`
	EndsAt      string            `json:"endsAt"`
	Fingerprint string            `json:"fingerprint"`
}

// AlertmanagerWebhookPayload is the Alertmanager webhook body.
type AlertmanagerWebhookPayload struct {
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Status            string            `json:"status"` // "firing" | "resolved"
	Receiver          string            `json:"receiver"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Alerts            []WebhookAlert    `json:"alerts"`
}

// POST /server-hub/api/webhooks/alertmanager
// Authenticated via ALERTMANAGER_WEBHOOK_SECRET (HMAC-SHA256 or shared secret header).
func (h *MonitoringHandler) AlertmanagerWebhook(c *gin.Context) {
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, 1<<20))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot read body"})
		return
	}

	// Validate shared secret if configured.
	if h.Cfg.AlertmanagerWebhookSecret != "" {
		if !checkAlertmanagerAuth(h.Cfg.AlertmanagerWebhookSecret, c, body) {
			if h.DB != nil {
				applog.Warn(h.DB.GDB, "webhook", "alertmanager webhook: auth failed")
			}
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid secret or signature"})
			return
		}
	}

	var payload AlertmanagerWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		if h.DB != nil {
			applog.Warn(h.DB.GDB, "webhook", "alertmanager webhook: malformed payload: "+err.Error())
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	if h.DB != nil {
		applog.Info(h.DB.GDB, "webhook",
			fmt.Sprintf("alertmanager webhook: status=%s alerts=%d receiver=%s",
				payload.Status, len(payload.Alerts), payload.Receiver))
	}

	// Publish SSE events for each alert and persist firing/resolved state.
	// Fingerprint dedupes repeated Alertmanager deliveries into one alert row.
	for _, a := range payload.Alerts {
		evType := "monitoring.alert.firing"
		if a.Status == "resolved" {
			evType = "monitoring.alert.resolved"
		}
		serverID := h.resolveAlertServer(a.Labels)
		h.persistWebhookAlert(a, serverID)
		if h.Broker != nil {
			h.Broker.Publish(evType, gin.H{
				"status":      a.Status,
				"labels":      a.Labels,
				"annotations": a.Annotations,
				"startsAt":    a.StartsAt,
				"endsAt":      a.EndsAt,
				"fingerprint": a.Fingerprint,
				"alertname":   a.Labels["alertname"],
				"severity":    a.Labels["severity"],
				"instance":    a.Labels["instance"],
				"serverId":    serverID,
			})
		}
	}

	// Also publish a group-level event.
	groupEvType := "monitoring.alert.firing"
	if payload.Status == "resolved" {
		groupEvType = "monitoring.alert.resolved"
	}
	if h.Broker != nil {
		h.Broker.Publish(groupEvType, gin.H{
			"groupKey":    payload.GroupKey,
			"status":      payload.Status,
			"receiver":    payload.Receiver,
			"alertCount":  len(payload.Alerts),
			"groupLabels": payload.GroupLabels,
		})
	}

	c.JSON(http.StatusOK, gin.H{"ok": true, "received": len(payload.Alerts)})
}

// checkAlertmanagerAuth validates the webhook caller when a shared secret is
// configured. Accepts the plaintext X-Alertmanager-Secret header (compared
// in constant time) or an HMAC-SHA256 body signature.
func checkAlertmanagerAuth(secret string, c *gin.Context, body []byte) bool {
	if provided := c.GetHeader("X-Alertmanager-Secret"); provided != "" {
		a := []byte(provided)
		b := []byte(secret)
		return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
	}
	if sig := c.GetHeader("X-Hub-Signature-256"); sig != "" {
		return verifySignature(secret, body, sig)
	}
	return false
}

// resolveAlertServer maps Alertmanager labels to a managed server id.
// Supports an explicit server_id label plus hostname/instance matching, so
// Prometheus scrape labels stay the extensible multi-server contract:
// add `labels: {server_id: "<id>"}` (or a resolvable hostname) to any job.
func (h *MonitoringHandler) resolveAlertServer(labels map[string]string) *uint {
	if h.DB == nil {
		return nil
	}
	if raw := strings.TrimSpace(labels["server_id"]); raw != "" {
		if id, err := strconv.ParseUint(raw, 10, 64); err == nil {
			var count int
			_ = h.DB.QueryRow(`SELECT COUNT(*) FROM managed_servers WHERE id=$1`, id).Scan(&count)
			if count > 0 {
				u := uint(id)
				return &u
			}
		}
	}
	candidates := []string{
		strings.TrimSpace(labels["hostname"]),
		hostPart(labels["instance"]),
	}
	for _, cand := range candidates {
		if cand == "" {
			continue
		}
		var id uint
		err := h.DB.QueryRow(`
			SELECT id FROM managed_servers
			WHERE hostname=$1 OR ip_address=$1 OR name=$1 LIMIT 1`, cand).Scan(&id)
		if err == nil {
			return &id
		}
	}
	return nil
}

// hostPart strips an optional :port suffix from an instance label.
func hostPart(instance string) string {
	instance = strings.TrimSpace(instance)
	if i := strings.LastIndex(instance, ":"); i > 0 {
		return instance[:i]
	}
	return instance
}

// persistWebhookAlert upserts the alerts table from one Alertmanager alert.
// Firing alerts insert once per fingerprint; resolved alerts close the open
// row. New firing alerts fan out to in-app notifications.
func (h *MonitoringHandler) persistWebhookAlert(a WebhookAlert, serverID *uint) {
	if h.DB == nil {
		return
	}
	fp := a.Fingerprint
	if fp == "" {
		return
	}
	severity := strings.ToUpper(strings.TrimSpace(a.Labels["severity"]))
	switch severity {
	case "INFO", "WARNING", "CRITICAL":
	default:
		severity = "WARNING"
	}
	condition := strings.TrimSpace(a.Labels["alertname"])
	if condition == "" {
		condition = "alertmanager"
	}

	if a.Status == "resolved" {
		res, err := h.DB.Exec(`
			UPDATE alerts SET status='RESOLVED', resolved_at=NOW()
			WHERE fingerprint=$1 AND status='TRIGGERED'`, fp)
		if err != nil {
			return
		}
		if n, _ := res.RowsAffected(); n > 0 {
			applog.Info(h.DB.GDB, "webhook",
				fmt.Sprintf("alertmanager resolved %s (%s)", condition, fp))
		}
		return
	}

	var count int
	_ = h.DB.QueryRow(`
		SELECT COUNT(*) FROM alerts WHERE fingerprint=$1 AND status='TRIGGERED'`, fp).Scan(&count)
	if count > 0 {
		return // duplicate delivery of an already-firing alert
	}
	message := strings.TrimSpace(a.Annotations["summary"])
	if message == "" {
		message = strings.TrimSpace(a.Annotations["description"])
	}
	if message == "" {
		message = condition + " firing on " + hostPart(a.Labels["instance"])
	}
	var sid any
	if serverID != nil {
		sid = *serverID
	}
	alertID, err := h.DB.InsertID(`
		INSERT INTO alerts (server_id,condition,threshold,severity,status,message,triggered_at,fingerprint,source)
		VALUES ($1,$2,0,$3,'TRIGGERED',$4,NOW(),$5,'alertmanager')`,
		sid, condition, severity, message, fp)
	if err != nil {
		return
	}
	applog.Info(h.DB.GDB, "webhook",
		fmt.Sprintf("alertmanager firing %s (%s)", condition, fp))
	notifyWebhookUsers(h.DB, serverID, severity+": "+message, message, "alert", alertID)
	if h.Broker != nil {
		h.Broker.Publish("alert.triggered", map[string]interface{}{
			"serverId": serverID, "condition": condition,
			"severity": severity, "message": message, "fingerprint": fp,
		})
	}
}

// notifyWebhookUsers fans a webhook alert out to every user's notification
// center, linked to the alert row when one was created.
func notifyWebhookUsers(db *database.DB, serverID *uint, title, body, category string, alertID int64) {
	rows, err := db.Query(`SELECT username FROM users`)
	if err != nil {
		return
	}
	defer rows.Close()
	var sid any
	if serverID != nil {
		sid = *serverID
	}
	for rows.Next() {
		var u string
		if err := rows.Scan(&u); err != nil {
			continue
		}
		_, _ = db.Exec(`
			INSERT INTO in_app_notifications (username,title,body,category,read,server_id,alert_id,created_at)
			VALUES ($1,$2,$3,$4,false,$5,$6,NOW())`,
			u, title, body, category, sid, alertID)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func roundF(v float64, decimals int) float64 {
	if v < 0 {
		return v // preserve sentinel -1 (unavailable)
	}
	factor := 1.0
	for i := 0; i < decimals; i++ {
		factor *= 10
	}
	return float64(int(v*factor+0.5)) / factor
}

// allowedMetrics is the set of metric names accepted by the admin PromQL endpoint.
// Restricts arbitrary PromQL to admin role only (enforced in route registration).
var _ = strings.ToLower // keep import
