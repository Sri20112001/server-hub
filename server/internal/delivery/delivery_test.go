package delivery

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"serverhub/internal/database"
	"serverhub/internal/testdb"
)

// fakeProvider records sends and fails on demand (no network).
type fakeProvider struct {
	mu       sync.Mutex
	sent     []string
	failNext int
	failWith error
}

func (f *fakeProvider) SendEmail(groupID uint, subject, text string) error {
	return f.send("EMAIL:" + subject)
}

func (f *fakeProvider) SendTelegram(text string) error {
	return f.send("TELEGRAM")
}

func (f *fakeProvider) send(tag string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext > 0 {
		f.failNext--
		if f.failWith != nil {
			return f.failWith
		}
		return fmt.Errorf("provider down")
	}
	f.sent = append(f.sent, tag)
	return nil
}

func (f *fakeProvider) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.sent)
}

func baseInput(key string) Input {
	return Input{
		Key: key, Severity: "CRITICAL", Title: "t-" + key, Body: "b",
		Channels: []string{ChEmail}, GroupID: 1,
		CooldownSec: 3600, RepeatSec: 3600, MaxRepeats: 3,
	}
}

func outboxCount(t *testing.T, db *database.DB, status string) int {
	t.Helper()
	var n int
	q := `SELECT COUNT(*) FROM delivery_outbox`
	if status != "" {
		q += ` WHERE status='` + status + `'`
	}
	if err := db.QueryRow(q).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// 1. Repeated delivery of the same firing alert sends one initial notification.
func TestDedupeFirstOnly(t *testing.T) {
	db := testdb.Open(t)
	for i := 0; i < 5; i++ {
		out, err := Dispatch(db, baseInput("alert-a"))
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 && out.Action != "queued" {
			t.Fatalf("first: want queued, got %v", out)
		}
		if i > 0 && out.Action != "suppressed" {
			t.Fatalf("repeat %d: want suppressed, got %v", i, out)
		}
	}
	if got := outboxCount(t, db, "pending"); got != 1 {
		t.Fatalf("want 1 queued row, got %d", got)
	}
}

// 2. Concurrent workers cannot duplicate one event.
func TestConcurrentDispatchSingleEnqueue(t *testing.T) {
	db := testdb.Open(t)
	var wg sync.WaitGroup
	actions := make([]string, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			out, err := Dispatch(db, baseInput("race-key"))
			if err != nil {
				actions[i] = "error"
				return
			}
			actions[i] = out.Action
		}(i)
	}
	wg.Wait()
	queued := 0
	for _, a := range actions {
		if a == "queued" {
			queued++
		}
	}
	if queued != 1 {
		t.Fatalf("want exactly 1 queued, got %d (%v)", queued, actions)
	}
	if got := outboxCount(t, db, "pending"); got != 1 {
		t.Fatalf("want 1 outbox row, got %d", got)
	}
}

// 3. Different incidents are not deduplicated.
func TestDistinctIncidentsSeparate(t *testing.T) {
	db := testdb.Open(t)
	for _, k := range []string{"inc-1", "inc-2"} {
		if out, err := Dispatch(db, baseInput(k)); err != nil || out.Action != "queued" {
			t.Fatalf("%s: %v %v", k, out, err)
		}
	}
	if got := outboxCount(t, db, "pending"); got != 2 {
		t.Fatalf("want 2 rows, got %d", got)
	}
}

// 4. Cooldowns suppress until expiry; 5. critical immediate; repeats capped.
func TestCooldownAndRepeats(t *testing.T) {
	db := testdb.Open(t)
	NowFunc = func() time.Time { return time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC) }
	defer func() { NowFunc = nil }()
	in := baseInput("cool-1")
	in.CooldownSec = 60
	in.RepeatSec = 60
	in.MaxRepeats = 1
	if out, _ := Dispatch(db, in); out.Action != "queued" {
		t.Fatalf("first: %v", out)
	}
	if out, _ := Dispatch(db, in); out.Reason != ReasonCooldown {
		t.Fatalf("within cooldown: %v", out)
	}
	// Past cooldown + repeat interval: one repeat allowed...
	NowFunc = func() time.Time { return time.Date(2026, 1, 1, 12, 2, 0, 0, time.UTC) }
	if out, _ := Dispatch(db, in); out.Action != "queued" {
		t.Fatalf("after interval: %v", out)
	}
	// ...then max repeats suppresses.
	NowFunc = func() time.Time { return time.Date(2026, 1, 1, 12, 4, 0, 0, time.UTC) }
	if out, _ := Dispatch(db, in); out.Reason != ReasonMaxRepeats {
		t.Fatalf("over max: %v", out)
	}
}

// 8. Resolution notifies once when enabled; muted without prior firing.
func TestRecoveryPolicy(t *testing.T) {
	db := testdb.Open(t)
	rec := baseInput("rec-1")
	rec.IsRecovery = true
	rec.NotifyRecovery = true
	if out, _ := Dispatch(db, rec); out.Reason != ReasonRecoveryMuted {
		t.Fatalf("recovery without firing: %v", out)
	}
	fire := baseInput("rec-1")
	if out, _ := Dispatch(db, fire); out.Action != "queued" {
		t.Fatalf("fire: %v", out)
	}
	if out, _ := Dispatch(db, rec); out.Action != "queued" {
		t.Fatalf("recovery after firing: %v", out)
	}
	if out, _ := Dispatch(db, rec); out.Reason != ReasonRecoveryMuted {
		t.Fatalf("second recovery: %v", out)
	}
	// Resolved-then-retriggered starts a new cycle.
	if out, _ := Dispatch(db, fire); out.Action != "queued" {
		t.Fatalf("retrigger after resolve: %v", out)
	}
}

// 6+7. Grouped alerts produce one digest with counts.
func TestDigestBatching(t *testing.T) {
	db := testdb.Open(t)
	for i := 0; i < 3; i++ {
		in := baseInput(fmt.Sprintf("dig-%d", i))
		in.Severity = "WARNING"
		in.DigestMode = DigestDigest
		in.DigestIntervalMin = 60
		in.GroupBy = []string{"severity"}
		in.Labels = map[string]string{"severity": "warning"}
		in.RuleID = 9
		if out, err := Dispatch(db, in); err != nil || out.Action != "digested" {
			t.Fatalf("event %d: %v %v", i, out, err)
		}
	}
	var batches int
	if err := db.QueryRow(`SELECT COUNT(*) FROM digest_batches WHERE status='open'`).Scan(&batches); err != nil {
		t.Fatal(err)
	}
	if batches != 1 {
		t.Fatalf("want 1 grouped batch, got %d", batches)
	}
	var count int
	if err := db.QueryRow(`SELECT count FROM digest_batches`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("batch count = %d, want 3", count)
	}
	if got := outboxCount(t, db, "pending"); got != 0 {
		t.Fatalf("digest must not page immediately, got %d rows", got)
	}
	// Critical bypasses digest batching.
	in := baseInput("dig-crit")
	in.DigestMode = DigestDigest
	if out, _ := Dispatch(db, in); out.Action != "queued" {
		t.Fatalf("critical digest-mode: %v", out)
	}
}

// 9. Quiet hours suppress (non-critical), critical bypass per config.
func TestQuietHours(t *testing.T) {
	db := testdb.Open(t)
	gid, err := db.InsertID(`INSERT INTO notification_groups
		(name,quiet_start,quiet_end,quiet_tz,quiet_allow_critical,created_at,updated_at)
		VALUES ('qh','00:00','23:59','UTC',true,NOW(),NOW())`)
	if err != nil {
		t.Fatal(err)
	}
	NowFunc = func() time.Time { return time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC) }
	defer func() { NowFunc = nil }()
	warn := baseInput("qh-warn")
	warn.Severity = "WARNING"
	warn.GroupID = uint(gid)
	if out, _ := Dispatch(db, warn); out.Reason != ReasonQuietHours {
		t.Fatalf("warning in quiet: %v", out)
	}
	crit := baseInput("qh-crit")
	crit.GroupID = uint(gid)
	if out, _ := Dispatch(db, crit); out.Action != "queued" {
		t.Fatalf("critical bypass: %v", out)
	}
}

// Emergency pause suppresses everything; expiry lifts it.
func TestEmergencyPause(t *testing.T) {
	db := testdb.Open(t)
	if _, err := db.Exec(`UPDATE notification_policy SET emergency_pause=true,
		pause_until=$1, pause_reason='drill' WHERE id=1`,
		time.Now().UTC().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if out, _ := Dispatch(db, baseInput("paused-1")); out.Reason != ReasonEmergencyPause {
		t.Fatalf("paused: %v", out)
	}
	if _, err := db.Exec(`UPDATE notification_policy SET pause_until=$1 WHERE id=1`,
		time.Now().UTC().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if out, _ := Dispatch(db, baseInput("paused-2")); out.Action != "queued" {
		t.Fatalf("expired pause must lift: %v", out)
	}
	if _, err := db.Exec(`UPDATE notification_policy SET emergency_pause=false WHERE id=1`); err != nil {
		t.Fatal(err)
	}
}

// Maintenance windows suppress; scope matching works.
func TestMaintenanceWindow(t *testing.T) {
	db := testdb.Open(t)
	now := time.Now().UTC()
	if _, err := db.Exec(`INSERT INTO maintenance_windows
		(name,scope,starts_at,ends_at,reason,enabled,created_by,created_at,updated_at)
		VALUES ('deploy-freeze','all',$1,$2,'drill',true,'test',NOW(),NOW())`,
		now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if out, _ := Dispatch(db, baseInput("mw-1")); out.Reason != ReasonMaintenance {
		t.Fatalf("maintenance: %v", out)
	}
}

// 10. Rate limits defer + overflow summary; retries; circuit breaker.
func TestRateLimitAndRetryAndCircuit(t *testing.T) {
	db := testdb.Open(t)
	if _, err := db.Exec(`UPDATE notification_policy SET email_per_hour=1 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	p := &fakeProvider{}
	if out, _ := Dispatch(db, baseInput("rl-1")); out.Action != "queued" {
		t.Fatalf("first: %v", out)
	}
	if err := Drain(db, p); err != nil {
		t.Fatal(err)
	}
	if p.count() != 1 {
		t.Fatalf("sent = %d, want 1", p.count())
	}
	if out, _ := Dispatch(db, baseInput("rl-2")); out.Action != "queued" {
		t.Fatalf("second (budget spent at dispatch? no — dispatch always queues): %v", out)
	}
	if err := Drain(db, p); err != nil {
		t.Fatal(err)
	}
	if p.count() != 1 {
		t.Fatalf("rate limit must hold at 1, sent = %d", p.count())
	}
	var deferred int
	if err := db.QueryRow(`SELECT COUNT(*) FROM delivery_outbox WHERE status='deferred'`).Scan(&deferred); err != nil || deferred != 1 {
		t.Fatalf("want 1 deferred row, got %d", deferred)
	}
	var overflow int
	if err := db.QueryRow(`SELECT COUNT(*) FROM digest_batches WHERE digest_key LIKE 'overflow::%'`).Scan(&overflow); err != nil || overflow != 1 {
		t.Fatalf("want overflow summary batch, got %d", overflow)
	}

	// Retry with backoff then failure recorded; circuit opens after 5.
	if _, err := db.Exec(`UPDATE notification_policy SET email_per_hour=1000 WHERE id=1`); err != nil {
		t.Fatal(err)
	}
	pf := &fakeProvider{failNext: 1000, failWith: fmt.Errorf("smtp down")}
	// Pin the clock BEFORE enqueue so backoff windows advance deterministically.
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	NowFunc = func() time.Time { return clock }
	defer func() { NowFunc = nil }()
	if _, err := Dispatch(db, baseInput("cb-1")); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxAttempts; i++ {
		clock = clock.Add(31 * time.Minute)
		_ = Drain(db, pf)
	}
	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status,attempts FROM delivery_outbox WHERE dedupe_key='cb-1'`).Scan(&status, &attempts); err != nil {
		t.Fatal(err)
	}
	if status != "failed" || attempts != MaxAttempts {
		t.Fatalf("status=%s attempts=%d, want failed/%d", status, attempts, MaxAttempts)
	}
	var opened string
	if err := db.QueryRow(`SELECT opened_until FROM delivery_provider_state WHERE channel='EMAIL'`).Scan(&opened); err != nil {
		t.Fatalf("circuit should be open: %v", err)
	}
	var attCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM delivery_attempts`).Scan(&attCount); err != nil || attCount < MaxAttempts {
		t.Fatalf("attempts logged = %d, want >= %d", attCount, MaxAttempts)
	}
}

// Worker marks sent rows; stale sending rows requeue on restart.
func TestSendMarksSentAndRequeue(t *testing.T) {
	db := testdb.Open(t)
	p := &fakeProvider{}
	if _, err := Dispatch(db, baseInput("sent-1")); err != nil {
		t.Fatal(err)
	}
	if err := Drain(db, p); err != nil {
		t.Fatal(err)
	}
	if got := outboxCount(t, db, "sent"); got != 1 {
		t.Fatalf("want 1 sent, got %d", got)
	}
	if _, err := db.Exec(`UPDATE delivery_outbox SET status='sending' WHERE dedupe_key='sent-1'`); err != nil {
		t.Fatal(err)
	}
	n, err := RequeueStale(db)
	if err != nil || n != 1 {
		t.Fatalf("requeue = %d, %v", n, err)
	}
}

// 13. Policy + state persist (restart = new process, same DB).
func TestStatePersistsAcrossRestarts(t *testing.T) {
	db := testdb.Open(t)
	if _, err := Dispatch(db, baseInput("persist-1")); err != nil {
		t.Fatal(err)
	}
	var repeats int
	var state string
	if err := db.QueryRow(`SELECT repeat_count,last_state FROM delivery_state WHERE dedupe_key='persist-1'`).
		Scan(&repeats, &state); err != nil || state != "firing" {
		t.Fatalf("state = %d/%q", repeats, state)
	}
}
