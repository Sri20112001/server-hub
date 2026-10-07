package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
)

type ServersHandler struct {
	DB     *database.DB
	Broker *events.Broker
}

// ─── Server CRUD ─────────────────────────────────────────────────────────────

func (h *ServersHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`
		SELECT id,name,hostname,ip_address,os,os_version,arch,cpu_info,cpu_cores,
		       ram_total,disk_total,status,agent_status,last_heartbeat,group_id,created_at,updated_at
		FROM managed_servers ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		m := scanServer(rows)
		if m != nil {
			out = append(out, m)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *ServersHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rows, err := h.DB.Query(`
		SELECT id,name,hostname,ip_address,os,os_version,arch,cpu_info,cpu_cores,
		       ram_total,disk_total,status,agent_status,last_heartbeat,group_id,created_at,updated_at
		FROM managed_servers WHERE id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	if rows.Next() {
		m := scanServer(rows)
		c.JSON(http.StatusOK, m)
		return
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
}

func (h *ServersHandler) Create(c *gin.Context) {
	var body struct {
		Name      string `json:"name"`
		Hostname  string `json:"hostname"`
		IPAddress string `json:"ipAddress"`
		GroupID   *uint  `json:"groupId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	id, err := h.DB.InsertID(`
		INSERT INTO managed_servers (name,hostname,ip_address,status,agent_status,group_id,created_at,updated_at)
		VALUES ($1,$2,$3,'UNKNOWN','UNKNOWN',$4,NOW(),NOW())`,
		body.Name, body.Hostname, body.IPAddress, body.GroupID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create", "managed_server", strconv.FormatInt(id, 10), "ok", body.Name)
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": body.Name})
}

func (h *ServersHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Name      *string `json:"name"`
		Hostname  *string `json:"hostname"`
		IPAddress *string `json:"ipAddress"`
		GroupID   *uint   `json:"groupId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, err = h.DB.Exec(`
		UPDATE managed_servers SET
		  name=COALESCE($1,name),
		  hostname=COALESCE($2,hostname),
		  ip_address=COALESCE($3,ip_address),
		  group_id=COALESCE($4,group_id),
		  updated_at=NOW()
		WHERE id=$5`,
		body.Name, body.Hostname, body.IPAddress, body.GroupID, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "update", "managed_server", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *ServersHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	_, _ = h.DB.Exec(`DELETE FROM agent_tokens WHERE server_id=$1`, id)
	_, _ = h.DB.Exec(`DELETE FROM managed_servers WHERE id=$1`, id)
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "managed_server", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── Agent Tokens ─────────────────────────────────────────────────────────────

func (h *ServersHandler) CreateToken(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Label string `json:"label"`
	}
	_ = c.ShouldBindJSON(&body)

	raw, hash, err := generateToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not generate token"})
		return
	}
	tokenID, err := h.DB.InsertID(`
		INSERT INTO agent_tokens (server_id,token_hash,label,revoked,created_at)
		VALUES ($1,$2,$3,false,NOW())`,
		id, hash, body.Label)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create-agent-token", "managed_server", strconv.FormatInt(id, 10), "ok", body.Label)
	// Return the raw token ONCE — never stored in plaintext.
	c.JSON(http.StatusCreated, gin.H{"id": tokenID, "token": raw, "label": body.Label})
}

func (h *ServersHandler) ListTokens(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rows, err := h.DB.Query(`
		SELECT id,server_id,label,revoked,created_at,last_used_at
		FROM agent_tokens WHERE server_id=$1 ORDER BY id DESC`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var tid, sid uint
		var label string
		var revoked bool
		var createdAt time.Time
		var lastUsed *time.Time
		if err := rows.Scan(&tid, &sid, &label, &revoked, &createdAt, &lastUsed); err == nil {
			out = append(out, gin.H{
				"id": tid, "serverId": sid, "label": label,
				"revoked": revoked, "createdAt": createdAt, "lastUsedAt": lastUsed,
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *ServersHandler) RevokeToken(c *gin.Context) {
	tid, err := strconv.ParseInt(c.Param("tokenId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid token id"})
		return
	}
	_, err = h.DB.Exec(`UPDATE agent_tokens SET revoked=true WHERE id=$1`, tid)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "revoke-agent-token", "agent_token", strconv.FormatInt(tid, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── Agent Endpoints (authenticated by AgentAuth middleware) ──────────────────

// POST /server-hub/api/agent/heartbeat
func (h *ServersHandler) Heartbeat(c *gin.Context) {
	serverID, ok := middleware.AgentServerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	now := time.Now().UTC()
	_, err := h.DB.Exec(`
		UPDATE managed_servers
		SET last_heartbeat=$1, agent_status='CONNECTED', status='ONLINE', updated_at=$1
		WHERE id=$2`, now, serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if h.Broker != nil {
		h.Broker.Publish("server.heartbeat", gin.H{"serverId": serverID, "ts": now})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// POST /server-hub/api/agent/metrics
func (h *ServersHandler) IngestMetrics(c *gin.Context) {
	serverID, ok := middleware.AgentServerID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return
	}
	var body struct {
		CPUUsage     float64 `json:"cpuUsage"`
		MemoryUsage  float64 `json:"memoryUsage"`
		MemoryUsedMB float64 `json:"memoryUsedMB"`
		DiskUsage    float64 `json:"diskUsage"`
		DiskUsedGB   float64 `json:"diskUsedGB"`
		NetRx        int64   `json:"netRx"`
		NetTx        int64   `json:"netTx"`
		LoadAvg1     float64 `json:"loadAvg1"`
		UptimeSec    int64   `json:"uptimeSec"`
		// System info (optional, updates managed_servers row)
		Hostname  string `json:"hostname"`
		OS        string `json:"os"`
		OSVersion string `json:"osVersion"`
		Arch      string `json:"arch"`
		CPUInfo   string `json:"cpuInfo"`
		CPUCores  int    `json:"cpuCores"`
		RAMTotal  int64  `json:"ramTotal"`
		DiskTotal int64  `json:"diskTotal"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	now := time.Now().UTC()
	// Store metric snapshot.
	_, err := h.DB.Exec(`
		INSERT INTO server_metrics
		  (server_id,timestamp,cpu_usage,memory_usage,memory_used_mb,disk_usage,disk_used_gb,net_rx,net_tx,load_avg1,uptime_sec)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		serverID, now,
		body.CPUUsage, body.MemoryUsage, body.MemoryUsedMB,
		body.DiskUsage, body.DiskUsedGB,
		body.NetRx, body.NetTx, body.LoadAvg1, body.UptimeSec)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Update server info fields if provided.
	if body.Hostname != "" || body.OS != "" {
		_, _ = h.DB.Exec(`
			UPDATE managed_servers SET
			  hostname=CASE WHEN $1!='' THEN $1 ELSE hostname END,
			  os=CASE WHEN $2!='' THEN $2 ELSE os END,
			  os_version=CASE WHEN $3!='' THEN $3 ELSE os_version END,
			  arch=CASE WHEN $4!='' THEN $4 ELSE arch END,
			  cpu_info=CASE WHEN $5!='' THEN $5 ELSE cpu_info END,
			  cpu_cores=CASE WHEN $6>0 THEN $6 ELSE cpu_cores END,
			  ram_total=CASE WHEN $7>0 THEN $7 ELSE ram_total END,
			  disk_total=CASE WHEN $8>0 THEN $8 ELSE disk_total END,
			  last_heartbeat=$9, agent_status='CONNECTED', status='ONLINE', updated_at=$9
			WHERE id=$10`,
			body.Hostname, body.OS, body.OSVersion, body.Arch,
			body.CPUInfo, body.CPUCores, body.RAMTotal, body.DiskTotal,
			now, serverID)
	}
	if h.Broker != nil {
		h.Broker.Publish("server.metrics", gin.H{
			"serverId": serverID, "cpu": body.CPUUsage,
			"memory": body.MemoryUsage, "disk": body.DiskUsage,
		})
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// GET /server-hub/api/servers/:id/metrics
func (h *ServersHandler) Metrics(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rangeMap := map[string]string{
		"1h": "1 hour", "6h": "6 hours", "24h": "24 hours",
		"7d": "7 days", "30d": "30 days",
	}
	r := c.DefaultQuery("range", "1h")
	interval, ok := rangeMap[r]
	if !ok {
		interval = "1 hour"
		r = "1h"
	}
	rows, err := h.DB.Query(`
		SELECT id,server_id,timestamp,cpu_usage,memory_usage,memory_used_mb,
		       disk_usage,disk_used_gb,net_rx,net_tx,load_avg1,uptime_sec
		FROM server_metrics
		WHERE server_id=$1 AND timestamp >= NOW() - INTERVAL '`+interval+`'
		ORDER BY timestamp ASC LIMIT 2000`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var mid, sid uint
		var ts time.Time
		var cpu, mem, memMB, disk, diskGB, load float64
		var rx, tx, uptime int64
		if err := rows.Scan(&mid, &sid, &ts, &cpu, &mem, &memMB, &disk, &diskGB, &rx, &tx, &load, &uptime); err == nil {
			out = append(out, gin.H{
				"id": mid, "serverId": sid, "timestamp": ts,
				"cpuUsage": cpu, "memoryUsage": mem, "memoryUsedMB": memMB,
				"diskUsage": disk, "diskUsedGB": diskGB,
				"netRx": rx, "netTx": tx, "loadAvg1": load, "uptimeSec": uptime,
			})
		}
	}
	c.JSON(http.StatusOK, gin.H{"range": r, "points": out})
}

// GET /server-hub/api/servers/:id/metrics/latest
func (h *ServersHandler) LatestMetrics(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	row := h.DB.QueryRow(`
		SELECT cpu_usage,memory_usage,memory_used_mb,disk_usage,disk_used_gb,
		       net_rx,net_tx,load_avg1,uptime_sec,timestamp
		FROM server_metrics WHERE server_id=$1 ORDER BY timestamp DESC LIMIT 1`, id)
	var cpu, mem, memMB, disk, diskGB, load float64
	var rx, tx, uptime int64
	var ts time.Time
	if err := row.Scan(&cpu, &mem, &memMB, &disk, &diskGB, &rx, &tx, &load, &uptime, &ts); err != nil {
		c.JSON(http.StatusNoContent, gin.H{})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"cpuUsage": cpu, "memoryUsage": mem, "memoryUsedMB": memMB,
		"diskUsage": disk, "diskUsedGB": diskGB,
		"netRx": rx, "netTx": tx, "loadAvg1": load, "uptimeSec": uptime, "timestamp": ts,
	})
}

// ─── Server Groups ────────────────────────────────────────────────────────────

type ServerGroupsHandler struct {
	DB *database.DB
}

func (h *ServerGroupsHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`SELECT id,name,description,created_at FROM server_groups ORDER BY name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uint
		var name, desc string
		var createdAt time.Time
		if err := rows.Scan(&id, &name, &desc, &createdAt); err == nil {
			out = append(out, gin.H{"id": id, "name": name, "description": desc, "createdAt": createdAt})
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *ServerGroupsHandler) Create(c *gin.Context) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	id, err := h.DB.InsertID(`INSERT INTO server_groups (name,description,created_at) VALUES ($1,$2,NOW())`,
		body.Name, body.Description)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": body.Name})
}

func (h *ServerGroupsHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	_, _ = h.DB.Exec(`UPDATE managed_servers SET group_id=NULL WHERE group_id=$1`, id)
	_, _ = h.DB.Exec(`DELETE FROM server_groups WHERE id=$1`, id)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── Alerts ───────────────────────────────────────────────────────────────────

type AlertsHandler struct {
	DB *database.DB
}

func (h *AlertsHandler) List(c *gin.Context) {
	status := c.DefaultQuery("status", "")
	var rows interface{ Next() bool; Scan(...any) error; Close() error }
	var err error
	if status != "" {
		rows, err = h.DB.Query(`
			SELECT id,server_id,condition,threshold,severity,status,message,triggered_at,resolved_at
			FROM alerts WHERE status=$1 ORDER BY triggered_at DESC LIMIT 200`, status)
	} else {
		rows, err = h.DB.Query(`
			SELECT id,server_id,condition,threshold,severity,status,message,triggered_at,resolved_at
			FROM alerts ORDER BY triggered_at DESC LIMIT 200`)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := scanAlerts(rows)
	c.JSON(http.StatusOK, out)
}

func (h *AlertsHandler) Resolve(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	now := time.Now().UTC()
	_, err = h.DB.Exec(`UPDATE alerts SET status='RESOLVED', resolved_at=$1 WHERE id=$2`, now, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "resolve-alert", "alert", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── Notifications ────────────────────────────────────────────────────────────

type NotificationsInAppHandler struct {
	DB *database.DB
}

func (h *NotificationsInAppHandler) List(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	rows, err := h.DB.Query(`
		SELECT id,username,title,body,category,read,server_id,alert_id,created_at
		FROM in_app_notifications WHERE username=$1 ORDER BY created_at DESC LIMIT 100`, u)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var nid uint
		var uname, title, body, cat string
		var read bool
		var sid, aid *uint
		var createdAt time.Time
		if err := rows.Scan(&nid, &uname, &title, &body, &cat, &read, &sid, &aid, &createdAt); err == nil {
			out = append(out, gin.H{
				"id": nid, "username": uname, "title": title, "body": body,
				"category": cat, "read": read, "serverId": sid, "alertId": aid, "createdAt": createdAt,
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *NotificationsInAppHandler) MarkRead(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	_, _ = h.DB.Exec(`UPDATE in_app_notifications SET read=true WHERE id=$1 AND username=$2`, id, u)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NotificationsInAppHandler) MarkAllRead(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	_, _ = h.DB.Exec(`UPDATE in_app_notifications SET read=true WHERE username=$1`, u)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// ─── helpers ─────────────────────────────────────────────────────────────────

func generateToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return
}

type scannable interface {
	Next() bool
	Scan(...any) error
	Close() error
}

func scanServer(rows scannable) gin.H {
	var id uint
	var name, hostname, ip, os, osv, arch, cpuInfo, status, agentStatus string
	var cpuCores int
	var ramTotal, diskTotal int64
	var lastHB *time.Time
	var groupID *uint
	var createdAt, updatedAt time.Time
	if err := rows.Scan(&id, &name, &hostname, &ip, &os, &osv, &arch, &cpuInfo, &cpuCores,
		&ramTotal, &diskTotal, &status, &agentStatus, &lastHB, &groupID, &createdAt, &updatedAt); err != nil {
		return nil
	}
	return gin.H{
		"id": id, "name": name, "hostname": hostname, "ipAddress": ip,
		"os": os, "osVersion": osv, "arch": arch, "cpuInfo": cpuInfo, "cpuCores": cpuCores,
		"ramTotal": ramTotal, "diskTotal": diskTotal,
		"status": status, "agentStatus": agentStatus,
		"lastHeartbeat": lastHB, "groupId": groupID,
		"createdAt": createdAt, "updatedAt": updatedAt,
	}
}

func scanAlerts(rows scannable) []gin.H {
	out := []gin.H{}
	for rows.Next() {
		var id uint
		var sid *uint
		var cond, severity, status, message string
		var threshold float64
		var triggeredAt time.Time
		var resolvedAt *time.Time
		if err := rows.Scan(&id, &sid, &cond, &threshold, &severity, &status, &message, &triggeredAt, &resolvedAt); err == nil {
			out = append(out, gin.H{
				"id": id, "serverId": sid, "condition": cond, "threshold": threshold,
				"severity": severity, "status": status, "message": message,
				"triggeredAt": triggeredAt, "resolvedAt": resolvedAt,
			})
		}
	}
	return out
}
