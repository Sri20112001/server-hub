package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/vgbackup"
)

// VgMgmtHandler serves the authenticated database-backup management API under /server-hub/api/db-backups.
type VgMgmtHandler struct {
	DB     *database.DB
	EncKey []byte
	Broker *events.Broker
}

// actor resolves caller identity and role from the JWT context.
func (h *VgMgmtHandler) actor(c *gin.Context) (username string, role vgbackup.BackupRole, userID uint, ok bool) {
	u, r := middleware.CurrentUser(c)
	if u == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthenticated"})
		return "", "", 0, false
	}
	bRole, err := vgbackup.MapBackupRole(r)
	if err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "invalid role"})
		return "", "", 0, false
	}
	var id uint
	if err := h.DB.QueryRow(`SELECT id FROM users WHERE username=?`, u).Scan(&id); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return "", "", 0, false
	}
	return u, bRole, id, true
}

func (h *VgMgmtHandler) publish(eventType string, data interface{}) {
	if h.Broker != nil {
		h.Broker.Publish(eventType, data)
	}
}

func (h *VgMgmtHandler) audit(c *gin.Context, action, resource, resourceID, result, detail string) {
	u, _ := middleware.CurrentUser(c)
	if u == "" {
		u = "system"
	}
	audit.Write(h.DB, u, action, resource, resourceID, result, detail)
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. Storage Targets
// ─────────────────────────────────────────────────────────────────────────────

type createStorageRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Type         vgbackup.VgStorageType `json:"type" binding:"required"`
	Bucket       string                 `json:"bucket"`
	Region       string                 `json:"region"`
	Endpoint     string                 `json:"endpoint"`
	UsePathStyle bool                   `json:"use_path_style"`
	AccessKey    string                 `json:"access_key"`
	SecretKey    string                 `json:"secret_key"`
	Path         string                 `json:"path"`
}

func (h *VgMgmtHandler) ListStorageTargets(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var targets []vgbackup.VgStorageTarget
	query := h.DB.GDB.Order("created_at ASC")
	if role != vgbackup.BackupAdmin {
		query = query.Where("created_by_user_id = ?", userID)
	}
	if err := query.Find(&targets).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list storage targets failed"})
		return
	}
	for i := range targets {
		targets[i].HasCredentials = targets[i].EncryptedAccessKey != "" || targets[i].EncryptedSecretKey != ""
	}
	if targets == nil {
		targets = []vgbackup.VgStorageTarget{}
	}
	c.JSON(http.StatusOK, targets)
}

func (h *VgMgmtHandler) GetStorageTarget(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var target vgbackup.VgStorageTarget
	if err := h.DB.GDB.Where("id = ?", id).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, target.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}
	target.HasCredentials = target.EncryptedAccessKey != "" || target.EncryptedSecretKey != ""
	c.JSON(http.StatusOK, target)
}

func (h *VgMgmtHandler) CreateStorageTarget(c *gin.Context) {
	_, _, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var req createStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	switch req.Type {
	case vgbackup.VgStorageLocal:
		cleanPath := strings.TrimSpace(req.Path)
		if cleanPath == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path is required for LOCAL targets"})
			return
		}
		// Portable absolute check (either OS convention): target paths are
		// agent-local, so host-native filepath.IsAbs alone misjudges
		// cross-platform agents. Relative paths stay rejected.
		if !vgbackup.VgIsAbsPath(cleanPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path must be an absolute path"})
			return
		}
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("unsupported storage type %q (supported: LOCAL)", req.Type)})
		return
	}

	target := vgbackup.VgStorageTarget{
		CreatedByUserID: userID,
		Name:            strings.TrimSpace(req.Name),
		Type:            req.Type,
		Bucket:          strings.TrimSpace(req.Bucket),
		Region:          strings.TrimSpace(req.Region),
		Endpoint:        strings.TrimSpace(req.Endpoint),
		UsePathStyle:    req.UsePathStyle,
		Path:            filepath.Clean(strings.TrimSpace(req.Path)),
		HasCredentials:  req.AccessKey != "" || req.SecretKey != "",
	}
	if err := h.DB.GDB.Create(&target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create storage target failed"})
		return
	}
	h.audit(c, "create", "vg_storage_target", target.ID.String(), "ok", target.Name)
	h.publish("vgbackup.storage", gin.H{"id": target.ID.String(), "action": "created"})
	c.JSON(http.StatusCreated, target)
}

func (h *VgMgmtHandler) UpdateStorageTarget(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var target vgbackup.VgStorageTarget
	if err := h.DB.GDB.Where("id = ?", id).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, target.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}

	var req createStorageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if name := strings.TrimSpace(req.Name); name != "" {
		updates["name"] = name
	}
	if req.Type != "" {
		if req.Type != vgbackup.VgStorageLocal {
			c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported storage type (supported: LOCAL)"})
			return
		}
		updates["type"] = req.Type
	}
	if cleanPath := strings.TrimSpace(req.Path); cleanPath != "" {
		if !vgbackup.VgIsAbsPath(cleanPath) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "path must be an absolute path"})
			return
		}
		updates["path"] = filepath.Clean(cleanPath)
	}

	if err := h.DB.GDB.Model(&target).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update storage target failed"})
		return
	}
	h.DB.GDB.First(&target, target.ID)
	target.HasCredentials = target.EncryptedAccessKey != "" || target.EncryptedSecretKey != ""
	h.audit(c, "update", "vg_storage_target", target.ID.String(), "ok", target.Name)
	h.publish("vgbackup.storage", gin.H{"id": target.ID.String(), "action": "updated"})
	c.JSON(http.StatusOK, target)
}

func (h *VgMgmtHandler) DeleteStorageTarget(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var target vgbackup.VgStorageTarget
	if err := h.DB.GDB.Where("id = ?", id).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, target.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}

	var jobCount int64
	h.DB.GDB.Model(&vgbackup.VgBackupJob{}).Where("storage_target_id = ?", id).Count(&jobCount)
	if jobCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("storage target is used by %d backup job(s); reassign or delete them first", jobCount)})
		return
	}

	if err := h.DB.GDB.Delete(&target).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete storage target failed"})
		return
	}
	h.audit(c, "delete", "vg_storage_target", target.ID.String(), "ok", target.Name)
	h.publish("vgbackup.storage", gin.H{"id": target.ID.String(), "action": "deleted"})
	c.JSON(http.StatusNoContent, nil)
}

func (h *VgMgmtHandler) TestStorageTarget(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	// Body-carried id (the route is POST /storage-targets/test): the target
	// being probed must be explicit in the request, never inferred.
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var target vgbackup.VgStorageTarget
	if err := h.DB.GDB.Where("id = ?", id).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, target.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}

	if target.Type != vgbackup.VgStorageLocal {
		c.JSON(http.StatusBadRequest, gin.H{"error": "connection testing is only supported for LOCAL targets in this phase"})
		return
	}
	if target.Path == "" {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "LOCAL storage target has no path configured"})
		return
	}
	// The probe writes a file: refuse symlinked paths outright so a
	// malicious or mistaken symlink can never redirect the probe write
	// outside the target directory. (Legit bind mounts are not symlinks.)
	cleanPath := filepath.Clean(target.Path)
	// Refuse symlinked paths outright so a malicious or mistaken symlink
	// can never redirect the probe write outside the target directory.
	// Nonexistent paths fall through to the probe below (502), and other
	// resolution errors do too — only a PROVEN escape is rejected here.
	if realPath, err := filepath.EvalSymlinks(cleanPath); err == nil && realPath != cleanPath {
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "storage path must resolve to a real directory (symlinks rejected)"})
		return
	}
	start := time.Now()
	// Test if directory exists or can be written to
	testFile := filepath.Join(cleanPath, ".serverhub-storage-test-"+uuid.New().String())
	if err := os.WriteFile(testFile, []byte("ok"), 0600); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "storage path not writable from server: " + err.Error()})
		return
	}
	_ = os.Remove(testFile)

	c.JSON(http.StatusOK, gin.H{
		"status":     "ok",
		"latency_ms": time.Since(start).Milliseconds(),
		"path":       target.Path,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. Database Connections
// ─────────────────────────────────────────────────────────────────────────────

type connectionRequest struct {
	Name      string   `json:"name" binding:"required"`
	Type      string   `json:"type" binding:"required"`
	AgentID   string   `json:"agent_id" binding:"required"`
	Host      string   `json:"host" binding:"required"`
	Port      int      `json:"port"`
	Username  string   `json:"username"`
	Password  string   `json:"password"`
	Databases []string `json:"databases"`
}

func normalizeConnectionType(raw string) (vgbackup.VgConnectionType, error) {
	t := vgbackup.VgConnectionType(strings.ToUpper(strings.TrimSpace(raw)))
	switch t {
	case vgbackup.VgConnectionPostgres, vgbackup.VgConnectionMongo, vgbackup.VgConnectionMssql:
		return t, nil
	case "MSSQL_SERVER", "SQL_SERVER":
		return vgbackup.VgConnectionMssql, nil
	}
	return "", fmt.Errorf("unsupported connection type %q (supported: POSTGRES, MONGODB, MSSQL)", raw)
}

func defaultPortFor(connType vgbackup.VgConnectionType) int {
	switch connType {
	case vgbackup.VgConnectionPostgres:
		return 5432
	case vgbackup.VgConnectionMongo:
		return 27017
	case vgbackup.VgConnectionMssql:
		return 1433
	}
	return 0
}

func toConnectionDTO(c vgbackup.VgDatabaseConnection) gin.H {
	meta := c.MetadataList()
	return gin.H{
		"id":              c.ID.String(),
		"created_by":      c.CreatedByUserID,
		"agent_id":        c.AgentID.String(),
		"name":            c.Name,
		"type":            string(c.Type),
		"host":            c.Host,
		"port":            c.Port,
		"username":        c.Username,
		"status":          string(c.Status),
		"last_checked_at": c.LastCheckedAt,
		"databases":       meta,
		"database_count":  len(meta),
		"agent":           c.Agent,
		"created_at":      c.CreatedAt,
		"updated_at":      c.UpdatedAt,
	}
}

func (h *VgMgmtHandler) ListConnections(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var conns []vgbackup.VgDatabaseConnection
	query := h.DB.GDB.Preload("Agent").Order("created_at ASC")
	if role != vgbackup.BackupAdmin {
		query = query.Where("created_by_user_id = ?", userID)
	}
	if err := query.Find(&conns).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list connections failed"})
		return
	}
	out := make([]gin.H, 0, len(conns))
	for _, cn := range conns {
		out = append(out, toConnectionDTO(cn))
	}
	c.JSON(http.StatusOK, out)
}

func (h *VgMgmtHandler) GetConnection(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var conn vgbackup.VgDatabaseConnection
	if err := h.DB.GDB.Preload("Agent").Where("id = ?", id).First(&conn).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, conn.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	c.JSON(http.StatusOK, toConnectionDTO(conn))
}

func (h *VgMgmtHandler) CreateConnection(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var req connectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	connType, err := normalizeConnectionType(req.Type)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_id"})
		return
	}
	var agent vgbackup.VgAgent
	if err := h.DB.GDB.Where("id = ?", agentID).First(&agent).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, agent.CreatedByUserID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent not found"})
		return
	}

	port := req.Port
	if port == 0 {
		port = defaultPortFor(connType)
	}
	if port < 1 || port > 65535 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "port must be 1-65535"})
		return
	}

	encPassword, err := vgEncrypt(h.EncKey, req.Password)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encryption failed"})
		return
	}

	conn := vgbackup.VgDatabaseConnection{
		CreatedByUserID:   userID,
		AgentID:           agentID,
		Name:              strings.TrimSpace(req.Name),
		Type:              connType,
		Host:              strings.TrimSpace(req.Host),
		Port:              port,
		Username:          strings.TrimSpace(req.Username),
		EncryptedPassword: encPassword,
		Status:            vgbackup.VgConnectionUnknown,
		DatabaseNames:     strings.Join(req.Databases, "\n"),
	}
	if err := h.DB.GDB.Create(&conn).Error; err != nil {
		// Names are unique per owner (unique index): concurrent duplicates
		// collapse to 409 instead of forking identical connections.
		if strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "a connection with this name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create connection failed"})
		return
	}
	h.DB.GDB.Preload("Agent").First(&conn, conn.ID)
	h.audit(c, "create", "vg_connection", conn.ID.String(), "ok", conn.Name)
	h.publish("vgbackup.connection", gin.H{"id": conn.ID.String(), "action": "created"})
	c.JSON(http.StatusCreated, toConnectionDTO(conn))
}

func (h *VgMgmtHandler) UpdateConnection(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var conn vgbackup.VgDatabaseConnection
	if err := h.DB.GDB.Where("id = ?", id).First(&conn).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, conn.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}

	var req connectionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{}
	if name := strings.TrimSpace(req.Name); name != "" {
		updates["name"] = name
	}
	if req.Type != "" {
		connType, err := normalizeConnectionType(req.Type)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		updates["type"] = connType
	}
	if host := strings.TrimSpace(req.Host); host != "" {
		updates["host"] = host
	}
	if req.Port > 0 {
		if req.Port < 1 || req.Port > 65535 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "port must be 1-65535"})
			return
		}
		updates["port"] = req.Port
	}
	if req.Username != "" {
		updates["username"] = strings.TrimSpace(req.Username)
	}
	if req.Password != "" {
		enc, err := vgEncrypt(h.EncKey, req.Password)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "encryption failed"})
			return
		}
		updates["encrypted_password"] = enc
		updates["status"] = string(vgbackup.VgConnectionUnknown)
	}
	if req.Databases != nil {
		updates["database_names"] = strings.Join(req.Databases, "\n")
	}

	if err := h.DB.GDB.Model(&conn).Updates(updates).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			c.JSON(http.StatusConflict, gin.H{"error": "a connection with this name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update connection failed"})
		return
	}
	h.DB.GDB.Preload("Agent").First(&conn, conn.ID)
	h.audit(c, "update", "vg_connection", conn.ID.String(), "ok", conn.Name)
	h.publish("vgbackup.connection", gin.H{"id": conn.ID.String(), "action": "updated"})
	c.JSON(http.StatusOK, toConnectionDTO(conn))
}

func (h *VgMgmtHandler) DeleteConnection(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var conn vgbackup.VgDatabaseConnection
	if err := h.DB.GDB.Where("id = ?", id).First(&conn).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, conn.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}

	var jobCount int64
	h.DB.GDB.Model(&vgbackup.VgBackupJob{}).Where("connection_id = ?", id).Count(&jobCount)
	if jobCount > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("connection is used by %d backup job(s); reassign or delete them first", jobCount)})
		return
	}

	if err := h.DB.GDB.Delete(&conn).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete connection failed"})
		return
	}
	h.audit(c, "delete", "vg_connection", conn.ID.String(), "ok", conn.Name)
	h.publish("vgbackup.connection", gin.H{"id": conn.ID.String(), "action": "deleted"})
	c.JSON(http.StatusNoContent, nil)
}

func (h *VgMgmtHandler) TestConnection(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	// Body-carried id (the route is POST /connections/test).
	var req struct {
		ID string `json:"id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	id, err := uuid.Parse(req.ID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var conn vgbackup.VgDatabaseConnection
	if err := h.DB.GDB.Preload("Agent").Where("id = ?", id).First(&conn).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, conn.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "connection not found"})
		return
	}

	if _, err := vgDecrypt(h.EncKey, conn.EncryptedPassword); err != nil {
		now := time.Now()
		h.DB.GDB.Model(&conn).Updates(map[string]interface{}{"status": string(vgbackup.VgConnectionError), "last_checked_at": &now})
		c.JSON(http.StatusBadRequest, gin.H{"status": "error", "error": "stored credential is unusable; re-enter the password"})
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	d := net.Dialer{Timeout: 5 * time.Second}
	nc, dialErr := d.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", conn.Host, conn.Port))
	if dialErr != nil {
		now := time.Now()
		h.DB.GDB.Model(&conn).Updates(map[string]interface{}{"status": string(vgbackup.VgConnectionError), "last_checked_at": &now})
		// Categorized, never echoed: the raw dial error contains the
		// probed host:port and OS-specific strings, which would turn this
		// operator endpoint into a network-mapping oracle. The operator
		// typed the target themselves; the category is all they need.
		c.JSON(http.StatusBadGateway, gin.H{"status": "error", "error": "host is unreachable: " + dialFailureCategory(dialErr)})
		return
	}
	nc.Close()

	now := time.Now()
	h.DB.GDB.Model(&conn).Updates(map[string]interface{}{
		"status":          string(vgbackup.VgConnectionConnected),
		"last_checked_at": &now,
	})

	c.JSON(http.StatusOK, gin.H{
		"status":       "ok",
		"latency_ms":   time.Since(start).Milliseconds(),
		"databases":    conn.MetadataList(),
		"agent_status": string(conn.Agent.Status),
	})
}

// dialFailureCategory maps a dial error to a stable category without
// echoing the probed address: dial strings embed host:port plus OS-specific
// detail, which would make this endpoint a network-mapping oracle.
func dialFailureCategory(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "refused"):
		return "connection refused"
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "deadline exceeded"):
		return "connection timed out"
	case strings.Contains(msg, "no such host"), strings.Contains(msg, "not known"):
		return "DNS resolution failed"
	default:
		return "connection failed"
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. Backup Policies
// ─────────────────────────────────────────────────────────────────────────────

var cronParser = cron.NewParser(
	cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
)

type policyRequest struct {
	Name                string `json:"name"`
	Strategy            string `json:"strategy"`
	CronExpr            string `json:"cron_expr"`
	Timezone            string `json:"timezone"`
	Enabled             *bool  `json:"enabled"`
	Mode                string `json:"mode"`
	Encrypted           *bool  `json:"encrypted"`
	RetentionDays       *int   `json:"retention_days"`
	VerificationEnabled *bool  `json:"verification_enabled"`
	MaxRetries          *int   `json:"max_retries"`
	RetryDelaySeconds   *int   `json:"retry_delay_seconds"`
	SLATargetMinutes    *int   `json:"sla_target_minutes"`
	RPOTargetMinutes    *int   `json:"rpo_target_minutes"`
	RTOTargetMinutes    *int   `json:"rto_target_minutes"`
}

func boolOr(b *bool, def bool) bool {
	if b == nil {
		return def
	}
	return *b
}

func intOr(i *int, def int) int {
	if i == nil {
		return def
	}
	return *i
}

func validatePolicy(req *policyRequest) (vgbackup.VgBackupPolicy, error) {
	p := vgbackup.VgBackupPolicy{}
	if strings.TrimSpace(req.Name) == "" {
		return p, fmt.Errorf("name is required")
	}
	strategy := strings.ToUpper(strings.TrimSpace(req.Strategy))
	if strategy == "" {
		strategy = vgbackup.VgStrategyFull
	}
	if strategy != vgbackup.VgStrategyFull {
		return p, fmt.Errorf("strategy must be FULL (incremental/differential are not implemented)")
	}
	mode := vgbackup.VgBackupMode(strings.ToUpper(strings.TrimSpace(req.Mode)))
	if mode == "" {
		mode = vgbackup.VgModeNormal
	}
	if mode != vgbackup.VgModeNormal && mode != vgbackup.VgModeCompressed {
		return p, fmt.Errorf("mode must be NORMAL or COMPRESSED")
	}
	retention := intOr(req.RetentionDays, 30)
	if retention < 0 || retention > 3650 {
		return p, fmt.Errorf("retention_days must be 0-3650")
	}
	maxRetries := intOr(req.MaxRetries, 0)
	if maxRetries < 0 || maxRetries > 10 {
		return p, fmt.Errorf("max_retries must be 0-10")
	}
	delay := intOr(req.RetryDelaySeconds, 300)
	if delay < 0 || delay > 86400 {
		return p, fmt.Errorf("retry_delay_seconds must be 0-86400")
	}
	for _, v := range []*int{req.SLATargetMinutes, req.RPOTargetMinutes, req.RTOTargetMinutes} {
		if v != nil && (*v < 0 || *v > 525600) {
			return p, fmt.Errorf("SLA/RPO/RTO targets must be 0-525600 minutes")
		}
	}
	cronExpr := strings.TrimSpace(req.CronExpr)
	if cronExpr != "" {
		if _, err := cronParser.Parse(cronExpr); err != nil {
			return p, fmt.Errorf("invalid cron_expr: %w", err)
		}
	}
	tz := strings.TrimSpace(req.Timezone)
	if tz == "" {
		tz = "UTC"
	}
	// Reject unknown timezones at write time: the scheduler silently
	// falls back to UTC for unresolvable names, which would run the job
	// at a different wall-clock time than the operator configured.
	if _, err := time.LoadLocation(tz); err != nil {
		return p, fmt.Errorf("invalid timezone %q", tz)
	}

	p.Name = strings.TrimSpace(req.Name)
	p.Strategy = strategy
	p.CronExpr = cronExpr
	p.Timezone = tz
	p.Enabled = boolOr(req.Enabled, true)
	p.Mode = mode
	p.Encrypted = boolOr(req.Encrypted, false)
	p.RetentionDays = retention
	p.VerificationEnabled = boolOr(req.VerificationEnabled, true)
	p.MaxRetries = maxRetries
	p.RetryDelaySeconds = delay
	p.SLATargetMinutes = intOr(req.SLATargetMinutes, 0)
	p.RPOTargetMinutes = intOr(req.RPOTargetMinutes, 0)
	p.RTOTargetMinutes = intOr(req.RTOTargetMinutes, 0)
	return p, nil
}

func (h *VgMgmtHandler) ListPolicies(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var policies []vgbackup.VgBackupPolicy
	query := h.DB.GDB.Order("created_at ASC")
	if role != vgbackup.BackupAdmin {
		query = query.Where("created_by_user_id = ?", userID)
	}
	if err := query.Find(&policies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list policies failed"})
		return
	}
	if policies == nil {
		policies = []vgbackup.VgBackupPolicy{}
	}
	c.JSON(http.StatusOK, policies)
}

func (h *VgMgmtHandler) GetPolicy(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var p vgbackup.VgBackupPolicy
	if err := h.DB.GDB.Where("id = ?", id).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, p.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	var jobCount int64
	h.DB.GDB.Model(&vgbackup.VgBackupJob{}).Where("policy_id = ?", id).Count(&jobCount)
	c.JSON(http.StatusOK, gin.H{"policy": p, "attached_jobs": jobCount})
}

func (h *VgMgmtHandler) CreatePolicy(c *gin.Context) {
	_, _, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var req policyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p, err := validatePolicy(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	p.CreatedByUserID = userID

	want := p
	if err := h.DB.GDB.Create(&p).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create policy failed"})
		return
	}
	// GORM default tag workaround
	h.DB.GDB.Model(&p).Updates(map[string]interface{}{
		"enabled":              want.Enabled,
		"encrypted":            want.Encrypted,
		"verification_enabled": want.VerificationEnabled,
	})
	h.DB.GDB.First(&p, p.ID)
	h.audit(c, "create", "vg_policy", p.ID.String(), "ok", p.Name)
	h.publish("vgbackup.policy", gin.H{"id": p.ID.String(), "action": "created"})
	c.JSON(http.StatusCreated, p)
}

func (h *VgMgmtHandler) UpdatePolicy(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var p vgbackup.VgBackupPolicy
	if err := h.DB.GDB.Where("id = ?", id).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, p.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	var req policyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := validatePolicy(&req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.DB.GDB.Model(&p).Updates(map[string]interface{}{
		"name":                 updated.Name,
		"strategy":             updated.Strategy,
		"cron_expr":            updated.CronExpr,
		"timezone":             updated.Timezone,
		"enabled":              updated.Enabled,
		"mode":                 updated.Mode,
		"encrypted":            updated.Encrypted,
		"retention_days":       updated.RetentionDays,
		"verification_enabled": updated.VerificationEnabled,
		"max_retries":          updated.MaxRetries,
		"retry_delay_seconds":  updated.RetryDelaySeconds,
		"sla_target_minutes":   updated.SLATargetMinutes,
		"rpo_target_minutes":   updated.RPOTargetMinutes,
		"rto_target_minutes":   updated.RTOTargetMinutes,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update policy failed"})
		return
	}
	h.DB.GDB.First(&p, p.ID)
	h.audit(c, "update", "vg_policy", p.ID.String(), "ok", p.Name)
	h.publish("vgbackup.policy", gin.H{"id": p.ID.String(), "action": "updated"})
	c.JSON(http.StatusOK, p)
}

func (h *VgMgmtHandler) DeletePolicy(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var p vgbackup.VgBackupPolicy
	if err := h.DB.GDB.Where("id = ?", id).First(&p).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, p.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "policy not found"})
		return
	}

	// Detach with behavior preservation: attached jobs run on the
	// policy's settings, so deleting the policy materializes those
	// settings into each job's inline columns (and schedule row) before
	// unlinking. Without this, jobs would silently fall back to stale
	// creation-time inline values — possibly retention 0 (disabled) or
	// an empty schedule that stops a previously scheduled job.
	var attached []vgbackup.VgBackupJob
	if err := h.DB.GDB.Where("policy_id = ?", id).Find(&attached).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete policy failed"})
		return
	}
	txErr := h.DB.GDB.Transaction(func(tx *gorm.DB) error {
		for i := range attached {
			job := &attached[i]
			if err := tx.Model(job).Updates(map[string]interface{}{
				"mode":               p.Mode,
				"encrypted":          p.Encrypted,
				"retention_days":     p.RetentionDays,
				"sla_target_minutes": p.SLATargetMinutes,
				"rpo_target_minutes": p.RPOTargetMinutes,
				"rto_target_minutes": p.RTOTargetMinutes,
				"policy_id":          nil,
			}).Error; err != nil {
				return err
			}
			if strings.TrimSpace(p.CronExpr) != "" {
				tz := strings.TrimSpace(p.Timezone)
				if tz == "" {
					tz = "UTC"
				}
				var sched vgbackup.VgBackupSchedule
				if err := tx.Where("backup_job_id = ?", job.ID).First(&sched).Error; err != nil {
					if err == gorm.ErrRecordNotFound {
						if err := tx.Create(&vgbackup.VgBackupSchedule{
							BackupJobID: job.ID, CronExpr: p.CronExpr, Timezone: tz,
						}).Error; err != nil {
							return err
						}
					} else {
						return err
					}
				} else if err := tx.Model(&sched).Updates(map[string]interface{}{
					"cron_expr": p.CronExpr, "timezone": tz,
				}).Error; err != nil {
					return err
				}
			}
		}
		return tx.Delete(&p).Error
	})
	if txErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "delete policy failed"})
		return
	}
	h.audit(c, "delete", "vg_policy", p.ID.String(), "ok", p.Name)
	h.publish("vgbackup.policy", gin.H{"id": p.ID.String(), "action": "deleted"})
	c.JSON(http.StatusNoContent, nil)
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. Backup Jobs
// ─────────────────────────────────────────────────────────────────────────────

type createJobRequest struct {
	AgentID          string                      `json:"agent_id" binding:"required"`
	StorageTargetID  string                      `json:"storage_target_id" binding:"required"`
	Name             string                      `json:"name" binding:"required"`
	SourceType       vgbackup.VgBackupSourceType `json:"source_type" binding:"required"`
	ConnectionID     string                      `json:"connection_id"`
	SourcePath       string                      `json:"source_path"`
	SourceDatabase   string                      `json:"source_database"`
	IncludePatterns  string                      `json:"include_patterns"`
	ExcludePatterns  string                      `json:"exclude_patterns"`
	Mode             vgbackup.VgBackupMode       `json:"mode"`
	Encrypted        bool                        `json:"encrypted"`
	RetentionDays    int                         `json:"retention_days"`
	ExportFormat     string                      `json:"export_format"`
	CronExpr         string                      `json:"cron_expr"`
	Timezone         string                      `json:"timezone"`
	PolicyID         string                      `json:"policy_id"`
	SLATargetMinutes int                         `json:"sla_target_minutes"`
	RPOTargetMinutes int                         `json:"rpo_target_minutes"`
	RTOTargetMinutes int                         `json:"rto_target_minutes"`
}

func isDatabaseSource(t vgbackup.VgBackupSourceType) bool {
	return t == vgbackup.VgSourcePostgres || t == vgbackup.VgSourceMongo ||
		t == vgbackup.VgSourceSQLServer || t == vgbackup.VgSourceMssqlServer
}

func normalizeExportFormat(sourceType vgbackup.VgBackupSourceType, raw string) (string, error) {
	format := strings.ToUpper(strings.TrimSpace(raw))
	if format == "" {
		format = "ARCHIVE"
	}
	if sourceType == vgbackup.VgSourceMongo {
		switch format {
		case "ARCHIVE", "JSON", "CSV":
			return format, nil
		default:
			return "", fmt.Errorf("export_format must be ARCHIVE, JSON, or CSV")
		}
	}
	if raw != "" {
		return "", fmt.Errorf("export_format only applies to MONGODB jobs")
	}
	return format, nil
}

// validateJobSourceType rejects unknown source types at write time: the
// value flows verbatim into the agent claim DTO, so an unconstrained
// string would dispatch unexecutable work instead of failing fast here.
func validateJobSourceType(t vgbackup.VgBackupSourceType) error {
	switch t {
	case vgbackup.VgSourceFilesystem, vgbackup.VgSourcePostgres,
		vgbackup.VgSourceMongo, vgbackup.VgSourceSQLServer,
		vgbackup.VgSourceMssqlServer, vgbackup.VgSourceDBF:
		return nil
	}
	return fmt.Errorf("unsupported source_type %q (supported: FILESYSTEM, POSTGRES, MONGODB, SQL_SERVER, MSSQL_SERVER, DBF)", string(t))
}

// normalizeJobMode validates an explicit mode; empty means "keep the
// existing/default" so partial updates can never blank the column with a
// value the agent cannot interpret.
func normalizeJobMode(raw vgbackup.VgBackupMode, def vgbackup.VgBackupMode) (vgbackup.VgBackupMode, error) {
	if strings.TrimSpace(string(raw)) == "" {
		return def, nil
	}
	m := vgbackup.VgBackupMode(strings.ToUpper(strings.TrimSpace(string(raw))))
	if m != vgbackup.VgModeNormal && m != vgbackup.VgModeCompressed {
		return "", fmt.Errorf("mode must be NORMAL or COMPRESSED")
	}
	return m, nil
}

// validateJobSchedule rejects unparseable cron expressions and unknown
// timezones at write time: the scheduler silently skips both (a bad cron
// never fires, a bad timezone runs as UTC), which would leave a job that
// looks scheduled but never runs at the configured time.
func validateJobSchedule(cronExpr, timezone string) error {
	if strings.TrimSpace(cronExpr) != "" {
		if _, err := cronParser.Parse(strings.TrimSpace(cronExpr)); err != nil {
			return fmt.Errorf("invalid cron_expr: %w", err)
		}
	}
	if tz := strings.TrimSpace(timezone); tz != "" {
		if _, err := time.LoadLocation(tz); err != nil {
			return fmt.Errorf("invalid timezone %q", tz)
		}
	}
	return nil
}

// validateJobRetention bounds retention_days to the same 0-3650 range as
// policies. 0 disables retention; negatives previously disabled it
// silently via the scheduler's `retention_days > 0` filter.
func validateJobRetention(days int) error {
	if days < 0 || days > 3650 {
		return fmt.Errorf("retention_days must be 0-3650")
	}
	return nil
}

// validateJobTarget bounds one SLA/RPO/RTO target like policies do.
func validateJobTarget(name string, v int) error {
	if v < 0 || v > 525600 {
		return fmt.Errorf("%s must be 0-525600 minutes", name)
	}
	return nil
}

func (h *VgMgmtHandler) ListBackupJobs(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var jobs []vgbackup.VgBackupJob
	query := h.DB.GDB.Preload("Schedule").Preload("Policy").Preload("Agent").Preload("StorageTarget").Preload("Connection").Order("created_at ASC")
	if role != vgbackup.BackupAdmin {
		query = query.Where("created_by_user_id = ?", userID)
	}
	if err := query.Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list backup jobs failed"})
		return
	}
	if jobs == nil {
		jobs = []vgbackup.VgBackupJob{}
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *VgMgmtHandler) GetBackupJob(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var job vgbackup.VgBackupJob
	if err := h.DB.GDB.Preload("Schedule").Preload("Policy").Preload("Agent").Preload("StorageTarget").Preload("Connection").
		Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, job.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *VgMgmtHandler) CreateBackupJob(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var req createJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_id"})
		return
	}
	storageID, err := uuid.Parse(req.StorageTargetID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid storage_target_id"})
		return
	}

	var agent vgbackup.VgAgent
	if err := h.DB.GDB.Where("id = ?", agentID).First(&agent).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, agent.CreatedByUserID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent not found"})
		return
	}

	var target vgbackup.VgStorageTarget
	if err := h.DB.GDB.Where("id = ?", storageID).First(&target).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage target not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, target.CreatedByUserID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage target not found"})
		return
	}

	mode, err := normalizeJobMode(req.Mode, vgbackup.VgModeNormal)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateJobSourceType(req.SourceType); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	exportFormat, err := normalizeExportFormat(req.SourceType, req.ExportFormat)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	retDays := req.RetentionDays
	if retDays == 0 {
		retDays = 30
	}
	if err := validateJobRetention(retDays); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateJobSchedule(req.CronExpr, req.Timezone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, tv := range []struct {
		name string
		v    int
	}{
		{"sla_target_minutes", req.SLATargetMinutes},
		{"rpo_target_minutes", req.RPOTargetMinutes},
		{"rto_target_minutes", req.RTOTargetMinutes},
	} {
		if err := validateJobTarget(tv.name, tv.v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	var connID *uuid.UUID
	if strings.TrimSpace(req.ConnectionID) != "" {
		parsed, err := uuid.Parse(req.ConnectionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection_id"})
			return
		}
		var conn vgbackup.VgDatabaseConnection
		if err := h.DB.GDB.Where("id = ?", parsed).First(&conn).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection not found"})
			return
		}
		if !vgbackup.CanAccessBackupResource(role, userID, conn.CreatedByUserID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection not found"})
			return
		}
		if isDatabaseSource(req.SourceType) && conn.AgentID != agentID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection belongs to a different agent; select the connection's agent"})
			return
		}
		connID = &parsed
	} else if isDatabaseSource(req.SourceType) && strings.TrimSpace(req.SourceDatabase) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_database is required for database jobs"})
		return
	}
	if (req.SourceType == vgbackup.VgSourceFilesystem || req.SourceType == vgbackup.VgSourceDBF) &&
		strings.TrimSpace(req.SourcePath) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_path is required for filesystem jobs"})
		return
	}

	var policyID *uuid.UUID
	if strings.TrimSpace(req.PolicyID) != "" {
		parsed, err := uuid.Parse(req.PolicyID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy_id"})
			return
		}
		var policy vgbackup.VgBackupPolicy
		if err := h.DB.GDB.Where("id = ?", parsed).First(&policy).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "policy not found"})
			return
		}
		if !vgbackup.CanAccessBackupResource(role, userID, policy.CreatedByUserID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "policy not found"})
			return
		}
		policyID = &parsed
	}

	job := vgbackup.VgBackupJob{
		CreatedByUserID:  userID,
		AgentID:          agentID,
		StorageTargetID:  storageID,
		PolicyID:         policyID,
		Name:             req.Name,
		SourceType:       req.SourceType,
		ConnectionID:     connID,
		SourcePath:       req.SourcePath,
		SourceDatabase:   req.SourceDatabase,
		IncludePatterns:  req.IncludePatterns,
		ExcludePatterns:  req.ExcludePatterns,
		Mode:             mode,
		Encrypted:        req.Encrypted,
		RetentionDays:    retDays,
		ExportFormat:     exportFormat,
		Enabled:          true,
		SLATargetMinutes: req.SLATargetMinutes,
		RPOTargetMinutes: req.RPOTargetMinutes,
		RTOTargetMinutes: req.RTOTargetMinutes,
	}

	if err := h.DB.GDB.Create(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create job failed"})
		return
	}

	if req.CronExpr != "" {
		tz := req.Timezone
		if tz == "" {
			tz = "UTC"
		}
		h.DB.GDB.Create(&vgbackup.VgBackupSchedule{
			BackupJobID: job.ID,
			CronExpr:    req.CronExpr,
			Timezone:    tz,
		})
	}

	h.DB.GDB.Preload("Schedule").Preload("Policy").Preload("Agent").Preload("StorageTarget").Preload("Connection").First(&job, job.ID)
	h.audit(c, "create", "vg_job", job.ID.String(), "ok", job.Name)
	h.publish("vgbackup.job", gin.H{"id": job.ID.String(), "action": "created"})
	c.JSON(http.StatusCreated, job)
}

func (h *VgMgmtHandler) UpdateBackupJob(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var job vgbackup.VgBackupJob
	if err := h.DB.GDB.Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, job.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	var req createJobRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	exportFormat, err := normalizeExportFormat(job.SourceType, req.ExportFormat)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Execution identity is immutable after creation: agent, storage
	// target, and source type are snapshotted into claims, ownership
	// checks, and run history. Clients send them back for display; a
	// differing value is rejected so a job can never silently change
	// owners — reassignment is delete-and-recreate.
	if req.AgentID != "" && req.AgentID != job.AgentID.String() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent cannot be changed after creation; recreate the job to move it"})
		return
	}
	if req.StorageTargetID != "" && req.StorageTargetID != job.StorageTargetID.String() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "storage target cannot be changed after creation; recreate the job to move it"})
		return
	}
	if strings.TrimSpace(string(req.SourceType)) != "" && req.SourceType != job.SourceType {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_type cannot be changed after creation; recreate the job"})
		return
	}

	mode, err := normalizeJobMode(req.Mode, job.Mode)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// 0 disables retention (matches policy semantics); negatives and
	// absurd values are rejected instead of silently disabling.
	if err := validateJobRetention(req.RetentionDays); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateJobSchedule(req.CronExpr, req.Timezone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for _, tv := range []struct {
		name string
		v    int
	}{
		{"sla_target_minutes", req.SLATargetMinutes},
		{"rpo_target_minutes", req.RPOTargetMinutes},
		{"rto_target_minutes", req.RTOTargetMinutes},
	} {
		if err := validateJobTarget(tv.name, tv.v); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Blanking a filesystem job's path (or a database job's database)
	// would dispatch unexecutable work, so blank values keep the stored
	// ones — clearing them is never a valid update.
	sourcePath := req.SourcePath
	if strings.TrimSpace(sourcePath) == "" {
		sourcePath = job.SourcePath
	}
	if (job.SourceType == vgbackup.VgSourceFilesystem || job.SourceType == vgbackup.VgSourceDBF) &&
		strings.TrimSpace(sourcePath) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_path is required for filesystem jobs"})
		return
	}
	sourceDatabase := req.SourceDatabase
	if strings.TrimSpace(sourceDatabase) == "" {
		sourceDatabase = job.SourceDatabase
	}

	updates := map[string]interface{}{
		"name":               req.Name,
		"source_path":        sourcePath,
		"source_database":    sourceDatabase,
		"include_patterns":   req.IncludePatterns,
		"exclude_patterns":   req.ExcludePatterns,
		"mode":               mode,
		"encrypted":          req.Encrypted,
		"retention_days":     req.RetentionDays,
		"export_format":      exportFormat,
		"sla_target_minutes": req.SLATargetMinutes,
		"rpo_target_minutes": req.RPOTargetMinutes,
		"rto_target_minutes": req.RTOTargetMinutes,
	}

	if strings.TrimSpace(req.ConnectionID) != "" {
		parsed, err := uuid.Parse(req.ConnectionID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid connection_id"})
			return
		}
		var conn vgbackup.VgDatabaseConnection
		if err := h.DB.GDB.Where("id = ?", parsed).First(&conn).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection not found"})
			return
		}
		if !vgbackup.CanAccessBackupResource(role, userID, conn.CreatedByUserID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection not found"})
			return
		}
		if isDatabaseSource(job.SourceType) && conn.AgentID != job.AgentID {
			c.JSON(http.StatusBadRequest, gin.H{"error": "connection belongs to a different agent; select the connection's agent"})
			return
		}
		updates["connection_id"] = parsed
	}

	// Effective database rule (mirrors create): a database job must end
	// up with either a connection or a database name.
	effConnID := job.ConnectionID
	if v, ok := updates["connection_id"]; ok {
		if pid, ok := v.(uuid.UUID); ok {
			effConnID = &pid
		}
	}
	if isDatabaseSource(job.SourceType) && effConnID == nil && strings.TrimSpace(sourceDatabase) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_database is required for database jobs without a connection"})
		return
	}

	if strings.TrimSpace(req.PolicyID) != "" {
		parsed, err := uuid.Parse(req.PolicyID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid policy_id"})
			return
		}
		var policy vgbackup.VgBackupPolicy
		if err := h.DB.GDB.Where("id = ?", parsed).First(&policy).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "policy not found"})
			return
		}
		if !vgbackup.CanAccessBackupResource(role, userID, policy.CreatedByUserID) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "policy not found"})
			return
		}
		updates["policy_id"] = parsed
	}

	if err := h.DB.GDB.Model(&job).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update job failed"})
		return
	}

	if req.CronExpr != "" {
		tz := req.Timezone
		if tz == "" {
			tz = "UTC"
		}
		var sched vgbackup.VgBackupSchedule
		h.DB.GDB.Where("backup_job_id = ?", job.ID).First(&sched)
		if sched.ID == uuid.Nil {
			h.DB.GDB.Create(&vgbackup.VgBackupSchedule{BackupJobID: job.ID, CronExpr: req.CronExpr, Timezone: tz})
		} else {
			h.DB.GDB.Model(&sched).Updates(map[string]interface{}{"cron_expr": req.CronExpr, "timezone": tz})
		}
	}

	h.DB.GDB.Preload("Schedule").Preload("Policy").Preload("Agent").Preload("StorageTarget").Preload("Connection").First(&job, job.ID)
	h.audit(c, "update", "vg_job", job.ID.String(), "ok", job.Name)
	h.publish("vgbackup.job", gin.H{"id": job.ID.String(), "action": "updated"})
	c.JSON(http.StatusOK, job)
}

func (h *VgMgmtHandler) DeleteBackupJob(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var job vgbackup.VgBackupJob
	if err := h.DB.GDB.Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, job.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	h.DB.GDB.Where("backup_job_id = ?", id).Delete(&vgbackup.VgBackupSchedule{})
	h.DB.GDB.Delete(&job)
	h.audit(c, "delete", "vg_job", job.ID.String(), "ok", job.Name)
	h.publish("vgbackup.job", gin.H{"id": job.ID.String(), "action": "deleted"})
	c.JSON(http.StatusNoContent, nil)
}

func (h *VgMgmtHandler) EnableBackupJob(c *gin.Context) {
	h.setJobEnabled(c, true)
}

func (h *VgMgmtHandler) DisableBackupJob(c *gin.Context) {
	h.setJobEnabled(c, false)
}

func (h *VgMgmtHandler) setJobEnabled(c *gin.Context, enabled bool) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var job vgbackup.VgBackupJob
	if err := h.DB.GDB.Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, job.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	h.DB.GDB.Model(&job).Update("enabled", enabled)
	action := "disabled"
	if enabled {
		action = "enabled"
	}
	h.audit(c, action, "vg_job", job.ID.String(), "ok", "")
	h.publish("vgbackup.job", gin.H{"id": job.ID.String(), "action": action})
	c.JSON(http.StatusOK, gin.H{"enabled": enabled})
}

func (h *VgMgmtHandler) RunNow(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var job vgbackup.VgBackupJob
	if err := h.DB.GDB.Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, job.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// In-flight guard
	var inflight int64
	h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Where("backup_job_id = ? AND status IN ?", job.ID,
			[]vgbackup.VgBackupRunStatus{
				vgbackup.VgRunPending, vgbackup.VgRunRunning,
				vgbackup.VgRunUploading, vgbackup.VgRunVerifying,
			}).
		Count(&inflight)
	if inflight > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "a run for this job is already in progress"})
		return
	}

	run := vgbackup.VgBackupRun{
		BackupJobID:     job.ID,
		AgentID:         job.AgentID,
		StorageTargetID: job.StorageTargetID,
		Status:          vgbackup.VgRunPending,
		SourceType:      string(job.SourceType),
	}
	if err := h.DB.GDB.Create(&run).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create run"})
		return
	}

	h.audit(c, "run-now", "vg_job", job.ID.String(), "ok", "run_id="+run.ID.String())
	h.publish("vgbackup.run", gin.H{"run_id": run.ID.String(), "status": string(vgbackup.VgRunPending)})
	c.JSON(http.StatusCreated, run)
}

type preflightCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // OK | WARN | FAIL
	Detail string `json:"detail,omitempty"`
}

func (h *VgMgmtHandler) Preflight(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var job vgbackup.VgBackupJob
	if err := h.DB.GDB.Preload("Agent").Preload("StorageTarget").Preload("Policy").
		Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, job.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	checks := []preflightCheck{
		h.checkAgentOnline(job),
		h.checkNoConflictingRun(job),
		h.checkStorageReachable(job),
		h.checkDiskSpace(job),
		h.checkEncryptionKey(job),
	}

	overall := "OK"
	for _, ch := range checks {
		if ch.Status == "FAIL" {
			overall = "FAIL"
			break
		}
		if ch.Status == "WARN" && overall != "FAIL" {
			overall = "WARN"
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"overall": overall,
		"checks":  checks,
	})
}

func (h *VgMgmtHandler) checkAgentOnline(job vgbackup.VgBackupJob) preflightCheck {
	if job.Agent.Lifecycle() == vgbackup.VgLifecycleOnline {
		return preflightCheck{Name: "Agent online", Status: "OK"}
	}
	return preflightCheck{
		Name:   "Agent online",
		Status: "FAIL",
		Detail: "Agent " + job.Agent.Name + " is not ONLINE — backup cannot be dispatched",
	}
}

func (h *VgMgmtHandler) checkNoConflictingRun(job vgbackup.VgBackupJob) preflightCheck {
	var inflight int64
	h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Where("backup_job_id = ? AND status IN ?", job.ID,
			[]vgbackup.VgBackupRunStatus{vgbackup.VgRunPending, vgbackup.VgRunRunning, vgbackup.VgRunUploading, vgbackup.VgRunVerifying}).
		Count(&inflight)
	if inflight == 0 {
		return preflightCheck{Name: "No conflicting run", Status: "OK"}
	}
	return preflightCheck{
		Name:   "No conflicting run",
		Status: "WARN",
		Detail: "A backup run for this job is already in progress",
	}
}

func (h *VgMgmtHandler) checkStorageReachable(job vgbackup.VgBackupJob) preflightCheck {
	if job.StorageTarget.Type == vgbackup.VgStorageLocal {
		if job.StorageTarget.Path == "" {
			return preflightCheck{Name: "Storage reachable", Status: "FAIL", Detail: "LOCAL storage target has no path configured"}
		}
		if _, err := os.Stat(job.StorageTarget.Path); err != nil {
			return preflightCheck{
				Name:   "Storage reachable",
				Status: "FAIL",
				Detail: "Storage path not accessible from server: " + err.Error(),
			}
		}
		return preflightCheck{Name: "Storage reachable", Status: "OK"}
	}
	return preflightCheck{
		Name:   "Storage reachable",
		Status: "WARN",
		Detail: "Remote storage (" + string(job.StorageTarget.Type) + ") reachability can only be verified at runtime",
	}
}

func (h *VgMgmtHandler) checkDiskSpace(job vgbackup.VgBackupJob) preflightCheck {
	if job.StorageTarget.Type != vgbackup.VgStorageLocal || job.StorageTarget.Path == "" {
		return preflightCheck{Name: "Disk space", Status: "WARN", Detail: "Cannot check disk space for remote storage targets"}
	}
	return preflightCheck{
		Name:   "Disk space",
		Status: "WARN",
		Detail: "Disk space is checked at runtime; ensure the storage path has sufficient free space",
	}
}

func (h *VgMgmtHandler) checkEncryptionKey(job vgbackup.VgBackupJob) preflightCheck {
	if !vgbackup.EffectivePolicy(&job).Encrypted {
		return preflightCheck{Name: "Encryption key", Status: "OK", Detail: "Encryption not enabled for this job"}
	}
	if len(h.EncKey) == 0 {
		return preflightCheck{Name: "Encryption key", Status: "FAIL", Detail: "SERVERHUB_ENCRYPTION_KEY is not configured or invalid"}
	}
	return preflightCheck{Name: "Encryption key", Status: "OK"}
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. Backup Runs & Artifacts
// ─────────────────────────────────────────────────────────────────────────────

func (h *VgMgmtHandler) ListBackupRuns(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 200 {
		limit = 50
	}
	offset := (page - 1) * limit

	query := h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Joins("JOIN vg_backup_jobs ON vg_backup_jobs.id = vg_backup_runs.backup_job_id")

	if role != vgbackup.BackupAdmin {
		query = query.Where("vg_backup_jobs.created_by_user_id = ?", userID)
	}

	if jobID := c.Query("job_id"); jobID != "" {
		query = query.Where("vg_backup_runs.backup_job_id = ?", jobID)
	}
	if status := c.Query("status"); status != "" {
		query = query.Where("vg_backup_runs.status = ?", status)
	}
	if q := strings.TrimSpace(c.Query("search")); q != "" {
		query = query.Where("vg_backup_jobs.name ILIKE ?", "%"+q+"%")
	}

	var total int64
	query.Count(&total)

	sortCol := map[string]string{
		"job":      "vg_backup_jobs.name",
		"started":  "vg_backup_runs.started_at",
		"duration": "vg_backup_runs.duration_seconds",
		"uploaded": "vg_backup_runs.bytes_uploaded",
		"created":  "vg_backup_runs.created_at",
	}[c.Query("sort")]
	if sortCol == "" {
		sortCol = "vg_backup_runs.created_at"
	}
	order := "DESC"
	if strings.EqualFold(c.Query("order"), "asc") {
		order = "ASC"
	}

	var runs []vgbackup.VgBackupRun
	query.Order(sortCol + " " + order).Order("vg_backup_runs.created_at DESC").
		Preload("BackupJob").Limit(limit).Offset(offset).Find(&runs)

	if runs == nil {
		runs = []vgbackup.VgBackupRun{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  runs,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

func (h *VgMgmtHandler) GetBackupRun(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Preload("BackupJob").Where("id = ?", id).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, run.BackupJob.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	c.JSON(http.StatusOK, run)
}

func (h *VgMgmtHandler) CancelBackupRun(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Preload("BackupJob").Where("id = ?", id).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, run.BackupJob.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if run.Status == vgbackup.VgRunCancelled {
		c.JSON(http.StatusOK, gin.H{"status": vgbackup.VgRunCancelled})
		return
	}
	if !vgbackup.CanTransition(run.Status, vgbackup.VgRunCancelled) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot cancel run in status: " + string(run.Status)})
		return
	}

	updates := map[string]interface{}{"cancel_requested": true}
	if run.Status == vgbackup.VgRunPending {
		updates["status"] = vgbackup.VgRunCancelled
	}
	h.DB.GDB.Model(&run).Updates(updates)

	h.audit(c, "cancel", "vg_run", run.ID.String(), "ok", "")
	h.publish("vgbackup.run", gin.H{"run_id": run.ID.String(), "status": string(vgbackup.VgRunCancelled)})
	c.JSON(http.StatusOK, gin.H{"status": vgbackup.VgRunCancelled})
}

func (h *VgMgmtHandler) ListArtifacts(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Preload("BackupJob").Where("id = ?", runID).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, run.BackupJob.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	var artifacts []vgbackup.VgBackupArtifact
	h.DB.GDB.Where("backup_run_id = ?", runID).Find(&artifacts)
	if artifacts == nil {
		artifacts = []vgbackup.VgBackupArtifact{}
	}
	c.JSON(http.StatusOK, artifacts)
}

func (h *VgMgmtHandler) VerifyRun(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Preload("BackupJob").Where("id = ?", runID).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, run.BackupJob.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	if run.Status != vgbackup.VgRunCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only COMPLETED runs can be verified"})
		return
	}
	if run.Checksum == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "run has no recorded checksum"})
		return
	}

	var target vgbackup.VgStorageTarget
	if err := h.DB.GDB.Where("id = ?", run.StorageTargetID).First(&target).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage target not found"})
		return
	}
	if target.Type != vgbackup.VgStorageLocal {
		c.JSON(http.StatusBadRequest, gin.H{"error": "verification is only supported for LOCAL storage targets"})
		return
	}

	sum, err := sha256File(run.StoragePath)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "could not read stored file: " + err.Error()})
		return
	}

	now := time.Now()
	if sum != strings.ToLower(run.Checksum) {
		h.DB.GDB.Model(&vgbackup.VgBackupArtifact{}).Where("backup_run_id = ?", run.ID).
			Updates(map[string]interface{}{"verification_status": vgbackup.VgArtifactVerifyFail, "verified_at": &now})
		c.JSON(http.StatusOK, gin.H{"match": false, "expected": run.Checksum, "actual": sum})
		return
	}

	h.DB.GDB.Model(&vgbackup.VgBackupArtifact{}).Where("backup_run_id = ?", run.ID).
		Updates(map[string]interface{}{"verification_status": vgbackup.VgArtifactVerified, "verified_at": &now})
	c.JSON(http.StatusOK, gin.H{"match": true, "checksum": sum})
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ─────────────────────────────────────────────────────────────────────────────
// 6. Restores
// ─────────────────────────────────────────────────────────────────────────────

type createRestoreRequest struct {
	BackupRunID     string `json:"backup_run_id" binding:"required"`
	AgentID         string `json:"agent_id" binding:"required"`
	DestinationPath string `json:"destination_path"`
	TargetDatabase  string `json:"target_database"`
	Confirmed       bool   `json:"confirmed" binding:"required"`
}

func (h *VgMgmtHandler) ListRestores(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var jobs []vgbackup.VgRestoreJob
	query := h.DB.GDB.Preload("BackupRun.BackupJob").Preload("Connection").Order("created_at DESC")
	if role != vgbackup.BackupAdmin {
		query = query.Joins("JOIN vg_backup_runs ON vg_backup_runs.id = vg_restore_jobs.backup_run_id").
			Joins("JOIN vg_backup_jobs ON vg_backup_jobs.id = vg_backup_runs.backup_job_id").
			Where("vg_backup_jobs.created_by_user_id = ?", userID)
	}
	if err := query.Find(&jobs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list restores failed"})
		return
	}
	if jobs == nil {
		jobs = []vgbackup.VgRestoreJob{}
	}
	c.JSON(http.StatusOK, jobs)
}

func (h *VgMgmtHandler) GetRestore(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var job vgbackup.VgRestoreJob
	if err := h.DB.GDB.Preload("BackupRun.BackupJob").Preload("Connection").
		Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "restore job not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, job.BackupRun.BackupJob.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "restore job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}

func (h *VgMgmtHandler) CreateRestore(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var req createRestoreRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !req.Confirmed {
		c.JSON(http.StatusBadRequest, gin.H{"error": "confirmed must be true to proceed with restore"})
		return
	}

	runID, err := uuid.Parse(req.BackupRunID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid backup_run_id"})
		return
	}
	agentID, err := uuid.Parse(req.AgentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent_id"})
		return
	}

	var agent vgbackup.VgAgent
	if err := h.DB.GDB.Where("id = ?", agentID).First(&agent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, agent.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Preload("BackupJob").Where("id = ?", runID).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup run not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, run.BackupJob.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup run not found"})
		return
	}
	if run.Status != vgbackup.VgRunCompleted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "only COMPLETED runs can be restored (status: " + string(run.Status) + ")"})
		return
	}

	var connID *uuid.UUID
	if run.BackupJob.ConnectionID != nil {
		var conn vgbackup.VgDatabaseConnection
		if err := h.DB.GDB.Where("id = ?", *run.BackupJob.ConnectionID).First(&conn).Error; err == nil {
			if conn.AgentID != agentID {
				c.JSON(http.StatusBadRequest, gin.H{"error": "restore must run on the connection's agent"})
				return
			}
			connID = run.BackupJob.ConnectionID
		}
	}

	job := vgbackup.VgRestoreJob{
		BackupRunID:     runID,
		AgentID:         agentID,
		ConnectionID:    connID,
		Status:          vgbackup.VgRestorePending,
		DestinationPath: req.DestinationPath,
		TargetDatabase:  req.TargetDatabase,
	}

	var live vgbackup.VgRestoreJob
	if err := h.DB.GDB.Where("backup_run_id = ? AND agent_id = ? AND destination_path = ? AND target_database = ? AND status = ?",
		runID, agentID, req.DestinationPath, req.TargetDatabase, vgbackup.VgRestorePending).First(&live).Error; err == nil {
		c.JSON(http.StatusOK, live)
		return
	}

	if err := h.DB.GDB.Create(&job).Error; err != nil {
		// Lost race with a concurrent duplicate: return the live winner
		// instead of a constraint error (partial unique on pending restores).
		var winner vgbackup.VgRestoreJob
		if herr := h.DB.GDB.Where("backup_run_id = ? AND agent_id = ? AND destination_path = ? AND target_database = ? AND status = ?",
			runID, agentID, req.DestinationPath, req.TargetDatabase, vgbackup.VgRestorePending).First(&winner).Error; herr == nil {
			c.JSON(http.StatusOK, winner)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create restore"})
		return
	}

	h.audit(c, "create-restore", "vg_restore", job.ID.String(), "ok", "run="+runID.String())
	h.publish("vgbackup.restore", gin.H{"restore_id": job.ID.String(), "status": string(vgbackup.VgRestorePending)})
	c.JSON(http.StatusCreated, job)
}

// ─────────────────────────────────────────────────────────────────────────────
// 7. Agents & Enrollment Tokens
// ─────────────────────────────────────────────────────────────────────────────

type generateEnrollmentRequest struct {
	Name           string `json:"name"`
	ExpiresMinutes int    `json:"expires_minutes"`
}

func (h *VgMgmtHandler) GenerateEnrollmentToken(c *gin.Context) {
	_, _, userID, ok := h.actor(c)
	if !ok {
		return
	}
	// All fields are optional (name auto-generated, TTL defaults to 30):
	// an empty body binds as EOF, which means "defaults", not "malformed".
	var req generateEnrollmentRequest
	if err := c.ShouldBindJSON(&req); err != nil && err.Error() != "EOF" {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ttl := req.ExpiresMinutes
	if ttl == 0 {
		ttl = 30
	}
	if ttl < 1 || ttl > 1440 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expires_minutes must be 1-1440"})
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "pending-" + vgRandomHex(4)
	}
	token := vgRandomHex(32)
	expiresAt := time.Now().Add(time.Duration(ttl) * time.Minute)

	var agentID uuid.UUID
	err := h.DB.GDB.Transaction(func(tx *gorm.DB) error {
		agent := vgbackup.VgAgent{
			CreatedByUserID: userID,
			Name:            name,
			Status:          vgbackup.VgAgentOffline,
		}
		if err := tx.Create(&agent).Error; err != nil {
			return err
		}
		tok := vgbackup.VgEnrollmentToken{
			CreatedByUserID: userID,
			AgentID:         &agent.ID,
			TokenHash:       vgSha256Hex(token),
			ExpiresAt:       expiresAt,
		}
		if err := tx.Create(&tok).Error; err != nil {
			return err
		}
		agentID = agent.ID
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create enrollment token"})
		return
	}

	h.audit(c, "create-enrollment-token", "vg_agent", agentID.String(), "ok", name)
	h.publish("vgbackup.agent", gin.H{"agent_id": agentID.String(), "event": "token-created"})
	c.JSON(http.StatusCreated, gin.H{
		"agent_id":         agentID,
		"enrollment_token": token,
		"expires_at":       expiresAt,
	})
}

func (h *VgMgmtHandler) ListEnrollmentTokens(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var toks []vgbackup.VgEnrollmentToken
	query := h.DB.GDB.Order("created_at DESC")
	if role != vgbackup.BackupAdmin {
		query = query.Where("created_by_user_id = ?", userID)
	}
	if err := query.Find(&toks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list tokens failed"})
		return
	}
	out := make([]gin.H, 0, len(toks))
	for _, t := range toks {
		var aID string
		if t.AgentID != nil {
			aID = t.AgentID.String()
		}
		out = append(out, gin.H{
			"id":         t.ID.String(),
			"agent_id":   aID,
			"expires_at": t.ExpiresAt,
			"used_at":    t.UsedAt,
			"revoked_at": t.RevokedAt,
			"created_at": t.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

func (h *VgMgmtHandler) RevokeEnrollmentToken(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var tok vgbackup.VgEnrollmentToken
	if err := h.DB.GDB.Where("id = ?", id).First(&tok).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, tok.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}

	now := time.Now()
	res := h.DB.GDB.Model(&vgbackup.VgEnrollmentToken{}).
		Where("id = ? AND used_at IS NULL AND revoked_at IS NULL", id).
		Update("revoked_at", &now)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "revoke failed"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "token not found"})
		return
	}

	h.audit(c, "revoke-enrollment-token", "vg_enrollment_token", id.String(), "ok", "")
	h.publish("vgbackup.agent", gin.H{"token_id": id.String(), "event": "token-revoked"})
	c.JSON(http.StatusNoContent, nil)
}

func agentWithLifecycle(a vgbackup.VgAgent) gin.H {
	return gin.H{
		"id":           a.ID.String(),
		"name":         a.Name,
		"platform":     a.Platform,
		"architecture": a.Architecture,
		"machine_name": a.MachineName,
		"status":       string(a.Status),
		"lifecycle":    a.Lifecycle(),
		"version":      a.Version,
		"last_seen_at": a.LastSeenAt,
		"installed_at": a.InstalledAt,
		"revoked_at":   a.RevokedAt,
		"created_at":   a.CreatedAt,
	}
}

func (h *VgMgmtHandler) ListAgents(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var agents []vgbackup.VgAgent
	query := h.DB.GDB.Order("created_at ASC")
	if role != vgbackup.BackupAdmin {
		query = query.Where("created_by_user_id = ?", userID)
	}
	if err := query.Find(&agents).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list agents failed"})
		return
	}
	out := make([]gin.H, 0, len(agents))
	for _, a := range agents {
		out = append(out, agentWithLifecycle(a))
	}
	c.JSON(http.StatusOK, out)
}

func (h *VgMgmtHandler) GetAgent(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var agent vgbackup.VgAgent
	if err := h.DB.GDB.Where("id = ?", id).First(&agent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, agent.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	c.JSON(http.StatusOK, agentWithLifecycle(agent))
}

func (h *VgMgmtHandler) RevokeAgent(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var agent vgbackup.VgAgent
	if err := h.DB.GDB.Where("id = ?", id).First(&agent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, agent.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	now := time.Now()
	h.DB.GDB.Model(&agent).Updates(map[string]interface{}{
		"revoked_at": &now,
		"status":     vgbackup.VgAgentOffline,
	})

	h.audit(c, "revoke-agent", "vg_agent", agent.ID.String(), "ok", agent.Name)
	h.publish("vgbackup.agent", gin.H{"agent_id": agent.ID.String(), "event": "revoked"})
	c.JSON(http.StatusOK, gin.H{"revoked": true})
}

// ─────────────────────────────────────────────────────────────────────────────
// 8. Dashboard Summary
// ─────────────────────────────────────────────────────────────────────────────

func (h *VgMgmtHandler) Dashboard(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}

	jobsQuery := h.DB.GDB.Model(&vgbackup.VgBackupJob{})
	runsQuery := h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Joins("JOIN vg_backup_jobs ON vg_backup_jobs.id = vg_backup_runs.backup_job_id")

	if role != vgbackup.BackupAdmin {
		jobsQuery = jobsQuery.Where("created_by_user_id = ?", userID)
		runsQuery = runsQuery.Where("vg_backup_jobs.created_by_user_id = ?", userID)
	}

	var totalJobs, enabledJobs int64
	jobsQuery.Count(&totalJobs)
	h.DB.GDB.Model(&vgbackup.VgBackupJob{}).Where("enabled = true").Scopes(func(d *gorm.DB) *gorm.DB {
		if role != vgbackup.BackupAdmin {
			return d.Where("created_by_user_id = ?", userID)
		}
		return d
	}).Count(&enabledJobs)

	// Presence derives from heartbeat recency (the same window as
	// Lifecycle), never from the stored status flag: nothing resets the
	// flag to OFFLINE when an agent goes quiet, so flag counts would
	// report long-dead agents as online. Revoked agents always count as
	// offline.
	var agentsOnline, totalAgents int64
	onlineCutoff := time.Now().Add(-vgbackup.VgOnlineThreshold)
	onlineQuery := h.DB.GDB.Model(&vgbackup.VgAgent{}).
		Where("revoked_at IS NULL AND last_seen_at IS NOT NULL AND last_seen_at > ?", onlineCutoff)
	totalQuery := h.DB.GDB.Model(&vgbackup.VgAgent{})
	if role != vgbackup.BackupAdmin {
		onlineQuery = onlineQuery.Where("created_by_user_id = ?", userID)
		totalQuery = totalQuery.Where("created_by_user_id = ?", userID)
	}
	onlineQuery.Count(&agentsOnline)
	totalQuery.Count(&totalAgents)
	agentsOffline := totalAgents - agentsOnline

	since := time.Now().Add(-24 * time.Hour)
	var successfulRuns, failedRuns int64
	runsQuery.Where("vg_backup_runs.status = ? AND vg_backup_runs.created_at > ?", vgbackup.VgRunCompleted, since).Count(&successfulRuns)
	h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Joins("JOIN vg_backup_jobs ON vg_backup_jobs.id = vg_backup_runs.backup_job_id").
		Where("vg_backup_runs.status = ? AND vg_backup_runs.created_at > ?", vgbackup.VgRunFailed, since).
		Scopes(func(d *gorm.DB) *gorm.DB {
			if role != vgbackup.BackupAdmin {
				return d.Where("vg_backup_jobs.created_by_user_id = ?", userID)
			}
			return d
		}).Count(&failedRuns)

	var totalBytes int64
	h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Joins("JOIN vg_backup_jobs ON vg_backup_jobs.id = vg_backup_runs.backup_job_id").
		Where("vg_backup_runs.status = ?", vgbackup.VgRunCompleted).
		Scopes(func(d *gorm.DB) *gorm.DB {
			if role != vgbackup.BackupAdmin {
				return d.Where("vg_backup_jobs.created_by_user_id = ?", userID)
			}
			return d
		}).
		Select("COALESCE(SUM(vg_backup_runs.bytes_uploaded), 0)").Scan(&totalBytes)

	var recentRuns []vgbackup.VgBackupRun
	h.DB.GDB.Preload("BackupJob").
		Joins("JOIN vg_backup_jobs ON vg_backup_jobs.id = vg_backup_runs.backup_job_id").
		Scopes(func(d *gorm.DB) *gorm.DB {
			if role != vgbackup.BackupAdmin {
				return d.Where("vg_backup_jobs.created_by_user_id = ?", userID)
			}
			return d
		}).
		Order("vg_backup_runs.created_at DESC").
		Limit(10).
		Find(&recentRuns)

	if recentRuns == nil {
		recentRuns = []vgbackup.VgBackupRun{}
	}

	// SLA/RPO targets are enforced here as read-only breach signals: for
	// each visible job whose resolved policy sets a target, the latest
	// COMPLETED run is compared against it. RPO measures freshness (no
	// successful backup within the window, or none ever); SLA measures
	// the last successful run's duration. RTO has no equivalent signal:
	// restore drills are rare operator-initiated events, so a stale
	// last-restore duration would false-alarm — RTO stays informational
	// (configured value only) until restore drills exist.
	slaBreaches := make([]gin.H, 0)
	rpoBreaches := make([]gin.H, 0)
	var breachJobs []vgbackup.VgBackupJob
	breachQuery := h.DB.GDB.Preload("Policy")
	if role != vgbackup.BackupAdmin {
		breachQuery = breachQuery.Where("created_by_user_id = ?", userID)
	}
	if err := breachQuery.Find(&breachJobs).Error; err == nil {
		for i := range breachJobs {
			job := &breachJobs[i]
			eff := vgbackup.EffectivePolicy(job)
			if eff.SLATargetMinutes <= 0 && eff.RPOTargetMinutes <= 0 {
				continue
			}
			var latest vgbackup.VgBackupRun
			if err := h.DB.GDB.Where("backup_job_id = ? AND status = ?", job.ID, vgbackup.VgRunCompleted).
				Order("completed_at DESC").First(&latest).Error; err != nil {
				latest = vgbackup.VgBackupRun{}
			}
			if eff.RPOTargetMinutes > 0 {
				if latest.ID == uuid.Nil {
					rpoBreaches = append(rpoBreaches, gin.H{
						"job_id": job.ID.String(), "job_name": job.Name,
						"rpo_target_minutes": eff.RPOTargetMinutes, "minutes_since_success": nil,
					})
				} else if latest.CompletedAt != nil &&
					time.Since(*latest.CompletedAt) > time.Duration(eff.RPOTargetMinutes)*time.Minute {
					rpoBreaches = append(rpoBreaches, gin.H{
						"job_id": job.ID.String(), "job_name": job.Name,
						"rpo_target_minutes":    eff.RPOTargetMinutes,
						"minutes_since_success": int64(time.Since(*latest.CompletedAt).Minutes()),
					})
				}
			}
			if eff.SLATargetMinutes > 0 && latest.ID != uuid.Nil &&
				latest.DurationSeconds > int64(eff.SLATargetMinutes)*60 {
				slaBreaches = append(slaBreaches, gin.H{
					"job_id": job.ID.String(), "job_name": job.Name,
					"sla_target_minutes": eff.SLATargetMinutes, "duration_seconds": latest.DurationSeconds,
				})
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_jobs":      totalJobs,
		"enabled_jobs":    enabledJobs,
		"agents_online":   agentsOnline,
		"agents_offline":  agentsOffline,
		"successful_runs": successfulRuns,
		"failed_runs":     failedRuns,
		"total_bytes":     totalBytes,
		"recent_runs":     recentRuns,
		"sla_breaches":    slaBreaches,
		"rpo_breaches":    rpoBreaches,
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// 9. Machines (agent-reported inventory, read-only)
// ─────────────────────────────────────────────────────────────────────────────

func (h *VgMgmtHandler) ListMachines(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	var machines []vgbackup.VgMachine
	query := h.DB.GDB.Preload("Agent").
		Joins("JOIN vg_agents ON vg_agents.id = vg_machines.agent_id")
	if role != vgbackup.BackupAdmin {
		query = query.Where("vg_agents.created_by_user_id = ?", userID)
	}
	if err := query.Find(&machines).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "list machines failed"})
		return
	}
	c.JSON(http.StatusOK, machines)
}

func (h *VgMgmtHandler) GetMachine(c *gin.Context) {
	_, role, userID, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var machine vgbackup.VgMachine
	if err := h.DB.GDB.Preload("Agent").Where("vg_machines.id = ?", id).First(&machine).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "machine not found"})
		return
	}
	if !vgbackup.CanAccessBackupResource(role, userID, machine.Agent.CreatedByUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "machine not found"})
		return
	}
	c.JSON(http.StatusOK, machine)
}

// ─────────────────────────────────────────────────────────────────────────────
// 10. Agent credential operations (admin only)
// ─────────────────────────────────────────────────────────────────────────────

// GenerateRegistrationToken creates a pending agent row plus a one-time
// registration key for the legacy register flow. The plaintext key is
// returned once; only its SHA-256 digest is stored. The key is single-use
// (registration clears it) and expires 24h after minting. Prefer
// enrollment tokens (which carry configurable expiry) for new setups.
func (h *VgMgmtHandler) GenerateRegistrationToken(c *gin.Context) {
	_, _, userID, ok := h.actor(c)
	if !ok {
		return
	}
	token := vgRandomHex(32)
	keyHash := vgSha256Hex(token)
	expiresAt := time.Now().Add(24 * time.Hour)
	agent := vgbackup.VgAgent{
		CreatedByUserID:       userID,
		Name:                  "pending-" + vgRandomHex(4),
		RegistrationKey:       &keyHash,
		RegistrationExpiresAt: &expiresAt,
		Status:                vgbackup.VgAgentOffline,
	}
	if err := h.DB.GDB.Create(&agent).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create agent"})
		return
	}
	h.audit(c, "create-registration-token", "vg_agent", agent.ID.String(), "ok", agent.Name)
	h.publish("vgbackup.agent", gin.H{"agent_id": agent.ID.String(), "event": "token-created"})
	c.JSON(http.StatusCreated, gin.H{
		"agent_id":         agent.ID,
		"registration_key": token,
	})
}

// RotateAgentToken issues a new agent token, immediately invalidating the
// old one. The agent row is locked (SELECT FOR UPDATE) so concurrent
// rotations serialize instead of issuing two live credentials.
func (h *VgMgmtHandler) RotateAgentToken(c *gin.Context) {
	_, _, _, ok := h.actor(c)
	if !ok {
		return
	}
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	newToken, tokenHash, err := vgMintAgentToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}

	var agent vgbackup.VgAgent
	txErr := h.DB.GDB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).First(&agent).Error; err != nil {
			return err
		}
		if agent.RevokedAt != nil {
			return errVgAgentRevokedOp
		}
		now := time.Now()
		return tx.Model(&agent).Updates(map[string]interface{}{
			"token_hash":       tokenHash,
			"token_rotated_at": &now,
		}).Error
	})
	if txErr != nil {
		// Existence and revocation both answer 404: a caller probing IDs
		// learns nothing beyond what listing already shows them.
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	h.audit(c, "rotate-agent-token", "vg_agent", agent.ID.String(), "ok", agent.Name)
	h.publish("vgbackup.agent", gin.H{"agent_id": agent.ID.String(), "event": "token-rotated"})
	c.JSON(http.StatusOK, gin.H{
		"agent_id":    agent.ID,
		"agent_token": newToken, // shown once, never stored
	})
}

var errVgAgentRevokedOp = errors.New("agent revoked")
