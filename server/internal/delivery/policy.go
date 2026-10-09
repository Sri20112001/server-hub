// Package delivery is the centralized notification policy engine.
//
// Every external message (EMAIL, TELEGRAM) flows through Dispatch, which
// applies one consistent policy: emergency pause → maintenance windows →
// quiet hours → per-incident dedupe/cooldown/repeats → digest batching →
// rate limits → durable outbox. A background worker sends due rows with
// bounded retries, backoff+jitter, and per-channel circuit breakers.
//
// IN_APP stays direct (cheap in-DB rows, cleared by the user) and never
// enters this pipeline.
package delivery

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"serverhub/internal/database"
)

// Severity levels (uppercased by callers; normalized here defensively).
const (
	SevCritical = "CRITICAL"
	SevWarning  = "WARNING"
	SevInfo     = "INFO"
)

// Channels delivered through this pipeline.
const (
	ChEmail    = "EMAIL"
	ChTelegram = "TELEGRAM"
)

// Outbox statuses.
const (
	StatusPending  = "pending"
	StatusSending  = "sending"
	StatusSent     = "sent"
	StatusFailed   = "failed"
	StatusDeferred = "deferred"
)

// Suppression reasons (observable, never silent).
const (
	ReasonEmergencyPause = "emergency-pause"
	ReasonMaintenance    = "maintenance"
	ReasonQuietHours     = "quiet-hours"
	ReasonCooldown       = "cooldown"
	ReasonMaxRepeats     = "max-repeats"
	ReasonRateLimited    = "rate-limited"
	ReasonRecoveryMuted  = "recovery-muted"
	ReasonDigestBatched  = "digest-batched"
)

// Policy mirrors the notification_policy singleton with safe defaults for
// a missing row (fresh installs seed row id=1 via migration).
type Policy struct {
	EmergencyPause      bool
	PauseUntil          *time.Time
	PauseReason         string
	CooldownCriticalSec int
	CooldownWarningSec  int
	CooldownInfoSec     int
	RepeatIntervalSec   int
	MaxRepeats          int
	NotifyOnRecovery    bool
	EmailPerHour        int
	TgPerHour           int
	DigestIntervalMin   int
}

func defaultPolicy() Policy {
	return Policy{
		CooldownCriticalSec: 900,
		CooldownWarningSec:  3600,
		CooldownInfoSec:     21600,
		RepeatIntervalSec:   3600,
		MaxRepeats:          3,
		NotifyOnRecovery:    true,
		EmailPerHour:        60,
		TgPerHour:           60,
		DigestIntervalMin:   60,
	}
}

// LoadPolicy reads the singleton row, seeding defaults on first use.
// Missing/invalid values fall back to safe defaults, never to zero.
func LoadPolicy(db *database.DB) Policy {
	p := defaultPolicy()
	if db == nil {
		return p
	}
	var id uint
	var pause bool
	var until *time.Time
	var reason string
	var cc, cw, ci, ri, mr, eph, tph, dim int
	var rec bool
	err := db.QueryRow(`SELECT id, emergency_pause, pause_until, pause_reason,
		cooldown_critical_sec, cooldown_warning_sec, cooldown_info_sec,
		repeat_interval_sec, max_repeats, notify_on_recovery,
		email_per_hour, tg_per_hour, digest_interval_min
		FROM notification_policy WHERE id=1`).
		Scan(&id, &pause, &until, &reason, &cc, &cw, &ci, &ri, &mr, &rec, &eph, &tph, &dim)
	if err != nil {
		if err == sql.ErrNoRows {
			_, _ = db.Exec(`INSERT INTO notification_policy (id) VALUES (1) ON CONFLICT (id) DO NOTHING`)
		}
		return p
	}
	d := defaultPolicy()
	p.EmergencyPause = pause
	p.PauseUntil = until
	p.PauseReason = reason
	p.CooldownCriticalSec = positiveOr(cc, d.CooldownCriticalSec)
	p.CooldownWarningSec = positiveOr(cw, d.CooldownWarningSec)
	p.CooldownInfoSec = positiveOr(ci, d.CooldownInfoSec)
	p.RepeatIntervalSec = positiveOr(ri, d.RepeatIntervalSec)
	p.MaxRepeats = nonNegativeOr(mr, d.MaxRepeats)
	p.NotifyOnRecovery = rec
	p.EmailPerHour = positiveOr(eph, d.EmailPerHour)
	p.TgPerHour = positiveOr(tph, d.TgPerHour)
	p.DigestIntervalMin = positiveOr(dim, d.DigestIntervalMin)
	return p
}

func positiveOr(v, def int) int {
	if v > 0 {
		return v
	}
	return def
}

func nonNegativeOr(v, def int) int {
	if v >= 0 {
		return v
	}
	return def
}

// NormalizeSeverity uppercases and validates; unknown becomes WARNING
// (never silently INFO, never empty).
func NormalizeSeverity(s string) string {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case SevCritical:
		return SevCritical
	case SevInfo:
		return SevInfo
	case SevWarning:
		return SevWarning
	default:
		return SevWarning
	}
}

// CooldownFor returns the per-severity repeat-notify interval.
func (p Policy) CooldownFor(severity string) time.Duration {
	switch NormalizeSeverity(severity) {
	case SevCritical:
		return time.Duration(p.CooldownCriticalSec) * time.Second
	case SevInfo:
		return time.Duration(p.CooldownInfoSec) * time.Second
	default:
		return time.Duration(p.CooldownWarningSec) * time.Second
	}
}

// BudgetFor returns the hourly message budget per channel.
func (p Policy) BudgetFor(channel string) int {
	if channel == ChTelegram {
		return p.TgPerHour
	}
	return p.EmailPerHour
}

// PauseActive reports whether the emergency pause suppresses delivery now.
func (p Policy) PauseActive(now time.Time) bool {
	if !p.EmergencyPause {
		return false
	}
	// An expiry in the past (or unset with pause on) — treat unset as
	// indefinite; expired pauses lift automatically.
	if p.PauseUntil != nil && !p.PauseUntil.IsZero() && now.After(*p.PauseUntil) {
		return false
	}
	return true
}

// MaintenanceActive reports whether a maintenance window covers scope now.
// Scope "all" matches everything; "server:<id>" matches that server.
func MaintenanceActive(db *database.DB, serverID *uint, now time.Time) (bool, string) {
	if db == nil {
		return false, ""
	}
	rows, err := db.Query(`SELECT name, scope FROM maintenance_windows
		WHERE enabled=true AND starts_at <= $1 AND ends_at > $1`, now)
	if err != nil {
		return false, ""
	}
	defer rows.Close()
	for rows.Next() {
		var name, scope string
		if err := rows.Scan(&name, &scope); err != nil {
			continue
		}
		if scope == "" || scope == "all" {
			return true, name
		}
		if serverID != nil && scope == "server:"+strconv.FormatUint(uint64(*serverID), 10) {
			return true, name
		}
	}
	return false, ""
}

// QuietInfo carries a group's quiet-hours configuration.
type QuietInfo struct {
	Start         string // "HH:MM", empty disables
	End           string
	TZ            string
	AllowCritical bool
}

// LoadGroupQuiet reads quiet-hours config for a group (zero value = off).
func LoadGroupQuiet(db *database.DB, groupID uint) QuietInfo {
	var q QuietInfo
	if db == nil || groupID == 0 {
		return q
	}
	_ = db.QueryRow(`SELECT quiet_start, quiet_end, quiet_tz, quiet_allow_critical
		FROM notification_groups WHERE id=$1`, groupID).
		Scan(&q.Start, &q.End, &q.TZ, &q.AllowCritical)
	return q
}

// InQuiet reports whether now falls in the quiet window. Empty start/end
// disables. Overnight spans (22:00-06:00) work; the IANA timezone handles
// DST, falling back to UTC when unloadable or blank.
func (q QuietInfo) InQuiet(now time.Time) bool {
	start, err1 := parseHM(q.Start)
	end, err2 := parseHM(q.End)
	if err1 != nil || err2 != nil {
		return false
	}
	loc := time.UTC
	if tz := strings.TrimSpace(q.TZ); tz != "" {
		if l, err := time.LoadLocation(tz); err == nil {
			loc = l
		}
	}
	mins := now.In(loc).Hour()*60 + now.In(loc).Minute()
	s, e := start[0]*60+start[1], end[0]*60+end[1]
	if s == e {
		return false
	}
	if s < e {
		return mins >= s && mins < e
	}
	return mins >= s || mins < e // overnight span
}

func parseHM(s string) ([2]int, error) {
	var out [2]int
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return out, fmt.Errorf("want HH:MM")
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return out, err
		}
		out[i] = n
	}
	if out[0] < 0 || out[0] > 23 || out[1] < 0 || out[1] > 59 {
		return out, fmt.Errorf("out of range")
	}
	return out, nil
}
