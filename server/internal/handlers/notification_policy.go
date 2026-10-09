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
	"serverhub/internal/delivery"
	"serverhub/internal/middleware"
)

// ─── Global delivery policy ───────────────────────────────────────────────

// PolicyHandler serves the singleton delivery policy. Reading is viewer+;
// changes are admin-only and audit-logged. The policy carries no secrets.
type PolicyHandler struct {
	DB *database.DB
}

func (h *PolicyHandler) Get(c *gin.Context) {
	p := delivery.LoadPolicy(h.DB)
	c.JSON(http.StatusOK, policyView(p))
}

func policyView(p delivery.Policy) gin.H {
	return gin.H{
		"emergencyPause": p.EmergencyPause, "pauseUntil": p.PauseUntil,
		"pauseReason": p.PauseReason,
		"cooldownCriticalSec": p.CooldownCriticalSec,
		"cooldownWarningSec":  p.CooldownWarningSec,
		"cooldownInfoSec":     p.CooldownInfoSec,
		"repeatIntervalSec": p.RepeatIntervalSec, "maxRepeats": p.MaxRepeats,
		"notifyOnRecovery": p.NotifyOnRecovery,
		"emailPerHour": p.EmailPerHour, "tgPerHour": p.TgPerHour,
		"digestIntervalMin": p.DigestIntervalMin,
	}
}

func (h *PolicyHandler) Update(c *gin.Context) {
	var body struct {
		EmergencyPause      *bool   `json:"emergencyPause"`
		PauseUntil          *string `json:"pauseUntil"`
		PauseReason         *string `json:"pauseReason"`
		CooldownCriticalSec *int    `json:"cooldownCriticalSec"`
		CooldownWarningSec  *int    `json:"cooldownWarningSec"`
		CooldownInfoSec     *int    `json:"cooldownInfoSec"`
		RepeatIntervalSec   *int    `json:"repeatIntervalSec"`
		MaxRepeats          *int    `json:"maxRepeats"`
		NotifyOnRecovery    *bool   `json:"notifyOnRecovery"`
		EmailPerHour        *int    `json:"emailPerHour"`
		TgPerHour           *int    `json:"tgPerHour"`
		DigestIntervalMin   *int    `json:"digestIntervalMin"`
		ClearPause          *bool   `json:"clearPause"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	between := func(name string, v *int, min, max int) error {
		if v == nil {
			return nil
		}
		if *v < min || *v > max {
			return fmt.Errorf("%s must be %d..%d", name, min, max)
		}
		return nil
	}
	for _, e := range []error{
		between("cooldownCriticalSec", body.CooldownCriticalSec, 60, 30*24*3600),
		between("cooldownWarningSec", body.CooldownWarningSec, 60, 30*24*3600),
		between("cooldownInfoSec", body.CooldownInfoSec, 60, 30*24*3600),
		between("repeatIntervalSec", body.RepeatIntervalSec, 300, 30*24*3600),
		between("maxRepeats", body.MaxRepeats, 0, 100),
		between("emailPerHour", body.EmailPerHour, 1, 10000),
		between("tgPerHour", body.TgPerHour, 1, 10000),
		between("digestIntervalMin", body.DigestIntervalMin, 15, 1440),
	} {
		if e != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": e.Error()})
			return
		}
	}
	var until any
	if body.PauseUntil != nil && strings.TrimSpace(*body.PauseUntil) != "" {
		t, err := time.Parse(time.RFC3339, strings.TrimSpace(*body.PauseUntil))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pauseUntil must be RFC3339"})
			return
		}
		if t.Before(time.Now().UTC()) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pauseUntil must be in the future"})
			return
		}
		until = t.UTC()
	}
	var reason any
	if body.PauseReason != nil {
		r := strings.TrimSpace(*body.PauseReason)
		if len(r) > 300 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "pauseReason too long (max 300)"})
			return
		}
		reason = r
	}
	var pause any
	if body.EmergencyPause != nil {
		pause = *body.EmergencyPause
	}
	if body.ClearPause != nil && *body.ClearPause {
		pause, until, reason = false, nil, ""
	}
	cols := []string{}
	args := []any{}
	set := func(col string, v any) {
		if v == nil {
			return
		}
		cols = append(cols, col+"=$"+strconv.Itoa(len(args)+1))
		args = append(args, v)
	}
	set("emergency_pause", pause)
	set("pause_until", until)
	set("pause_reason", reason)
	set("cooldown_critical_sec", intOrNil(body.CooldownCriticalSec))
	set("cooldown_warning_sec", intOrNil(body.CooldownWarningSec))
	set("cooldown_info_sec", intOrNil(body.CooldownInfoSec))
	set("repeat_interval_sec", intOrNil(body.RepeatIntervalSec))
	set("max_repeats", intOrNil(body.MaxRepeats))
	set("notify_on_recovery", boolOrNil(body.NotifyOnRecovery))
	set("email_per_hour", intOrNil(body.EmailPerHour))
	set("tg_per_hour", intOrNil(body.TgPerHour))
	set("digest_interval_min", intOrNil(body.DigestIntervalMin))
	if len(cols) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to update"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	cols = append(cols, "updated_by=$"+strconv.Itoa(len(args)+1), "updated_at=NOW()")
	args = append(args, u)
	if _, err := h.DB.Exec(`UPDATE notification_policy SET `+strings.Join(cols, ", ")+` WHERE id=1`, args...); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	audit.Write(h.DB, u, "update", "notification_policy", "1", "ok", strings.Join(cols, ","))
	c.JSON(http.StatusOK, policyView(delivery.LoadPolicy(h.DB)))
}

func intOrNil(v *int) any {
	if v == nil {
		return nil
	}
	return *v
}

func boolOrNil(v *bool) any {
	if v == nil {
		return nil
	}
	return *v
}

// ─── Delivery history ─────────────────────────────────────────────────────

type DeliveriesHandler struct {
	DB *database.DB
}

// List recent outbox rows (titles/bodies truncated): delivery history and
// failures without exposing provider secrets (none are stored here).
func (h *DeliveriesHandler) List(c *gin.Context) {
	limit := 50
	if v, err := strconv.Atoi(c.DefaultQuery("limit", "50")); err == nil && v > 0 && v <= 200 {
		limit = v
	}
	status := strings.ToLower(strings.TrimSpace(c.Query("status")))
	channel := strings.ToUpper(strings.TrimSpace(c.Query("channel")))
	q := `SELECT id,dedupe_key,severity,channel,group_id,
		LEFT(title,200) AS title,LEFT(body,500) AS body,
		status,attempts,next_retry_at,last_error,created_at,sent_at
		FROM delivery_outbox WHERE 1=1`
	args := []any{}
	if status != "" {
		switch status {
		case "pending", "sending", "sent", "failed", "deferred":
			q += ` AND status=$` + strconv.Itoa(len(args)+1)
			args = append(args, status)
		default:
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
			return
		}
	}
	if channel != "" {
		if channel != "EMAIL" && channel != "TELEGRAM" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel"})
			return
		}
		q += ` AND channel=$` + strconv.Itoa(len(args)+1)
		args = append(args, channel)
	}
	q += ` ORDER BY id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit)
	rows, err := h.DB.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uint
		var gid uint
		var key, sev, ch, title, bdy, st, lerr string
		var att int
		var next time.Time
		var created time.Time
		var sent *time.Time
		if err := rows.Scan(&id, &key, &sev, &ch, &gid, &title, &bdy,
			&st, &att, &next, &lerr, &created, &sent); err == nil {
			out = append(out, gin.H{
				"id": id, "dedupeKey": key, "severity": sev, "channel": ch,
				"groupId": gid, "title": title, "body": bdy, "status": st,
				"attempts": att, "nextRetryAt": next, "lastError": lerr,
				"createdAt": created, "sentAt": sent,
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

// ─── Maintenance windows ──────────────────────────────────────────────────

type MaintenanceHandler struct {
	DB *database.DB
}

func (h *MaintenanceHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`SELECT id,name,scope,starts_at,ends_at,reason,enabled,created_by,created_at,updated_at
		FROM maintenance_windows ORDER BY starts_at`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []gin.H{}
	for rows.Next() {
		var id uint
		var name, scope, reason, by string
		var enabled bool
		var s, e, created, updated time.Time
		if err := rows.Scan(&id, &name, &scope, &s, &e, &reason, &enabled, &by, &created, &updated); err == nil {
			active := enabled && !time.Now().UTC().Before(s) && time.Now().UTC().Before(e)
			out = append(out, gin.H{
				"id": id, "name": name, "scope": scope,
				"startsAt": s, "endsAt": e, "reason": reason,
				"enabled": enabled, "active": active, "createdBy": by,
				"createdAt": created, "updatedAt": updated,
			})
		}
	}
	c.JSON(http.StatusOK, out)
}

func validateWindow(name, scope string, s, e time.Time) error {
	if strings.TrimSpace(name) == "" || len(name) > 100 {
		return fmt.Errorf("name is required (max 100)")
	}
	if scope == "" || scope == "all" {
		scope = "all"
	} else if strings.HasPrefix(scope, "server:") {
		if _, err := strconv.ParseUint(strings.TrimPrefix(scope, "server:"), 10, 32); err != nil {
			return fmt.Errorf("scope must be all or server:<id>")
		}
	} else {
		return fmt.Errorf("scope must be all or server:<id>")
	}
	if !e.After(s) {
		return fmt.Errorf("endsAt must be after startsAt")
	}
	if e.Sub(s) > 30*24*time.Hour {
		return fmt.Errorf("window too long (max 30 days)")
	}
	return nil
}

func (h *MaintenanceHandler) Create(c *gin.Context) {
	var body struct {
		Name    string `json:"name"`
		Scope   string `json:"scope"`
		StartsAt string `json:"startsAt"`
		EndsAt   string `json:"endsAt"`
		Reason  string `json:"reason"`
		Enabled *bool  `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	s, err := time.Parse(time.RFC3339, strings.TrimSpace(body.StartsAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "startsAt must be RFC3339"})
		return
	}
	e, err := time.Parse(time.RFC3339, strings.TrimSpace(body.EndsAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "endsAt must be RFC3339"})
		return
	}
	scope := strings.TrimSpace(body.Scope)
	if scope == "" {
		scope = "all"
	}
	if err := validateWindow(body.Name, scope, s.UTC(), e.UTC()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(body.Reason) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason too long (max 500)"})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	u, _ := middleware.CurrentUser(c)
	id, err := h.DB.InsertID(`INSERT INTO maintenance_windows
		(name,scope,starts_at,ends_at,reason,enabled,created_by,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NOW(),NOW())`,
		strings.TrimSpace(body.Name), scope, s.UTC(), e.UTC(),
		strings.TrimSpace(body.Reason), enabled, u)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	audit.Write(h.DB, u, "create", "maintenance_window", strconv.FormatInt(id, 10), "ok", body.Name)
	c.JSON(http.StatusCreated, gin.H{"id": id, "name": strings.TrimSpace(body.Name)})
}

func (h *MaintenanceHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var cur struct {
		name, scope, reason string
		s, e                time.Time
		enabled             bool
	}
	if err := h.DB.QueryRow(`SELECT name,scope,starts_at,ends_at,reason,enabled
		FROM maintenance_windows WHERE id=$1`, id).
		Scan(&cur.name, &cur.scope, &cur.s, &cur.e, &cur.reason, &cur.enabled); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "window not found"})
		return
	}
	var body struct {
		Name    *string `json:"name"`
		Scope   *string `json:"scope"`
		StartsAt *string `json:"startsAt"`
		EndsAt   *string `json:"endsAt"`
		Reason  *string `json:"reason"`
		Enabled *bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	name := cur.name
	if body.Name != nil {
		name = strings.TrimSpace(*body.Name)
	}
	scope := cur.scope
	if body.Scope != nil {
		scope = strings.TrimSpace(*body.Scope)
		if scope == "" {
			scope = "all"
		}
	}
	s, e := cur.s, cur.e
	if body.StartsAt != nil {
		var err error
		if s, err = time.Parse(time.RFC3339, strings.TrimSpace(*body.StartsAt)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "startsAt must be RFC3339"})
			return
		}
	}
	if body.EndsAt != nil {
		var err error
		if e, err = time.Parse(time.RFC3339, strings.TrimSpace(*body.EndsAt)); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "endsAt must be RFC3339"})
			return
		}
	}
	reason := cur.reason
	if body.Reason != nil {
		reason = strings.TrimSpace(*body.Reason)
	}
	enabled := cur.enabled
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	if err := validateWindow(name, scope, s.UTC(), e.UTC()); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(reason) > 500 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason too long (max 500)"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	if _, err := h.DB.Exec(`UPDATE maintenance_windows SET
		name=$1,scope=$2,starts_at=$3,ends_at=$4,reason=$5,enabled=$6,updated_at=NOW()
		WHERE id=$7`, name, scope, s.UTC(), e.UTC(), reason, enabled, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	audit.Write(h.DB, u, "update", "maintenance_window", strconv.FormatInt(id, 10), "ok", name)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *MaintenanceHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM maintenance_windows WHERE id=$1`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "window not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete", "maintenance_window", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

