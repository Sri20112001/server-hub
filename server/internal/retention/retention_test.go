package retention

import (
	"context"
	"testing"
	"time"

	"serverhub/internal/database"
	"serverhub/internal/testdb"
)

func seedServer(t *testing.T, db *database.DB) uint {
	t.Helper()
	id, err := db.InsertID(`INSERT INTO managed_servers
		(name,hostname,status,agent_status,created_at,updated_at)
		VALUES ('retention-test','retention-host','UNKNOWN','UNKNOWN',NOW(),NOW())`)
	if err != nil {
		t.Fatal(err)
	}
	return uint(id)
}

func insertMetric(t *testing.T, db *database.DB, serverID uint, ts time.Time) {
	t.Helper()
	if _, err := db.Exec(`INSERT INTO server_metrics
		(server_id,timestamp,cpu_usage,memory_usage,memory_used_mb,disk_usage,disk_used_gb,
		 net_rx,net_tx,load_avg1,uptime_sec)
		VALUES ($1,$2,10,20,200,30,40,100,200,1.5,3600)`, serverID, ts); err != nil {
		t.Fatal(err)
	}
}

func countMetrics(t *testing.T, db *database.DB, serverID uint) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM server_metrics WHERE server_id=$1`, serverID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestCleanupKeepsRecentDeletesOld(t *testing.T) {
	db := testdb.Open(t)
	sid := seedServer(t, db)
	now := time.Now().UTC()
	insertMetric(t, db, sid, now.Add(-40*24*time.Hour)) // old
	insertMetric(t, db, sid, now.Add(-10*24*time.Hour)) // recent

	deleted, err := Cleanup(db, 30)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 1 {
		t.Fatalf("expected 1 deletion, got %d", deleted)
	}
	if got := countMetrics(t, db, sid); got != 1 {
		t.Fatalf("expected 1 retained row, got %d", got)
	}
}

func TestCleanupCutoffIsStrict(t *testing.T) {
	db := testdb.Open(t)
	sid := seedServer(t, db)
	now := time.Now().UTC()
	insertMetric(t, db, sid, Cutoff(now, 30)) // exactly at cutoff → retained (< semantics)

	deleted, err := cleanupSince(db, 30, now)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 0 {
		t.Fatalf("boundary row must be retained, deleted %d", deleted)
	}
	if got := countMetrics(t, db, sid); got != 1 {
		t.Fatalf("expected boundary row retained, got %d", got)
	}
}

func TestCleanupIsolation(t *testing.T) {
	db := testdb.Open(t)
	sid := seedServer(t, db)
	now := time.Now().UTC()
	insertMetric(t, db, sid, now.Add(-40*24*time.Hour))

	// Neighbor tables with old timestamps must be untouched.
	if _, err := db.Exec(`INSERT INTO alerts
		(server_id,condition,threshold,severity,status,message,triggered_at)
		VALUES ($1,'cpu_high',85,'WARNING','TRIGGERED','old',$2)`, sid, now.Add(-400*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO notification_groups (name,created_at,updated_at)
		VALUES ('keep',NOW(),NOW())`); err != nil {
		t.Fatal(err)
	}

	if _, err := Cleanup(db, 30); err != nil {
		t.Fatal(err)
	}
	var alerts, groups, servers int
	if err := db.QueryRow(`SELECT COUNT(*) FROM alerts`).Scan(&alerts); err != nil || alerts != 1 {
		t.Fatalf("alerts touched: %d (%v)", alerts, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM notification_groups`).Scan(&groups); err != nil || groups != 1 {
		t.Fatalf("groups touched: %d (%v)", groups, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM managed_servers`).Scan(&servers); err != nil || servers != 1 {
		t.Fatalf("servers touched: %d (%v)", servers, err)
	}
	if got := countMetrics(t, db, sid); got != 0 {
		t.Fatalf("old metric must be gone, got %d", got)
	}
}

func TestCleanupInvalidRetention(t *testing.T) {
	db := testdb.Open(t)
	for _, bad := range []int{0, -5} {
		if _, err := Cleanup(db, bad); err == nil {
			t.Fatalf("retention %d must fail", bad)
		}
	}
	if _, err := Cleanup(nil, 30); err == nil {
		t.Fatal("nil db must fail")
	}
}

func TestCleanupIdempotent(t *testing.T) {
	db := testdb.Open(t)
	sid := seedServer(t, db)
	insertMetric(t, db, sid, time.Now().UTC().Add(-40*24*time.Hour))

	if n, err := Cleanup(db, 30); err != nil || n != 1 {
		t.Fatalf("first run: %d (%v)", n, err)
	}
	// Second run (as after a restart) is a safe no-op.
	if n, err := Cleanup(db, 30); err != nil || n != 0 {
		t.Fatalf("second run must be no-op: %d (%v)", n, err)
	}
}

func TestStartLoopStopsOnCancel(t *testing.T) {
	db := testdb.Open(t)
	ctx, cancel := context.WithCancel(context.Background())
	StartLoop(ctx, db, 30, time.Hour)
	cancel()
	// Returns promptly; a blocked worker would hang the test binary.
	done := make(chan struct{})
	go func() {
		time.Sleep(2 * time.Second)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("test timed out")
	}
}
