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
	"serverhub/internal/rules"
)

// NotificationRulesHandler manages Phase 2 notification rules.
// RBAC mirrors notification groups: viewer reads, operator mutates,
// admin deletes.
type NotificationRulesHandler struct {
	DB *database.DB
}

func (h *NotificationRulesHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`
		SELECT r.id, r.name, r.description, r.enabled, r.event_type, r.severity,
		       r.condition_json, r.notification_group_id, r.channels,
		       r.cooldown_seconds, r.notify_on_recovery,
		       r.created_by, r.updated_by, r.created_at, r.updated_at,
		       COALESCE(g.name, '') AS group_name
		FROM notification_rules r
		LEFT JOIN notification_groups g ON g.id = r.notification_group_id
		ORDER BY r.id`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id, gid uint
		var name, desc, eventType, sev, cond, ch, gname string
		var enabled, rec bool
		var cd int
		var by, uby string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &name, &desc, &enabled, &eventType, &sev, &cond,
			&gid, &ch, &cd, &rec, &by, &uby, &createdAt, &updatedAt, &gname); err != nil {
			continue
		}
		out = append(out, gin.H{
			"id": id, "name": name, "description": desc, "enabled": enabled,
			"eventType": eventType, "severity": sev, "conditionJson": cond,
			"notificationGroupId": gid, "groupName": gname,
			"channels": ch, "cooldownSeconds": cd, "notifyOnRecovery": rec,
			"createdBy": by, "updatedBy": uby,
			"createdAt": createdAt, "updatedAt": updatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

func (h *NotificationRulesHandler) Get(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	rows, err := h.DB.Query(`
		SELECT r.id, r.name, r.description, r.enabled, r.event_type, r.severity,
		       r.condition_json, r.notification_group_id, r.channels,
		       r.cooldown_seconds, r.notify_on_recovery,
		       r.created_by, r.updated_by, r.created_at, r.updated_at,
		       COALESCE(g.name, '') AS group_name
		FROM notification_rules r
		LEFT JOIN notification_groups g ON g.id = r.notification_group_id
		WHERE r.id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	if !rows.Next() {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	var rid, gid uint
	var name, desc, eventType, sev, cond, ch, gname string
	var enabled, rec bool
	var cd int
	var by, uby string
	var createdAt, updatedAt time.Time
	if err := rows.Scan(&rid, &name, &desc, &enabled, &eventType, &sev, &cond,
		&gid, &ch, &cd, &rec, &by, &uby, &createdAt, &updatedAt, &gname); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id": rid, "name": name, "description": desc, "enabled": enabled,
		"eventType": eventType, "severity": sev, "conditionJson": cond,
		"notificationGroupId": gid, "groupName": gname,
		"channels": ch, "cooldownSeconds": cd, "notifyOnRecovery": rec,
		"createdBy": by, "updatedBy": uby,
		"createdAt": createdAt, "updatedAt": updatedAt,
	})
}

type ruleBody struct {
	Name                *string  `json:"name"`
	Description         *string  `json:"description"`
	Enabled             *bool    `json:"enabled"`
	EventType           *string  `json:"eventType"`
	Severity            *string  `json:"severity"`
	ConditionJSON       *string  `json:"conditionJson"`
	NotificationGroupID *uint    `json:"notificationGroupId"`
	Channels            []string `json:"channels"`
	CooldownSeconds     *int     `json:"cooldownSeconds"`
	NotifyOnRecovery    *bool    `json:"notifyOnRecovery"`
}

func (h *NotificationRulesHandler) Create(c *gin.Context) {
	var body ruleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if body.Name == nil || body.EventType == nil || body.NotificationGroupID == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, eventType, and notificationGroupId are required"})
		return
	}
	name := strings.TrimSpace(*body.Name)
	desc := strOr(body.Description, "")
	eventType := strings.ToUpper(strings.TrimSpace(*body.EventType))
	sev := rules.NormalizeSeverity(strOr(body.Severity, ""))
	cond := ""
	if body.ConditionJSON != nil {
		cond = strings.TrimSpace(*body.ConditionJSON)
	}
	channels := body.Channels
	if channels == nil {
		channels = []string{"EMAIL"}
	}
	cd := 0
	if body.CooldownSeconds != nil {
		cd = *body.CooldownSeconds
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rec := false
	if body.NotifyOnRecovery != nil {
		rec = *body.NotifyOnRecovery
	}
	if err := rules.ValidateRule(name, desc, eventType, sev, cond, channels, cd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.groupExists(*body.NotificationGroupID) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notification group not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	id, err := h.DB.InsertID(`
		INSERT INTO notification_rules
		  (name,description,enabled,event_type,severity,condition_json,
		   notification_group_id,channels,cooldown_seconds,notify_on_recovery,
		   created_by,updated_by,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NOW(),NOW())`,
		name, desc, enabled, eventType, sev, cond, *body.NotificationGroupID,
		rules.NormalizeChannels(channels), cd, rec, u, u)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "rule name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	audit.Write(h.DB, u, "create", "notification_rule", strconv.FormatInt(id, 10), "ok", name)
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": name})
}

func (h *NotificationRulesHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body ruleBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	cur, ok := h.loadRule(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	name := strOr(body.Name, cur["name"].(string))
	desc := strOr(body.Description, cur["description"].(string))
	eventType := cur["eventType"].(string)
	if body.EventType != nil {
		eventType = strings.ToUpper(strings.TrimSpace(*body.EventType))
	}
	sev := cur["severity"].(string)
	if body.Severity != nil {
		sev = rules.NormalizeSeverity(*body.Severity)
	}
	cond := cur["conditionJson"].(string)
	if body.ConditionJSON != nil {
		cond = strings.TrimSpace(*body.ConditionJSON)
	}
	gid := cur["notificationGroupId"].(uint)
	if body.NotificationGroupID != nil {
		gid = *body.NotificationGroupID
	}
	channels := strings.Split(cur["channels"].(string), ",")
	if body.Channels != nil {
		channels = body.Channels
	}
	cd := cur["cooldownSeconds"].(int)
	if body.CooldownSeconds != nil {
		cd = *body.CooldownSeconds
	}
	enabled := cur["enabled"].(bool)
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	rec := cur["notifyOnRecovery"].(bool)
	if body.NotifyOnRecovery != nil {
		rec = *body.NotifyOnRecovery
	}
	if err := rules.ValidateRule(name, desc, eventType, sev, cond, channels, cd); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !h.groupExists(gid) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "notification group not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	_, err = h.DB.Exec(`
		UPDATE notification_rules SET
		  name=$1, description=$2, enabled=$3, event_type=$4, severity=$5,
		  condition_json=$6, notification_group_id=$7, channels=$8,
		  cooldown_seconds=$9, notify_on_recovery=$10,
		  updated_by=$11, updated_at=NOW()
		WHERE id=$12`,
		name, desc, enabled, eventType, sev, cond, gid,
		rules.NormalizeChannels(channels), cd, rec, u, id)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "unique") {
			c.JSON(http.StatusConflict, gin.H{"error": "rule name already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	audit.Write(h.DB, u, "update", "notification_rule", strconv.FormatInt(id, 10), "ok", name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// loadRule returns the stored row as plain values for PATCH merging.
func (h *NotificationRulesHandler) loadRule(id int64) (map[string]any, bool) {
	var name, desc, eventType, sev, cond, ch string
	var enabled, rec bool
	var gid uint
	var cd int
	err := h.DB.QueryRow(`
		SELECT name, description, enabled, event_type, severity, condition_json,
		       notification_group_id, channels, cooldown_seconds, notify_on_recovery
		FROM notification_rules WHERE id=$1`, id).Scan(
		&name, &desc, &enabled, &eventType, &sev, &cond, &gid, &ch, &cd, &rec)
	if err != nil {
		return nil, false
	}
	return map[string]any{
		"name": name, "description": desc, "enabled": enabled,
		"eventType": eventType, "severity": sev, "conditionJson": cond,
		"notificationGroupId": gid, "channels": ch, "cooldownSeconds": cd,
		"notifyOnRecovery": rec,
	}, true
}

func (h *NotificationRulesHandler) groupExists(gid uint) bool {
	var count int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM notification_groups WHERE id=$1`, gid).Scan(&count)
	return count > 0
}

func (h *NotificationRulesHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM notification_rules WHERE id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "rule not found"})
		return
	}
	_, _ = h.DB.Exec(`DELETE FROM notification_rule_state WHERE rule_id=$1`, id)
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "notification_rule", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func strOr(s *string, def string) string {
	if s == nil {
		return def
	}
	return strings.TrimSpace(*s)
}
