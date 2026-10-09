package retention

import (
	"testing"
	"time"

	"serverhub/internal/database"
	"serverhub/internal/testdb"
)

func insertAppLog(t *testing.T, db *database.DB, ts time.Time, msg string) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO app_logs (timestamp, level, source, message) VALUES (?,?,?,?)`,
		ts.UTC().Format("2006-01-02 15:04:05.000000"), "INFO", "system", msg); err != nil {
		t.Fatal(err)
	}
}

func countAppLogs(t *testing.T, db *database.DB) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_logs`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAppLogsPruneKeepsRecentDeletesOld(t *testing.T) {
	db := testdb.Open(t)
	now := time.Now().UTC()
	insertAppLog(t, db, now.Add(-100*24*time.Hour), "old")
	insertAppLog(t, db, now.Add(-24*time.Hour), "recent")

	deleted, err := CleanupAppLogs(db, 90)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deletion, got %d", deleted)
	}
	if got := countAppLogs(t, db); got != 1 {
		t.Fatalf("expected 1 retained row, got %d", got)
	}
	var msg string
	if err := db.QueryRow(`SELECT message FROM app_logs`).Scan(&msg); err != nil || msg != "recent" {
		t.Fatalf("retained row = %q, want recent", msg)
	}
}

func TestAppLogsTriggerStillGuardsRecent(t *testing.T) {
	db := testdb.Open(t)
	insertAppLog(t, db, time.Now().UTC(), "fresh")
	// Recent DELETE must be rejected by the trigger.
	if _, err := db.Exec(`DELETE FROM app_logs WHERE message='fresh'`); err == nil {
		t.Fatal("DELETE of recent app_logs row should be rejected")
	}
	// UPDATE of any row must be rejected.
	if _, err := db.Exec(`UPDATE app_logs SET message='tampered' WHERE message='fresh'`); err == nil {
		t.Fatal("UPDATE app_logs should be rejected")
	}
	if got := countAppLogs(t, db); got != 1 {
		t.Fatalf("fresh row must survive, n=%d", got)
	}
}

func TestAuditLogsNeverPruned(t *testing.T) {
	db := testdb.Open(t)
	// Even an ancient audit row must survive: no prune job touches
	// audit_logs and its trigger has no age exception.
	if _, err := db.Exec(`INSERT INTO audit_logs (actor, action, resource, resource_id, result, metadata, timestamp)
		VALUES ('op','login','auth','','ok','', CURRENT_TIMESTAMP - INTERVAL '400 days')`); err != nil {
		t.Fatal(err)
	}
	if _, err := CleanupAppLogs(db, 90); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("audit row must survive pruning, n=%d err=%v", n, err)
	}
	if _, err := db.Exec(`DELETE FROM audit_logs`); err == nil {
		t.Fatal("DELETE audit_logs should be rejected")
	}
}

func TestDeliveryRetentionPrunesTerminalOnly(t *testing.T) {
	db := testdb.Open(t)
	old := time.Now().UTC().Add(-100 * 24 * time.Hour)
	new_ := time.Now().UTC().Add(-time.Hour)
	mk := func(key, status string, ts time.Time) {
		t.Helper()
		if _, err := db.Exec(`INSERT INTO delivery_outbox
			(dedupe_key,severity,channel,group_id,title,body,status,attempts,next_retry_at,created_at)
			VALUES (?,?,?,?,?,?,?,0,?,?)`,
			key, "WARNING", "EMAIL", 0, "t", "b", status, ts, ts); err != nil {
			t.Fatal(err)
		}
	}
	mk("old-sent", "sent", old)
	mk("old-pending", "pending", old) // live queue: must survive
	mk("new-sent", "sent", new_)
	mk("old-failed", "failed", old)
	if _, err := db.Exec(`INSERT INTO delivery_attempts (outbox_id,ok,error,latency_ms,created_at)
		VALUES (1,true,'',5,$1)`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO digest_batches
		(digest_key,window_end,channel,count,status,created_at,updated_at)
		VALUES ('k',$1,'EMAIL',2,'flushed',NOW(),NOW())`, old); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO delivery_state
		(dedupe_key,severity,first_notified_at,last_notified_at,repeat_count,
		 last_state,suppressed_count,updated_at)
		VALUES ('old-firing','WARNING',$1,$1,0,'firing',0,$1)`, old); err != nil {
		t.Fatal(err)
	}
	deleted, err := CleanupDeliveries(db, 90)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 4 { // old-sent, old-failed, attempt, flushed digest
		t.Fatalf("deleted = %d, want 4", deleted)
	}
	// Firing state + pending queue survive regardless of age.
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM delivery_outbox WHERE dedupe_key='old-pending'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("pending queue must survive, n=%d", n)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM delivery_state WHERE dedupe_key='old-firing'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("firing state must survive, n=%d", n)
	}
	var audits int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	_ = audits
}

func TestHealthResultsPrune(t *testing.T) {
	db := testdb.Open(t)
	hcID, err := db.InsertID(`INSERT INTO health_checks (name,type,target,interval,timeout,expected_status,enabled,status)
		VALUES ('prune-test','http','http://localhost:9/x',60,10,200,true,'UNKNOWN')`)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	old := now.Add(-40 * 24 * time.Hour).Format("2006-01-02 15:04:05.000000")
	new_ := now.Add(-24 * time.Hour).Format("2006-01-02 15:04:05.000000")
	if _, err := db.Exec(`INSERT INTO health_check_results (health_check_id,timestamp,status,response_time_ms,error)
		VALUES (?,?, 'DOWN', 5, ''), (?,?, 'UP', 3, '')`, hcID, old, hcID, new_); err != nil {
		t.Fatal(err)
	}
	deleted, err := CleanupHealthCheckResults(db, 30)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deletion, got %d", deleted)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM health_check_results`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("expected 1 retained result, n=%d", n)
	}
}
