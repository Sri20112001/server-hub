package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

var schema = `
CREATE TABLE IF NOT EXISTS users (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	username TEXT NOT NULL UNIQUE,
	password_hash TEXT NOT NULL,
	role TEXT NOT NULL DEFAULT 'admin',
	created_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS projects (
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
);
CREATE TABLE IF NOT EXISTS services (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	type TEXT NOT NULL DEFAULT 'other',
	container_name TEXT NOT NULL DEFAULT '',
	internal_port INTEGER,
	host_port INTEGER,
	health_url TEXT NOT NULL DEFAULT '',
	docker_service_name TEXT NOT NULL DEFAULT '',
	status TEXT NOT NULL DEFAULT 'unknown',
	last_health TEXT NOT NULL DEFAULT 'UNKNOWN',
	last_health_at TEXT NOT NULL DEFAULT '',
	response_time_ms INTEGER,
	UNIQUE(project_id, name)
);
CREATE INDEX IF NOT EXISTS idx_services_project ON services(project_id);
CREATE TABLE IF NOT EXISTS deployments (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	commit_sha TEXT NOT NULL DEFAULT '',
	branch TEXT NOT NULL DEFAULT '',
	trigger TEXT NOT NULL DEFAULT 'manual',
	status TEXT NOT NULL DEFAULT 'PENDING',
	started_at TEXT NOT NULL DEFAULT (datetime('now')),
	completed_at TEXT NOT NULL DEFAULT '',
	duration_sec INTEGER,
	logs TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_deployments_project ON deployments(project_id);
CREATE TABLE IF NOT EXISTS secrets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	name TEXT NOT NULL,
	environment TEXT NOT NULL DEFAULT 'production',
	service_id INTEGER REFERENCES services(id) ON DELETE SET NULL,
	encrypted_value TEXT NOT NULL,
	nonce TEXT NOT NULL,
	updated_at TEXT NOT NULL DEFAULT (datetime('now')),
	UNIQUE(project_id, environment, name)
);
CREATE INDEX IF NOT EXISTS idx_secrets_project ON secrets(project_id);
CREATE TABLE IF NOT EXISTS gateway_routes (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	project_id INTEGER REFERENCES projects(id) ON DELETE SET NULL,
	host TEXT NOT NULL DEFAULT '',
	path_prefix TEXT NOT NULL,
	target TEXT NOT NULL,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL DEFAULT (datetime('now')),
	updated_at TEXT NOT NULL DEFAULT (datetime('now'))
);
CREATE TABLE IF NOT EXISTS audit_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	actor TEXT NOT NULL,
	action TEXT NOT NULL,
	resource TEXT NOT NULL,
	resource_id TEXT NOT NULL DEFAULT '',
	timestamp TEXT NOT NULL DEFAULT (datetime('now')),
	result TEXT NOT NULL DEFAULT 'ok',
	metadata TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_audit_time ON audit_logs(timestamp);
`

func Open(dbPath string) (*sql.DB, error) {
	dir := filepath.Dir(dbPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("mkdir data dir: %w", err)
		}
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA journal_mode=WAL;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec("PRAGMA foreign_keys=ON;"); err != nil {
		return nil, err
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	// Lightweight forward migrations for DBs created by older versions.
	for _, stmt := range []string{
		`ALTER TABLE projects ADD COLUMN auto_deploy INTEGER NOT NULL DEFAULT 0`,
		`CREATE TABLE IF NOT EXISTS server_snapshots (
			ts INTEGER NOT NULL PRIMARY KEY,
			cpu REAL NOT NULL DEFAULT 0,
			mem_pct REAL NOT NULL DEFAULT 0,
			mem_used_mb REAL NOT NULL DEFAULT 0,
			disk_pct REAL NOT NULL DEFAULT 0,
			net_rx BIGINT NOT NULL DEFAULT 0,
			net_tx BIGINT NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_snapshots_ts ON server_snapshots(ts)`,
		`ALTER TABLE server_snapshots ADD COLUMN disk_read BIGINT NOT NULL DEFAULT 0`,
		`ALTER TABLE server_snapshots ADD COLUMN disk_write BIGINT NOT NULL DEFAULT 0`,		`CREATE TABLE IF NOT EXISTS operations (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			target_type TEXT NOT NULL DEFAULT '',
			target_id TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL DEFAULT 'QUEUED',
			stage TEXT NOT NULL DEFAULT '',
			stages TEXT NOT NULL DEFAULT '[]',
			initiated_by TEXT NOT NULL DEFAULT '',
			error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			started_at TEXT NOT NULL DEFAULT '',
			completed_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_operations_time ON operations(created_at)`,
		`CREATE TABLE IF NOT EXISTS backups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
			kind TEXT NOT NULL DEFAULT 'snapshot',
			path TEXT NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL DEFAULT 'SUCCESS',
			created_at TEXT NOT NULL DEFAULT (datetime('now')),
			logs TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS idx_backups_project ON backups(project_id)`,
	} {
		_, _ = db.Exec(stmt) // ignore "duplicate column" errors
	}
	return db, nil
}
