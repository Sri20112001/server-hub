package database_test

import (
	"testing"

	"serverhub/internal/applog"
	"serverhub/internal/audit"
	"serverhub/internal/testdb"
)

// Log tables must be append-only: INSERT works, UPDATE/DELETE are rejected
// by Postgres triggers.
func TestAppLogsImmutable(t *testing.T) {
	db := testdb.Open(t)

	if _, err := db.Exec(`INSERT INTO app_logs (timestamp, level, source, message) VALUES (CURRENT_TIMESTAMP, 'INFO', 'system', 'hello')`); err != nil {
		t.Fatalf("insert app_logs: %v", err)
	}
	if _, err := db.Exec(`UPDATE app_logs SET message='tampered' WHERE message='hello'`); err == nil {
		t.Fatal("UPDATE app_logs should be rejected")
	}
	if _, err := db.Exec(`DELETE FROM app_logs WHERE message='hello'`); err == nil {
		t.Fatal("DELETE app_logs should be rejected")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_logs WHERE message='hello'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("original app_logs row must survive, n=%d err=%v", n, err)
	}
}

func TestAuditLogsImmutable(t *testing.T) {
	db := testdb.Open(t)

	audit.Write(db, "tester", "login", "auth", "", "ok", "")
	if _, err := db.Exec(`UPDATE audit_logs SET result='tampered'`); err == nil {
		t.Fatal("UPDATE audit_logs should be rejected")
	}
	if _, err := db.Exec(`DELETE FROM audit_logs`); err == nil {
		t.Fatal("DELETE audit_logs should be rejected")
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM audit_logs`).Scan(&n); err != nil || n < 1 {
		t.Fatalf("audit row must survive, n=%d err=%v", n, err)
	}
}

// Every audit event must also land in the central app_logs store.
func TestAuditMirrorsToAppLogs(t *testing.T) {
	db := testdb.Open(t)

	var before int
	_ = db.QueryRow(`SELECT COUNT(*) FROM app_logs`).Scan(&before)
	audit.Write(db, "tester", "deploy", "project", "42", "ok", "sha123")
	var after int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_logs`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before+1 {
		t.Fatalf("audit event not mirrored to app_logs: before=%d after=%d", before, after)
	}
}

// History tables cannot be wiped; pipeline UPDATEs still work.
func TestHistoryTablesDeleteProtected(t *testing.T) {
	db := testdb.Open(t)

	deployID, err := db.InsertID(`INSERT INTO deployments (project_id,commit_sha,branch,trigger,status,started_at) VALUES (1,'abc','main','test','RUNNING',CURRENT_TIMESTAMP)`)
	if err != nil {
		t.Fatal(err)
	}
	// Legitimate pipeline progress (RUNNING -> SUCCESS) must keep working.
	if _, err := db.Exec(`UPDATE deployments SET status='SUCCESS', logs=? WHERE id=?`, "done", deployID); err != nil {
		t.Fatalf("deployment status update should be allowed: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM deployments WHERE id=?`, deployID); err == nil {
		t.Fatal("DELETE deployments should be rejected")
	}
	if _, err := db.Exec(`DELETE FROM deployments`); err == nil {
		t.Fatal("wipe deployments should be rejected")
	}

	opID := "op_test123"
	if _, err := db.Exec(`INSERT INTO operations (id,type,target_type,target_id,status,stages,initiated_by,created_at) VALUES (?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`,
		opID, "deploy", "project", "1", "QUEUED", "[]", "tester"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM operations WHERE id=?`, opID); err == nil {
		t.Fatal("DELETE operations should be rejected")
	}

	bid, err := db.InsertID(`INSERT INTO backups (project_id,kind,path,size_bytes,status,logs) VALUES (1,'snapshot','/tmp/x.tar.gz',10,'SUCCESS','logs')`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM backups WHERE id=?`, bid); err == nil {
		t.Fatal("DELETE backups should be rejected")
	}

	if _, err := db.Exec(`INSERT INTO server_snapshots (ts,cpu) VALUES (1234567890, 10)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM server_snapshots WHERE ts=1234567890`); err == nil {
		t.Fatal("DELETE server_snapshots should be rejected")
	}
}

// Prune must never delete: append-only logs are kept forever.
func TestPruneNeverDeletes(t *testing.T) {
	db := testdb.Open(t)

	applog.Write(db.GDB, applog.Entry{Level: "INFO", Source: "system", Message: "keep me"})
	n, err := applog.Prune(db.GDB, 30)
	if err != nil || n != 0 {
		t.Fatalf("Prune should be a no-op: n=%d err=%v", n, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM app_logs`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("Prune deleted logs: count=%d err=%v", count, err)
	}
}
