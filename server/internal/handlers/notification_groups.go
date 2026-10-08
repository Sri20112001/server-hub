package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
	"serverhub/internal/notify"
)

// NotificationGroupsHandler manages email recipient groups (Phase 1).
// RBAC mirrors server groups: viewer reads, operator mutates, admin deletes.
type NotificationGroupsHandler struct {
	DB *database.DB
}

func (h *NotificationGroupsHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`
		SELECT g.id, g.name, g.description, g.created_at, g.updated_at,
		       COUNT(m.id) AS members
		FROM notification_groups g
		LEFT JOIN notification_group_members m ON m.group_id = g.id
		GROUP BY g.id ORDER BY g.name`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uint
		var name, desc string
		var createdAt, updatedAt time.Time
		var members int
		if err := rows.Scan(&id, &name, &desc, &createdAt, &updatedAt, &members); err == nil {
			out = append(out, gin.H{
				"id": id, "name": name, "description": desc,
				"memberCount": members, "createdAt": createdAt, "updatedAt": updatedAt,
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *NotificationGroupsHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var gid uint
	var name, desc string
	var createdAt, updatedAt time.Time
	if err := h.DB.QueryRow(`
		SELECT id, name, description, created_at, updated_at
		FROM notification_groups WHERE id=$1`, id).Scan(&gid, &name, &desc, &createdAt, &updatedAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": gid, "name": name, "description": desc,
		"members": h.listMembers(id),
		"createdAt": createdAt, "updatedAt": updatedAt,
	})
}

func (h *NotificationGroupsHandler) listMembers(groupID int64) []gin.H {
	rows, err := h.DB.Query(`
		SELECT id, email, created_at FROM notification_group_members
		WHERE group_id=$1 ORDER BY id`, groupID)
	if err != nil {
		return []gin.H{}
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var mid uint
		var email string
		var createdAt time.Time
		if err := rows.Scan(&mid, &email, &createdAt); err == nil {
			out = append(out, gin.H{"id": mid, "email": email, "createdAt": createdAt})
		}
	}
	return out
}

func (h *NotificationGroupsHandler) Create(c *gin.Context) {
	var body struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if len(body.Name) > 100 || len(body.Description) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name (max 100) or description (max 500) too long"})
		return
	}
	id, err := h.DB.InsertID(`
		INSERT INTO notification_groups (name,description,created_at,updated_at)
		VALUES ($1,$2,NOW(),NOW())`,
		strings.TrimSpace(body.Name), strings.TrimSpace(body.Description))
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "group name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create", "notification_group", strconv.FormatInt(id, 10), "ok", body.Name)
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": strings.TrimSpace(body.Name)})
}

func (h *NotificationGroupsHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Name        *string `json:"name"`
		Description *string `json:"description"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if body.Name != nil && strings.TrimSpace(*body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name must not be empty"})
		return
	}
	if (body.Name != nil && len(*body.Name) > 100) || (body.Description != nil && len(*body.Description) > 500) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name (max 100) or description (max 500) too long"})
		return
	}
	res, err := h.DB.Exec(`
		UPDATE notification_groups SET
		  name=COALESCE($1,name),
		  description=COALESCE($2,description),
		  updated_at=NOW()
		WHERE id=$3`,
		nullableStr(body.Name), nullableStr(body.Description), id)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "group name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "update", "notification_group", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func nullableStr(s *string) any {
	if s == nil {
		return nil
	}
	return strings.TrimSpace(*s)
}

func (h *NotificationGroupsHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	// A group referenced by notification rules cannot be deleted silently:
	// disable or delete those rules first (no fallback redirection).
	var refs int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM notification_rules WHERE notification_group_id=$1`, id).Scan(&refs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if refs > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "group is referenced by notification rules"})
		return
	}
	// Explicit member cleanup first (works even without the FK cascade).
	_, _ = h.DB.Exec(`DELETE FROM notification_group_members WHERE group_id=$1`, id)
	res, err := h.DB.Exec(`DELETE FROM notification_groups WHERE id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	// A deleted default group must not leave a dangling pointer: fall back
	// to the legacy To address.
	_, _ = h.DB.Exec(`DELETE FROM app_settings WHERE key=$1`, notify.KeyEmailGroupID)
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "notification_group", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *NotificationGroupsHandler) AddMember(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var exists int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM notification_groups WHERE id=$1`, id).Scan(&exists); err != nil || exists == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	email := notify.NormalizeMemberEmail(body.Email)
	if email == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "valid email is required"})
		return
	}
	if len(email) > 254 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email too long (max 254)"})
		return
	}
	var count int
	if err := h.DB.QueryRow(`SELECT COUNT(*) FROM notification_group_members WHERE group_id=$1`, id).Scan(&count); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if count >= notify.MaxGroupMembers {
		c.JSON(http.StatusBadRequest, gin.H{"error": "group is full (max 10 members)"})
		return
	}
	mid, err := h.DB.InsertID(`
		INSERT INTO notification_group_members (group_id,email,created_at)
		VALUES ($1,$2,NOW())`, id, email)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "email already in group"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "add-member", "notification_group", strconv.FormatInt(id, 10), "ok", email)
	c.JSON(http.StatusCreated, gin.H{"id": mid, "email": email})
}

func (h *NotificationGroupsHandler) RemoveMember(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	mid, err := strconv.ParseInt(c.Param("memberId"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid member id"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM notification_group_members WHERE id=$1 AND group_id=$2`, mid, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "member not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "remove-member", "notification_group", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
