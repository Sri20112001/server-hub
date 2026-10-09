package handlers

import (
	"fmt"
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
		SELECT g.id, g.name, g.description, g.quiet_start, g.quiet_end,
		       g.quiet_tz, g.quiet_allow_critical, g.created_at, g.updated_at,
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
		var name, desc, qs, qe, qtz string
		var qallow bool
		var createdAt, updatedAt time.Time
		var members int
		if err := rows.Scan(&id, &name, &desc, &qs, &qe, &qtz, &qallow,
			&createdAt, &updatedAt, &members); err == nil {
			out = append(out, gin.H{
				"id": id, "name": name, "description": desc,
				"quietStart": qs, "quietEnd": qe, "quietTZ": qtz,
				"quietAllowCritical": qallow,
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
	var name, desc, qs, qe, qtz string
	var qallow bool
	var createdAt, updatedAt time.Time
	if err := h.DB.QueryRow(`
		SELECT id, name, description, quiet_start, quiet_end, quiet_tz,
		       quiet_allow_critical, created_at, updated_at
		FROM notification_groups WHERE id=$1`, id).Scan(
		&gid, &name, &desc, &qs, &qe, &qtz, &qallow, &createdAt, &updatedAt); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "group not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": gid, "name": name, "description": desc,
		"quietStart": qs, "quietEnd": qe, "quietTZ": qtz,
		"quietAllowCritical": qallow,
		"members": h.listMembers(id),
		"createdAt": createdAt, "updatedAt": updatedAt,
	})
}

// validateQuiet checks HH:MM pairs and a loadable IANA timezone.
// Both empty disables quiet hours.
func validateQuiet(start, end, tz string) error {
	if strings.TrimSpace(start) == "" && strings.TrimSpace(end) == "" {
		return nil
	}
	for _, v := range []string{start, end} {
		parts := strings.Split(strings.TrimSpace(v), ":")
		if len(parts) != 2 {
			return fmt.Errorf("quiet hours must be HH:MM")
		}
		h, err1 := strconv.Atoi(parts[0])
		m, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
			return fmt.Errorf("quiet hours must be HH:MM")
		}
	}
	if strings.TrimSpace(tz) == "" {
		return fmt.Errorf("quiet timezone is required with quiet hours")
	}
	if _, err := time.LoadLocation(strings.TrimSpace(tz)); err != nil {
		return fmt.Errorf("unknown timezone %q", tz)
	}
	return nil
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
		Name               string `json:"name"`
		Description        string `json:"description"`
		QuietStart         string `json:"quietStart"`
		QuietEnd           string `json:"quietEnd"`
		QuietTZ            string `json:"quietTZ"`
		QuietAllowCritical *bool  `json:"quietAllowCritical"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || strings.TrimSpace(body.Name) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if len(body.Name) > 100 || len(body.Description) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name (max 100) or description (max 500) too long"})
		return
	}
	qtz := strings.TrimSpace(body.QuietTZ)
	if qtz == "" {
		qtz = "UTC"
	}
	if err := validateQuiet(body.QuietStart, body.QuietEnd, qtz); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	qallow := true
	if body.QuietAllowCritical != nil {
		qallow = *body.QuietAllowCritical
	}
	id, err := h.DB.InsertID(`
		INSERT INTO notification_groups
		  (name,description,quiet_start,quiet_end,quiet_tz,quiet_allow_critical,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,NOW(),NOW())`,
		strings.TrimSpace(body.Name), strings.TrimSpace(body.Description),
		strings.TrimSpace(body.QuietStart), strings.TrimSpace(body.QuietEnd), qtz, qallow)
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
		Name               *string `json:"name"`
		Description        *string `json:"description"`
		QuietStart         *string `json:"quietStart"`
		QuietEnd           *string `json:"quietEnd"`
		QuietTZ            *string `json:"quietTZ"`
		QuietAllowCritical *bool   `json:"quietAllowCritical"`
		ClearQuiet         *bool   `json:"clearQuiet"`
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
	// Merge quiet-hours patch over the stored row for validation.
	var curQS, curQE, curTZ string
	_ = h.DB.QueryRow(`SELECT quiet_start, quiet_end, quiet_tz FROM notification_groups WHERE id=$1`, id).
		Scan(&curQS, &curQE, &curTZ)
	qs, qe, qtz := curQS, curQE, curTZ
	if body.QuietStart != nil {
		qs = strings.TrimSpace(*body.QuietStart)
	}
	if body.QuietEnd != nil {
		qe = strings.TrimSpace(*body.QuietEnd)
	}
	if body.QuietTZ != nil && strings.TrimSpace(*body.QuietTZ) != "" {
		qtz = strings.TrimSpace(*body.QuietTZ)
	}
	if body.ClearQuiet != nil && *body.ClearQuiet {
		qs, qe = "", ""
	}
	if err := validateQuiet(qs, qe, qtz); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var qallow any
	if body.QuietAllowCritical != nil {
		qallow = *body.QuietAllowCritical
	}
	res, err := h.DB.Exec(`
		UPDATE notification_groups SET
		  name=COALESCE($1,name),
		  description=COALESCE($2,description),
		  quiet_start=$3, quiet_end=$4, quiet_tz=$5,
		  quiet_allow_critical=COALESCE($6,quiet_allow_critical),
		  updated_at=NOW()
		WHERE id=$7`,
		nullableStr(body.Name), nullableStr(body.Description), qs, qe, qtz, qallow, id)
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
