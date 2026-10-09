package rules

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"serverhub/internal/applog"
	"serverhub/internal/database"
	"serverhub/internal/notify"
)

// Sender delivers rule notifications. The production implementation fans out
// through the existing notify package; tests substitute a fake. SMTP is
// never performed inside a database transaction (see Engine.Evaluate).
type Sender interface {
	// SendEmail delivers subject/text to every member of groupID.
	SendEmail(groupID uint, subject, text string) error
	// SendTelegram delivers text to the configured chat.
	SendTelegram(text string) error
	// SendInApp files an in-app notification for every user.
	SendInApp(serverID *uint, alertID *int64, title, body string) error
}

// NotifySender is the production Sender backed by the notify package.
type NotifySender struct {
	DB *database.DB
}

func (s *NotifySender) SendEmail(groupID uint, subject, text string) error {
	to := notify.GroupMemberEmails(s.DB, groupID)
	if len(to) == 0 {
		return fmt.Errorf("notification group %d has no valid recipients", groupID)
	}
	return notify.SendEmailTo(s.DB, to, subject, text)
}

func (s *NotifySender) SendTelegram(text string) error {
	return notify.SendTelegramText(s.DB, text)
}

// DeliveryRequest is what the engine hands to the centralized delivery
// pipeline for EMAIL/TELEGRAM channels. IN_APP stays direct via Sender.
type DeliveryRequest struct {
	RuleID            uint
	GroupID           uint
	ServerID          *uint
	Key               string // stable incident identity
	Fingerprint       string
	Severity          string
	Title             string
	Body              string
	Channels          []string
	Labels            map[string]string
	Resource          string
	IsRecovery        bool
	CooldownSec       int
	RepeatSec         int
	MaxRepeats        int
	NotifyRecovery    bool
	DigestMode        string
	DigestIntervalMin int
	GroupBy           []string
}

func (s *NotifySender) SendInApp(serverID *uint, alertID *int64, title, body string) error {
	notify.NotifyUsers(s.DB, serverID, alertID, title, body, "alert")
	return nil
}

// Engine evaluates NotificationEvents against enabled rules.
// Zero value is safe but inert: Evaluate no-ops without DB + Sender.
type Engine struct {
	DB      *database.DB
	Sender  Sender
	Enabled bool
	// Now is overridden in tests; defaults to time.Now().UTC.
	Now func() time.Time
	// Deliver routes EMAIL/TELEGRAM through the centralized delivery
	// pipeline and returns the pipeline outcome action
	// (notified|queued|digested|suppressed). Nil keeps the legacy
	// direct-send behavior (used by unit tests without a pipeline).
	Deliver func(req DeliveryRequest) (string, error)
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// ruleRow is an enabled rule in deterministic evaluation order (id ASC).
type ruleRow struct {
	ID                  uint
	Name                string
	EventType           string
	Severity            string
	ConditionJSON       string
	NotificationGroupID uint
	Channels            []string
	CooldownSeconds     int
	NotifyOnRecovery    bool
	RepeatIntervalSec   int
	MaxRepeats          int
	DigestMode          string
	DigestIntervalMin   int
	GroupBy             []string
}

// Evaluate processes one event against all enabled rules for its type.
// Disabled engine, unknown types, and DB failures are silent no-ops (legacy
// pipeline runs independently). A failing rule is logged and skipped — it
// never blocks other rules.
func (e *Engine) Evaluate(ev Event) {
	if e == nil || !e.Enabled || e.DB == nil || e.Sender == nil {
		return
	}
	if ev.Fingerprint == "" {
		if ev.ServerID != nil {
			ev.Fingerprint = fmt.Sprintf("server:%d:%s", *ev.ServerID, ev.Condition)
		} else {
			ev.Fingerprint = "global:" + ev.Condition
		}
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = e.now()
	}
	rules, err := e.loadRules(ev.Type)
	if err != nil {
		e.log("rule evaluation failed", ev, "", fmt.Sprintf("load rules: %v", err))
		return
	}
	for _, r := range rules {
		if err := e.evaluateRule(r, ev); err != nil {
			e.log("rule evaluation failed", ev, r.Name, err.Error())
		}
	}
}

func (e *Engine) loadRules(eventType string) ([]ruleRow, error) {
	// Recovery events are also offered to rules watching the counterpart
	// firing type (when they opted into recovery): a rule on SERVER_ALERT
	// with notify_on_recovery handles SERVER_ALERT_RESOLVED.
	types := []string{eventType}
	recoveryOnly := ""
	switch eventType {
	case EventAgentOnline:
		types = append(types, EventAgentOffline)
		recoveryOnly = EventAgentOffline
	case EventServerAlertResolved:
		types = append(types, EventServerAlert)
		recoveryOnly = EventServerAlert
	}
	second := ""
	if len(types) > 1 {
		second = types[1]
	}
	rows, err := e.DB.Query(`
		SELECT id,name,event_type,severity,condition_json,notification_group_id,
		       channels,cooldown_seconds,notify_on_recovery,
		       repeat_interval_sec,max_repeats,digest_mode,digest_interval_min,group_by
		FROM notification_rules
		WHERE enabled=true AND event_type IN ($1,$2)
		  AND (event_type <> $3 OR notify_on_recovery=true)
		ORDER BY id ASC`, types[0], second, recoveryOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ruleRow
	for rows.Next() {
		var r ruleRow
		var sev, cond, ch string
		var gid uint
		var cd, ri, mr, dim int
		var rec bool
		var dm, gb string
		if err := rows.Scan(&r.ID, &r.Name, &r.EventType, &sev, &cond, &gid, &ch, &cd, &rec,
			&ri, &mr, &dm, &dim, &gb); err != nil {
			continue
		}
		r.Severity = sev
		r.ConditionJSON = cond
		r.NotificationGroupID = gid
		r.Channels = parseChannels(ch)
		r.CooldownSeconds = cd
		r.NotifyOnRecovery = rec
		r.RepeatIntervalSec = ri
		r.MaxRepeats = mr
		r.DigestMode = dm
		r.DigestIntervalMin = dim
		r.GroupBy = splitCSV(gb)
		out = append(out, r)
	}
	return out, rows.Err()
}

// splitCSV splits a stored comma list, trimming blanks.
func splitCSV(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseChannels splits a stored channel list, keeping supported values only.
func parseChannels(raw string) []string {
	var out []string
	for _, c := range strings.Split(raw, ",") {
		c = strings.ToUpper(strings.TrimSpace(c))
		if validChannel(c) && !contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func contains(in []string, s string) bool {
	for _, v := range in {
		if v == s {
			return true
		}
	}
	return false
}

// evaluateRule matches one rule and dispatches. State claim + cooldown check
// happen inside a row-locked transaction; delivery happens after commit.
// Delivery semantics: at-most-once per cooldown window is favored over
// at-least-once — a crash between state commit and SMTP send may skip one
// notification rather than duplicate it.
func (e *Engine) evaluateRule(r ruleRow, ev Event) error {
	// Recovery events match on incident identity (fingerprint + prior
	// firing state), never on re-evaluated severity/conditions: a recovered
	// value would fail the very condition that fired the alert.
	if !ev.IsRecovery() {
		if r.Severity != "" && !strings.EqualFold(r.Severity, ev.Severity) {
			return nil
		}
		cond, err := ParseCondition(r.ConditionJSON)
		if err != nil {
			return fmt.Errorf("rule %q bad condition: %w", r.Name, err)
		}
		if !cond.Matches(ev) {
			return nil
		}
	}
	if len(r.Channels) == 0 {
		return fmt.Errorf("rule %q has no valid channels", r.Name)
	}

	suppress, err := e.claim(r, ev)
	if err != nil {
		return err
	}
	if suppress != "" {
		e.log("rule suppressed ("+suppress+")", ev, r.Name, "")
		return nil
	}

	title := ev.Severity + ": " + ev.Message
	if ev.Severity == "" {
		title = ev.Message
	}
	body := ev.Message
	var external []string
	for _, ch := range r.Channels {
		switch ch {
		case "EMAIL", "TELEGRAM":
			external = append(external, ch)
		case "IN_APP":
			if derr := e.Sender.SendInApp(ev.ServerID, ev.AlertID, title, body); derr != nil {
				e.log("rule dispatch failed", ev, r.Name, ch+": "+derr.Error())
			} else {
				e.log("rule dispatched", ev, r.Name, ch)
			}
		}
	}
	if len(external) > 0 {
		req := DeliveryRequest{
			RuleID: r.ID, GroupID: r.NotificationGroupID, ServerID: ev.ServerID,
			Key: fmt.Sprintf("rule:%d:%s", r.ID, ev.Fingerprint),
			Fingerprint: ev.Fingerprint,
			Severity: ev.Severity, Title: title, Body: body,
			Channels: external,
			Labels:   map[string]string{"condition": ev.Condition},
			Resource: ev.Message,
			IsRecovery: ev.IsRecovery(),
			CooldownSec: r.CooldownSeconds, RepeatSec: r.RepeatIntervalSec,
			MaxRepeats: r.MaxRepeats, NotifyRecovery: r.NotifyOnRecovery,
			DigestMode: r.DigestMode, DigestIntervalMin: r.DigestIntervalMin,
			GroupBy: r.GroupBy,
		}
		if e.Deliver != nil {
			action, derr := e.Deliver(req)
			if derr != nil {
				e.log("rule delivery failed", ev, r.Name, derr.Error())
			} else {
				e.log("rule delivery "+action, ev, r.Name, strings.Join(external, ","))
			}
		} else {
			// Legacy direct-send fallback (unit tests without a pipeline).
			for _, ch := range external {
				var derr error
				switch ch {
				case "EMAIL":
					derr = e.Sender.SendEmail(r.NotificationGroupID, title, body)
				case "TELEGRAM":
					derr = e.Sender.SendTelegram(title + "\n" + body)
				}
				if derr != nil {
					e.log("rule dispatch failed", ev, r.Name, ch+": "+derr.Error())
				} else {
					e.log("rule dispatched", ev, r.Name, ch)
				}
			}
		}
	}
	return nil
}

// claim locks the (rule, fingerprint) state row, decides whether this event
// notifies, and advances the state. Returns a suppress reason or "" to
// proceed with delivery.
func (e *Engine) claim(r ruleRow, ev Event) (suppress string, err error) {
	tx, err := e.DB.SQL.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var lastNotified time.Time
	var lastState string
	var hasRow bool
	err = tx.QueryRow(`SELECT last_notified_at, last_state FROM notification_rule_state
		WHERE rule_id=$1 AND fingerprint=$2 FOR UPDATE`, r.ID, ev.Fingerprint).Scan(&lastNotified, &lastState)
	if err == sql.ErrNoRows {
		hasRow = false
	} else if err != nil {
		return "", err
	} else {
		hasRow = true
	}
	now := e.now()
	newState := "firing"
	if ev.IsRecovery() {
		newState = "resolved"
		if !r.NotifyOnRecovery || lastState != "firing" {
			// Silent bookkeeping: recovery observed, but nothing to notify
			// (disabled, or no firing notification preceded it).
			if err := e.upsertState(tx, r.ID, ev, hasRow, now, "resolved"); err != nil {
				return "", err
			}
			if err := tx.Commit(); err != nil {
				return "", err
			}
			return "recovery without preceding firing", nil
		}
	} else if r.CooldownSeconds > 0 && hasRow && lastState == "firing" {
		if now.Sub(lastNotified) < time.Duration(r.CooldownSeconds)*time.Second {
			if err := e.touchEvent(tx, r.ID, ev, now); err != nil {
				return "", err
			}
			if err := tx.Commit(); err != nil {
				return "", err
			}
			return "cooldown", nil
		}
	}
	if err := e.upsertState(tx, r.ID, ev, hasRow, now, newState); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return "", nil
}

func (e *Engine) upsertState(tx *sql.Tx, ruleID uint, ev Event, hasRow bool, now time.Time, state string) error {
	if hasRow {
		_, err := tx.Exec(`UPDATE notification_rule_state
			SET last_notified_at=$1, last_state=$2, last_event_at=$3, updated_at=$4
			WHERE rule_id=$5 AND fingerprint=$6`, now, state, ev.Timestamp, now, ruleID, ev.Fingerprint)
		return err
	}
	_, err := tx.Exec(`INSERT INTO notification_rule_state
		(rule_id,fingerprint,last_notified_at,last_state,last_event_at,created_at,updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, ruleID, ev.Fingerprint, now, state, ev.Timestamp, now, now)
	return err
}

func (e *Engine) touchEvent(tx *sql.Tx, ruleID uint, ev Event, now time.Time) error {
	_, err := tx.Exec(`UPDATE notification_rule_state
		SET last_event_at=$1, updated_at=$2 WHERE rule_id=$3 AND fingerprint=$4`,
		ev.Timestamp, now, ruleID, ev.Fingerprint)
	return err
}

// log records engine activity without secrets (counts, never addresses).
func (e *Engine) log(msg string, ev Event, rule, detail string) {
	sid := "nil"
	if ev.ServerID != nil {
		sid = fmt.Sprintf("%d", *ev.ServerID)
	}
	extra := ""
	if detail != "" {
		extra = " (" + detail + ")"
	}
	if e.DB == nil {
		return
	}
	applog.Info(e.DB.GDB, "system",
		fmt.Sprintf("notify: %s rule=%q event=%s server=%s fp=%s%s",
			msg, rule, ev.Type, sid, ev.Fingerprint, extra))
}
