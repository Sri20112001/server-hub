package delivery

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"serverhub/internal/applog"
	"serverhub/internal/database"
)

// Digest modes.
const (
	DigestImmediate = "immediate"
	DigestDigest    = "digest"
)

// Input is one notification request. Key must be a stable incident
// identity (source + fingerprint/event + resource) — never message text,
// which varies per evaluation and would defeat dedupe.
type Input struct {
	Key        string
	Severity   string
	Title      string
	Body       string
	Channels   []string // subset of EMAIL, TELEGRAM
	GroupID    uint
	ServerID   *uint
	RuleID     uint   // 0 for non-rule (legacy) sends
	Labels     map[string]string
	Resource   string // human resource summary for digests
	IsRecovery bool
	// Resolved per-incident config (rule values or policy defaults):
	CooldownSec   int
	RepeatSec     int
	MaxRepeats    int
	NotifyRecovery bool
	// Digest config (rule-driven; legacy sends are always immediate):
	DigestMode        string
	DigestIntervalMin int
	GroupBy           []string
}

// Outcome describes what Dispatch decided (observable, never silent).
type Outcome struct {
	Action string // notified | queued | digested | suppressed
	Reason string // suppress reason or "" when delivered/queued
}

// NowFunc allows tests to pin the clock.
var NowFunc func() time.Time

func now() time.Time {
	if NowFunc != nil {
		return NowFunc().UTC()
	}
	return time.Now().UTC()
}

// Dispatch applies the full policy and either enqueues channel deliveries
// or records a suppression. State claim + enqueue happen in one
// transaction: concurrent workers cannot double-enqueue one incident, and
// a crash before commit sends nothing (at-most-once favored, matching the
// rules engine's established semantics).
func Dispatch(db *database.DB, in Input) (Outcome, error) {
	if db == nil {
		return Outcome{}, fmt.Errorf("nil database")
	}
	in.Severity = NormalizeSeverity(in.Severity)
	in.Channels = cleanChannels(in.Channels)
	if in.Key == "" || len(in.Channels) == 0 {
		return Outcome{}, fmt.Errorf("key and at least one channel are required")
	}
	if in.Title == "" {
		in.Title = in.Key
	}
	ts := now()
	pol := LoadPolicy(db)

	// 1. Emergency pause (evaluation continues; only sending pauses).
	if pol.PauseActive(ts) {
		touchSuppressed(db, in, ReasonEmergencyPause, ts)
		return Outcome{Action: "suppressed", Reason: ReasonEmergencyPause}, nil
	}
	// 2. Maintenance windows.
	if ok, name := MaintenanceActive(db, in.ServerID, ts); ok {
		// Critical incidents during maintenance are still recorded but
		// held: they join the overflow summary instead of paging.
		touchSuppressed(db, in, ReasonMaintenance, ts)
		log(db, "maintenance window %q suppressed %s", name, in.Key)
		return Outcome{Action: "suppressed", Reason: ReasonMaintenance}, nil
	}
	// 3. Quiet hours (group-scoped; critical bypass per group config).
	quiet := LoadGroupQuiet(db, in.GroupID)
	if quiet.InQuiet(ts) && !(in.Severity == SevCritical && quiet.AllowCritical) {
		touchSuppressed(db, in, ReasonQuietHours, ts)
		return Outcome{Action: "suppressed", Reason: ReasonQuietHours}, nil
	}

	// 4. Per-incident claim + 5. enqueue, atomically.
	tx, err := db.SQL.Begin()
	if err != nil {
		return Outcome{}, err
	}
	defer tx.Rollback()
	outcome, err := claimAndEnqueue(tx, db, pol, in, ts)
	if err != nil {
		return Outcome{}, err
	}
	if err := tx.Commit(); err != nil {
		return Outcome{}, err
	}
	return outcome, nil
}

func cleanChannels(in []string) []string {
	var out []string
	for _, c := range in {
		c = strings.ToUpper(strings.TrimSpace(c))
		if (c == ChEmail || c == ChTelegram) && !contains(out, c) {
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

// claimAndEnqueue runs inside the caller's transaction.
func claimAndEnqueue(tx *sql.Tx, db *database.DB, pol Policy, in Input, ts time.Time) (Outcome, error) {
	var lastNotified time.Time
	var repeatCount, suppressed int
	var lastState, lastReason string
	var hasRow bool
	err := tx.QueryRow(`SELECT last_notified_at, repeat_count, last_state, suppressed_count, last_suppress_reason
		FROM delivery_state WHERE dedupe_key=$1 FOR UPDATE`, in.Key).
		Scan(&lastNotified, &repeatCount, &lastState, &suppressed, &lastReason)
	if err == sql.ErrNoRows {
		hasRow = false
	} else if err != nil {
		return Outcome{}, err
	} else {
		hasRow = true
	}

	cooldown := time.Duration(in.CooldownSec) * time.Second
	if in.CooldownSec <= 0 {
		cooldown = pol.CooldownFor(in.Severity)
	}
	repeatSec := in.RepeatSec
	if repeatSec <= 0 {
		repeatSec = pol.RepeatIntervalSec
	}
	maxRep := in.MaxRepeats
	if maxRep <= 0 {
		// Zero means "policy default" at the API layer; the dispatcher
		// default of 0 would mean "no repeats ever", so fall back.
		maxRep = pol.MaxRepeats
	}
	notifyRec := in.NotifyRecovery || (!hasRow && pol.NotifyOnRecovery && in.IsRecovery)

	setState := func(state string, notified bool, reason string, suppr int) error {
		ln := lastNotified
		rc := repeatCount
		if notified {
			ln = ts
			if hasRow && lastState == "firing" && state == "firing" {
				rc++
			} else if !hasRow || lastState == "resolved" {
				rc = 0
			}
		}
		if hasRow {
			_, err := tx.Exec(`UPDATE delivery_state SET severity=$1,
				first_notified_at=CASE WHEN $2 THEN $3 ELSE first_notified_at END,
				last_notified_at=$4, repeat_count=$5, last_state=$6,
				suppressed_count=$7, last_suppress_reason=$8, updated_at=$9
				WHERE dedupe_key=$10`,
				in.Severity, !hasRow, ts, ln, rc, state, suppr, reason, ts, in.Key)
			return err
		}
		first := ts
		if !notified {
			first = time.Time{}
		} else if ln.IsZero() {
			ln = ts
		}
		_, err := tx.Exec(`INSERT INTO delivery_state
			(dedupe_key,severity,first_notified_at,last_notified_at,repeat_count,
			 last_state,suppressed_count,last_suppress_reason,updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			in.Key, in.Severity, first, ln, rc, state, suppr, reason, ts)
		return err
	}

	enqueue := func() error {
		for _, ch := range in.Channels {
			if _, err := tx.Exec(`INSERT INTO delivery_outbox
				(dedupe_key,severity,channel,group_id,title,body,status,attempts,next_retry_at)
				VALUES ($1,$2,$3,$4,$5,$6,'pending',0,$7)`,
				in.Key, in.Severity, ch, in.GroupID,
				truncate(in.Title, 300), truncate(in.Body, 4000), ts); err != nil {
				return err
			}
		}
		return nil
	}

	// Recovery path: notify once per incident when enabled; a recovery
	// with no preceding firing notification is muted, never paged.
	if in.IsRecovery {
		if !notifyRec || lastState != "firing" {
			if err := setState("resolved", false, ReasonRecoveryMuted, suppressed+1); err != nil {
				return Outcome{}, err
			}
			return Outcome{Action: "suppressed", Reason: ReasonRecoveryMuted}, nil
		}
		if in.DigestMode == DigestDigest {
			if err := appendDigest(tx, in, ts); err != nil {
				return Outcome{}, err
			}
			if err := setState("resolved", true, "", suppressed); err != nil {
				return Outcome{}, err
			}
			return Outcome{Action: "digested", Reason: ReasonDigestBatched}, nil
		}
		if err := enqueue(); err != nil {
			return Outcome{}, err
		}
		if err := setState("resolved", true, "", suppressed); err != nil {
			return Outcome{}, err
		}
		return Outcome{Action: "queued"}, nil
	}

	// Firing path.
	if hasRow && lastState == "resolved" {
		// Resolved-then-retriggered: a NEW incident cycle. Drop the old
		// state row so repeats/suppression reset; setState inserts fresh.
		if _, err := tx.Exec(`DELETE FROM delivery_state WHERE dedupe_key=$1`, in.Key); err != nil {
			return Outcome{}, err
		}
		hasRow = false
		lastState = ""
		repeatCount = 0
		suppressed = 0
	}
	if hasRow && lastState == "firing" {
		if ts.Sub(lastNotified) < cooldown {
			if err := setState("firing", false, ReasonCooldown, suppressed+1); err != nil {
				return Outcome{}, err
			}
			return Outcome{Action: "suppressed", Reason: ReasonCooldown}, nil
		}
		if repeatCount >= maxRep {
			if err := setState("firing", false, ReasonMaxRepeats, suppressed+1); err != nil {
				return Outcome{}, err
			}
			return Outcome{Action: "suppressed", Reason: ReasonMaxRepeats}, nil
		}
		if repeatSec > 0 && ts.Sub(lastNotified) < time.Duration(repeatSec)*time.Second {
			// Between cooldown expiry and repeat interval: hold quietly
			// (counted, not paged).
			if err := setState("firing", false, ReasonCooldown, suppressed+1); err != nil {
				return Outcome{}, err
			}
			return Outcome{Action: "suppressed", Reason: ReasonCooldown}, nil
		}
	}
	_ = lastReason

	// Digest-mode rules batch instead of paging immediately. Critical
	// incidents bypass the batch when the caller says so (engine passes
	// DigestMode=immediate for critical rules by default).
	if in.DigestMode == DigestDigest && in.Severity != SevCritical {
		if err := appendDigest(tx, in, ts); err != nil {
			return Outcome{}, err
		}
		if err := setState("firing", true, "", suppressed); err != nil {
			return Outcome{}, err
		}
		return Outcome{Action: "digested", Reason: ReasonDigestBatched}, nil
	}
	if err := enqueue(); err != nil {
		return Outcome{}, err
	}
	if err := setState("firing", true, "", suppressed); err != nil {
		return Outcome{}, err
	}
	return Outcome{Action: "queued"}, nil
}

// touchSuppressed records a policy suppression without a state claim
// (used before any incident state exists). Best-effort, never fatal.
func touchSuppressed(db *database.DB, in Input, reason string, ts time.Time) {
	_, _ = db.Exec(`INSERT INTO delivery_state
		(dedupe_key,severity,first_notified_at,last_notified_at,repeat_count,
		 last_state,suppressed_count,last_suppress_reason,updated_at)
		VALUES (?,?,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,0,'',1,?,CURRENT_TIMESTAMP)
		ON CONFLICT(dedupe_key) DO UPDATE SET
			suppressed_count=delivery_state.suppressed_count+1,
			last_suppress_reason=excluded.last_suppress_reason,
			updated_at=CURRENT_TIMESTAMP`,
		in.Key, in.Severity, reason)
	log(db, "suppressed %s (%s)", in.Key, reason)
}

// appendDigest adds one event to the current window batch (upsert).
func appendDigest(tx *sql.Tx, in Input, ts time.Time) error {
	interval := in.DigestIntervalMin
	if interval <= 0 {
		interval = 60
	}
	windowEnd := ts.Truncate(time.Duration(interval) * time.Minute).Add(time.Duration(interval) * time.Minute)
	key := digestKey(in)
	titles := truncate(in.Title, 200)
	res := in.Resource
	if res == "" {
		res = in.Key
	}
	res = truncate(res, 200)
	_, err := tx.Exec(`INSERT INTO digest_batches
		(digest_key,window_end,channel,group_id,severity,count,titles,resources,status,updated_at)
		VALUES ($1,$2,$3,$4,$5,1,$6,$7,'open',$8)
		ON CONFLICT (digest_key,window_end) DO UPDATE SET
			count=digest_batches.count+1,
			titles=SUBSTRING(digest_batches.titles || chr(10) || excluded.titles FOR 4000),
			resources=SUBSTRING(digest_batches.resources || ',' || excluded.resources FOR 2000),
			updated_at=excluded.updated_at
		WHERE digest_batches.status='open'`,
		key+"::"+in.Channels[0], windowEnd, in.Channels[0], in.GroupID, in.Severity, titles, res, ts)
	if err != nil {
		return err
	}
	// Fan out to remaining channels as sibling batches.
	for _, ch := range in.Channels[1:] {
		if _, err := tx.Exec(`INSERT INTO digest_batches
			(digest_key,window_end,channel,group_id,severity,count,titles,resources,status,updated_at)
			VALUES ($1,$2,$3,$4,$5,1,$6,$7,'open',$8)
			ON CONFLICT (digest_key,window_end) DO UPDATE SET
				count=digest_batches.count+1,
				titles=SUBSTRING(digest_batches.titles || chr(10) || excluded.titles FOR 4000),
				resources=SUBSTRING(digest_batches.resources || ',' || excluded.resources FOR 2000),
				updated_at=excluded.updated_at
			WHERE digest_batches.status='open'`,
			key+"::"+ch, windowEnd, ch, in.GroupID, in.Severity, titles, res, ts); err != nil {
			return err
		}
	}
	return nil
}

// digestKey groups by rule + configured label values.
func digestKey(in Input) string {
	base := fmt.Sprintf("rule:%d", in.RuleID)
	if len(in.GroupBy) == 0 {
		return base
	}
	var parts []string
	for _, k := range in.GroupBy {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		parts = append(parts, k+"="+in.Labels[k])
	}
	if len(parts) == 0 {
		return base
	}
	return base + "|" + strings.Join(parts, ",")
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "…(truncated)"
}

func log(db *database.DB, format string, args ...interface{}) {
	if db == nil || db.GDB == nil {
		return
	}
	applog.Info(db.GDB, "notify", fmt.Sprintf(format, args...))
}
