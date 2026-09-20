package database

import (
	"database/sql"
	"testing"

	_ "github.com/glebarez/sqlite"
)

// Legacy DBs (created by the pre-GORM schema) must open without rebuilds:
// AutoMigrate recreated tables via a temp table and dropped NOT NULL columns,
// crashing boot with "NOT NULL constraint failed: users__temp.username".
func TestOpenLegacyDBPreservesData(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	raw, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	// Old-schema tables, as created by the pre-GORM db.go.
	for _, stmt := range []string{
		`CREATE TABLE users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			role TEXT NOT NULL DEFAULT 'admin',
			created_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE projects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			repository TEXT NOT NULL DEFAULT '',
			branch TEXT NOT NULL DEFAULT 'main',
			environment TEXT NOT NULL DEFAULT 'production',
			deployment_path TEXT NOT NULL DEFAULT '',
			compose_file TEXT NOT NULL DEFAULT 'docker-compose.yml',
			gateway_prefix TEXT NOT NULL DEFAULT '',
			health_url TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'unknown',
			auto_deploy INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			updated_at TEXT NOT NULL DEFAULT (datetime('now'))
		)`,
		`INSERT INTO users (username, password_hash, role) VALUES ('admin','hash','admin')`,
		`INSERT INTO projects (name) VALUES ('legacy-proj')`,
	} {
		if _, err := raw.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	raw.Close()

	db, err := OpenDatabase("", path)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer db.Close()

	var username string
	if err := db.QueryRow(`SELECT username FROM users WHERE id=1`).Scan(&username); err != nil {
		t.Fatalf("legacy user missing: %v", err)
	}
	if username != "admin" {
		t.Fatalf("legacy user corrupted: %q", username)
	}
	var proj string
	if err := db.QueryRow(`SELECT name FROM projects WHERE id=1`).Scan(&proj); err != nil {
		t.Fatalf("legacy project missing: %v", err)
	}
	// New aggregator table must exist.
	var tbl string
	if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name='app_logs'`).Scan(&tbl); err != nil {
		t.Fatalf("app_logs missing: %v", err)
	}
	id, err := db.InsertID(`INSERT INTO projects (name) VALUES ('new-proj')`)
	if err != nil || id != 2 {
		t.Fatalf("insert on legacy db: id=%d err=%v", id, err)
	}
}

func TestOpenFreshDBCreatesAllTables(t *testing.T) {
	db, err := OpenDatabase("", t.TempDir()+"/fresh.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, tbl := range []string{"users", "projects", "services", "deployments",
		"secrets", "gateway_routes", "audit_logs", "operations", "backups",
		"server_snapshots", "app_logs"} {
		var name string
		if err := db.QueryRow(`SELECT name FROM sqlite_master WHERE name=?`, tbl).Scan(&name); err != nil {
			t.Fatalf("table %s missing: %v", tbl, err)
		}
	}
}
