package delivery

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"serverhub/internal/database"
)

// Retry policy: bounded attempts with exponential backoff + jitter.
const (
	MaxAttempts      = 5
	BaseRetryDelay   = 30 * time.Second
	CircuitThreshold = 5
	CircuitOpenFor   = 5 * time.Minute
	ClaimBatchSize   = 25
)

// Provider sends one message on a channel. Implementations must be
// synchronous and side-effect only (no policy decisions inside).
type Provider interface {
	SendEmail(groupID uint, subject, text string) error
	SendTelegram(text string) error
}

// StartLoop drains due outbox rows and flushes due digest batches until
// ctx is cancelled. Safe across instances: claims use FOR UPDATE SKIP
// LOCKED, so two workers never send the same row.
func StartLoop(ctx context.Context, db *database.DB, p Provider, interval time.Duration) {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		run := func() {
			if err := Drain(db, p); err != nil {
				log(db, "worker drain: %v", err)
			}
			if err := FlushDigests(db); err != nil {
				log(db, "worker digests: %v", err)
			}
		}
		run()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

type outboxRow struct {
	ID       uint
	Dedupe   string
	Severity string
	Channel  string
	GroupID  uint
	Title    string
	Body     string
	Attempts int
}

// Drain claims and sends due rows once. Returns the first error encountered
// (rows already handled are unaffected).
func Drain(db *database.DB, p Provider) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}
	pol := LoadPolicy(db)
	ts := now()
	rows, err := claimDue(db, ts)
	if err != nil {
		return err
	}
	var firstErr error
	for _, r := range rows {
		if err := sendOne(db, p, pol, r, ts); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// claimDue locks up to ClaimBatchSize due rows, marking them sending.
func claimDue(db *database.DB, ts time.Time) ([]outboxRow, error) {
	tx, err := db.SQL.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,dedupe_key,severity,channel,group_id,title,body,attempts
		FROM delivery_outbox
		WHERE status IN ('pending','deferred') AND next_retry_at <= $1
		ORDER BY next_retry_at ASC LIMIT $2
		FOR UPDATE SKIP LOCKED`, ts, ClaimBatchSize)
	if err != nil {
		return nil, err
	}
	var out []outboxRow
	for rows.Next() {
		var r outboxRow
		if err := rows.Scan(&r.ID, &r.Dedupe, &r.Severity, &r.Channel, &r.GroupID, &r.Title, &r.Body, &r.Attempts); err == nil {
			out = append(out, r)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, r := range out {
		if _, err := tx.Exec(`UPDATE delivery_outbox SET status='sending' WHERE id=$1`, r.ID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return out, nil
}

// sendOne delivers a claimed row: circuit breaker → rate budget → provider
// → retry/defer/failed bookkeeping. Never panics; always resolves the row
// out of 'sending' (crash between claim and resolve requeues via the
// status: a restart sweeps stale 'sending' rows back to pending).
func sendOne(db *database.DB, p Provider, pol Policy, r outboxRow, ts time.Time) error {
	fail := func(msg string, retry bool) {
		recordAttempt(db, r.ID, false, msg, 0)
		if retry && r.Attempts+1 < MaxAttempts {
			delay := backoff(r.Attempts)
			_, _ = db.Exec(`UPDATE delivery_outbox SET status='pending',
				attempts=attempts+1, last_error=?, next_retry_at=? WHERE id=?`,
				truncate(msg, 500), ts.Add(delay), r.ID)
			log(db, "retry %s #%d in %s: %s", r.Channel, r.Attempts+1, delay.Round(time.Second), msg)
			return
		}
		_, _ = db.Exec(`UPDATE delivery_outbox SET status='failed',
			attempts=attempts+1, last_error=? WHERE id=?`, truncate(msg, 500), r.ID)
		log(db, "failed %s %q: %s", r.Channel, r.Dedupe, msg)
	}

	// Circuit breaker.
	if until, ok := circuitOpen(db, r.Channel, ts); ok {
		_, _ = db.Exec(`UPDATE delivery_outbox SET status='deferred',
			last_error=?, next_retry_at=? WHERE id=?`,
			fmt.Sprintf("circuit open until %s", until.Format(time.RFC3339)), until, r.ID)
		return nil
	}
	// Rate budget: count sent in the trailing hour.
	budget := pol.BudgetFor(r.Channel)
	var sent int
	_ = db.QueryRow(`SELECT COUNT(*) FROM delivery_outbox
		WHERE channel=? AND status='sent' AND sent_at > ?`, r.Channel, ts.Add(-time.Hour)).Scan(&sent)
	if sent >= budget {
		deferAt := ts.Add(15 * time.Minute)
		_, _ = db.Exec(`UPDATE delivery_outbox SET status='deferred',
			last_error=?, next_retry_at=? WHERE id=?`,
			fmt.Sprintf("rate budget %d/h reached; coalesced into overflow summary", budget), deferAt, r.ID)
		recordOverflow(db, r, ts)
		log(db, "rate-limited %s (%d/h); overflow recorded", r.Channel, budget)
		return nil
	}

	// Provider send (timed for latency observability).
	start := time.Now()
	var serr error
	switch r.Channel {
	case ChEmail:
		serr = p.SendEmail(r.GroupID, r.Title, r.Body)
	case ChTelegram:
		serr = p.SendTelegram(r.Title + "\n" + r.Body)
	default:
		serr = fmt.Errorf("unsupported channel %q", r.Channel)
	}
	latency := time.Since(start).Milliseconds()
	if serr != nil {
		openCircuit(db, r.Channel, ts)
		fail(serr.Error(), true)
		return serr
	}
	closeCircuit(db, r.Channel, ts)
	recordAttempt(db, r.ID, true, "", latency)
	_, _ = db.Exec(`UPDATE delivery_outbox SET status='sent', sent_at=?,
		attempts=attempts+1, last_error='' WHERE id=?`, ts, r.ID)
	return nil
}

// backoff returns exponential delay + jitter: 30s, 60s, 120s, 240s (±25%).
func backoff(attempts int) time.Duration {
	d := BaseRetryDelay << attempts
	if d > 30*time.Minute {
		d = 30 * time.Minute
	}
	jitter := time.Duration(rand.Int63n(int64(d) / 2))
	return d/2 + jitter + time.Second
}

// circuitOpen reports whether the channel breaker is open now.
func circuitOpen(db *database.DB, channel string, ts time.Time) (time.Time, bool) {
	var until *time.Time
	_ = db.QueryRow(`SELECT opened_until FROM delivery_provider_state WHERE channel=?`, channel).Scan(&until)
	if until != nil && !until.IsZero() && ts.Before(*until) {
		return *until, true
	}
	return time.Time{}, false
}

func openCircuit(db *database.DB, channel string, ts time.Time) {
	var n int
	_ = db.QueryRow(`SELECT consecutive_failures FROM delivery_provider_state WHERE channel=?`, channel).Scan(&n)
	n++
	until := sql.NullTime{}
	if n >= CircuitThreshold {
		until = sql.NullTime{Time: ts.Add(CircuitOpenFor), Valid: true}
		log(db, "circuit open for %s (%d consecutive failures)", channel, n)
	}
	_, _ = db.Exec(`INSERT INTO delivery_provider_state (channel,consecutive_failures,opened_until,last_failure_at)
		VALUES (?,?,?,?) ON CONFLICT (channel) DO UPDATE SET
		consecutive_failures=excluded.consecutive_failures,
		opened_until=excluded.opened_until, last_failure_at=excluded.last_failure_at`,
		channel, n, nullTime(until), ts)
}

func closeCircuit(db *database.DB, channel string, ts time.Time) {
	_, _ = db.Exec(`INSERT INTO delivery_provider_state (channel,consecutive_failures,opened_until,last_success_at)
		VALUES (?,0,NULL,?) ON CONFLICT (channel) DO UPDATE SET
		consecutive_failures=0, opened_until=NULL, last_success_at=excluded.last_success_at`,
		channel, ts)
}

func nullTime(nt sql.NullTime) interface{} {
	if nt.Valid {
		return nt.Time
	}
	return nil
}

func recordAttempt(db *database.DB, outboxID uint, ok bool, errMsg string, latencyMs int64) {
	_, _ = db.Exec(`INSERT INTO delivery_attempts (outbox_id,ok,error,latency_ms,created_at)
		VALUES (?,?,?,?,CURRENT_TIMESTAMP)`, outboxID, ok, truncate(errMsg, 500), latencyMs)
}

// recordOverflow coalesces throttled events into a per-channel overflow
// digest so throttling never silently loses information.
func recordOverflow(db *database.DB, r outboxRow, ts time.Time) {
	windowEnd := ts.Truncate(15 * time.Minute).Add(15 * time.Minute)
	_, _ = db.Exec(`INSERT INTO digest_batches
		(digest_key,window_end,channel,group_id,severity,count,titles,resources,status,updated_at)
		VALUES (?,?,?,?,'WARNING',1,?,?,'open',?)
		ON CONFLICT (digest_key,window_end) DO UPDATE SET
			count=digest_batches.count+1,
			titles=SUBSTRING(digest_batches.titles || chr(10) || excluded.titles FOR 4000),
			updated_at=excluded.updated_at
		WHERE digest_batches.status='open'`,
		"overflow::"+r.Channel, windowEnd, r.Channel, r.GroupID,
		"throttled: "+truncate(r.Title, 150), r.Dedupe, ts)
}

// RequeueStale recovers rows stuck in 'sending' (worker crash between claim
// and resolve). Called at worker startup.
func RequeueStale(db *database.DB) (int64, error) {
	if db == nil {
		return 0, fmt.Errorf("nil database")
	}
	res, err := db.Exec(`UPDATE delivery_outbox SET status='pending'
		WHERE status='sending'`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	if n > 0 {
		log(db, "requeued %d stale sending row(s) after restart", n)
	}
	return n, nil
}

// FlushDigests sends due batches (window passed) as one summary per batch.
func FlushDigests(db *database.DB) error {
	if db == nil {
		return fmt.Errorf("nil database")
	}
	ts := now()
	tx, err := db.SQL.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT id,digest_key,channel,group_id,severity,count,titles,resources
		FROM digest_batches WHERE status='open' AND window_end <= $1
		ORDER BY window_end ASC LIMIT 25 FOR UPDATE SKIP LOCKED`, ts)
	if err != nil {
		return err
	}
	var batches []digestBatch
	for rows.Next() {
		var b digestBatch
		if err := rows.Scan(&b.id, &b.key, &b.channel, &b.group, &b.severity, &b.count, &b.titles, &b.resources); err == nil {
			batches = append(batches, b)
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, b := range batches {
		if _, err := tx.Exec(`UPDATE digest_batches SET status='flushed' WHERE id=$1`, b.id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	for _, b := range batches {
		title, body := b.message()
		_, _ = db.Exec(`INSERT INTO delivery_outbox
			(dedupe_key,severity,channel,group_id,title,body,status,attempts,next_retry_at)
			VALUES ($1,$2,$3,$4,$5,$6,'pending',0,$7)`,
			fmt.Sprintf("digest:%d:%d", b.id, ts.UnixNano()),
			b.severity, b.channel, b.group, title, body, ts)
		log(db, "digest flushed %s (%d events)", b.key, b.count)
	}
	return nil
}

// digestBatch is one due batch claimed by FlushDigests.
type digestBatch struct {
	id                            uint
	key, channel, severity, titles, resources string
	group                         uint
	count                         int
}

func (b digestBatch) message() (string, string) {
	title := fmt.Sprintf("[%s] %d grouped notifications", b.severity, b.count)
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d notification(s) grouped by %s:\n%s", b.count, b.key, b.titles)
	if b.resources != "" {
		fmt.Fprintf(&sb, "\nResources: %s", b.resources)
	}
	return title, truncate(sb.String(), 4000)
}
