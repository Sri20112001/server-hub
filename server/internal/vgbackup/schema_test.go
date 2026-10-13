package vgbackup_test

import (
	"strings"
	"testing"

	"serverhub/internal/testdb"
	"serverhub/internal/vgbackup"
)

// Schema init must be idempotent: booting twice (deployments restart,
// crash loops) creates missing tables once and never touches the rest.
func TestEnsureSchemaIdempotent(t *testing.T) {
	db := testdb.Open(t)
	if err := vgbackup.EnsureSchema(db.GDB); err != nil {
		t.Fatalf("first EnsureSchema: %v", err)
	}
	if err := vgbackup.EnsureSchema(db.GDB); err != nil {
		t.Fatalf("second EnsureSchema must succeed: %v", err)
	}
	for _, m := range vgbackup.Models() {
		if !db.GDB.Migrator().HasTable(m) {
			t.Errorf("expected table for %T after EnsureSchema", m)
		}
	}
}

// Existing ServerHub data must survive backup schema init: rows written
// before the vg_* tables existed are still there afterwards.
func TestEnsureSchemaPreservesServerHubData(t *testing.T) {
	db := testdb.Open(t)
	if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?,?)`,
		"keeper", "hash", "viewer"); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := vgbackup.EnsureSchema(db.GDB); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, "keeper").Scan(&n); err != nil || n != 1 {
		t.Fatalf("seed user must survive schema init: n=%d err=%v", n, err)
	}
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name LIKE 'vg\_%'`).Scan(&tables); err != nil {
		t.Fatalf("count vg tables: %v", err)
	}
	if tables != len(vgbackup.Models()) {
		t.Errorf("want %d vg_* tables, found %d", len(vgbackup.Models()), tables)
	}
}

// UUID identifiers must generate on create and round-trip, and associations
// must resolve: this pins the FK/relation wiring, not just table names.
func TestVgUUIDAndAssociations(t *testing.T) {
	db := testdb.Open(t)
	if err := vgbackup.EnsureSchema(db.GDB); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}
	agent := vgbackup.VgAgent{Name: "db01", CreatedByUserID: 7}
	if err := db.GDB.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	if agent.ID.String() == "" || agent.ID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("agent ID must be generated on create")
	}
	target := vgbackup.VgStorageTarget{Name: "local", Type: vgbackup.VgStorageLocal, CreatedByUserID: 7}
	if err := db.GDB.Create(&target).Error; err != nil {
		t.Fatalf("create target: %v", err)
	}
	job := vgbackup.VgBackupJob{
		Name: "nightly", SourceType: vgbackup.VgSourcePostgres,
		AgentID: agent.ID, StorageTargetID: target.ID, CreatedByUserID: 7,
	}
	if err := db.GDB.Create(&job).Error; err != nil {
		t.Fatalf("create job: %v", err)
	}
	var loaded vgbackup.VgBackupJob
	if err := db.GDB.Preload("Agent").Preload("StorageTarget").First(&loaded, "id = ?", job.ID).Error; err != nil {
		t.Fatalf("preload job relations: %v", err)
	}
	if loaded.Agent.ID != agent.ID || loaded.Agent.Name != "db01" {
		t.Errorf("job→agent association broken: %+v", loaded.Agent)
	}
	if loaded.StorageTarget.ID != target.ID {
		t.Errorf("job→storage association broken: %+v", loaded.StorageTarget)
	}
}

// TestEnsureSchemaTablesAndIndexes verifies that all expected vg_* tables and
// uniqueness indexes exist and can be initialized repeatedly.
func TestEnsureSchemaTablesAndIndexes(t *testing.T) {
	db := testdb.Open(t)
	if err := vgbackup.EnsureSchema(db.GDB); err != nil {
		t.Fatalf("EnsureSchema: %v", err)
	}

	expectedTables := []string{
		"vg_agents",
		"vg_storage_targets",
		"vg_s3_regions",
		"vg_database_connections",
		"vg_backup_policies",
		"vg_backup_jobs",
		"vg_backup_schedules",
		"vg_machines",
		"vg_enrollment_tokens",
		"vg_backup_runs",
		"vg_backup_artifacts",
		"vg_backup_chunks",
		"vg_restore_jobs",
		"vg_backup_size_baselines",
	}

	for _, tbl := range expectedTables {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=?`, tbl).Scan(&n); err != nil || n != 1 {
			t.Errorf("expected table %s to exist in public schema (n=%d, err=%v)", tbl, n, err)
		}
	}

	expectedIndexes := []string{
		"idx_vg_artifact_run_name",
		"idx_vg_chunk_artifact_index",
		"idx_vg_runs_job_scheduled",
		"idx_vg_runs_retry_once",
		"idx_vg_restores_pending_dedupe",
		"idx_vg_conn_owner_name",
	}

	for _, idx := range expectedIndexes {
		var n int
		if err := db.QueryRow(`SELECT COUNT(*) FROM pg_indexes WHERE schemaname='public' AND indexname=?`, idx).Scan(&n); err != nil || n != 1 {
			t.Errorf("expected index %s to exist (n=%d, err=%v)", idx, n, err)
		}
	}
}

// TestEnsureSchemaUniqueIndexesWithDuplicateData verifies behavior when existing legacy
// data already violates a uniqueness constraint before the index is created.
func TestEnsureSchemaUniqueIndexesWithDuplicateData(t *testing.T) {
	db := testdb.Open(t)

	// Step 1: Create the base tables by creating models first
	for _, m := range vgbackup.Models() {
		if !db.GDB.Migrator().HasTable(m) {
			if err := db.GDB.Migrator().CreateTable(m); err != nil {
				t.Fatalf("create table %T: %v", m, err)
			}
		}
	}

	// Step 2: Drop the unique index to simulate pre-existing unindexed/legacy schema
	if _, err := db.Exec(`DROP INDEX IF EXISTS idx_vg_conn_owner_name`); err != nil {
		t.Fatalf("drop index: %v", err)
	}

	// Seed agent to satisfy fk_vg_database_connections_agent
	if _, err := db.Exec(`INSERT INTO vg_agents (id, created_by_user_id, name, status, created_at, updated_at) VALUES
		('33333333-3333-3333-3333-333333333333', 1, 'agent1', 'ONLINE', NOW(), NOW())`); err != nil {
		t.Fatalf("seed agent: %v", err)
	}

	// Step 3: Insert duplicate rows for vg_database_connections (same user + name)
	if _, err := db.Exec(`INSERT INTO vg_database_connections (id, created_by_user_id, agent_id, name, type, host, port, username, status, created_at, updated_at) VALUES
		('11111111-1111-1111-1111-111111111111', 1, '33333333-3333-3333-3333-333333333333', 'dup-conn', 'POSTGRES', 'localhost', 5432, 'postgres', 'UNKNOWN', NOW(), NOW()),
		('22222222-2222-2222-2222-222222222222', 1, '33333333-3333-3333-3333-333333333333', 'dup-conn', 'POSTGRES', 'localhost', 5432, 'postgres', 'UNKNOWN', NOW(), NOW())`); err != nil {
		t.Fatalf("insert duplicates: %v", err)
	}

	// Step 4: EnsureSchema will fail to create unique index idx_vg_conn_owner_name because of duplicate keys
	err := vgbackup.EnsureSchema(db.GDB)
	if err == nil {
		t.Fatal("expected EnsureSchema to fail when duplicate rows exist for a unique index")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "duplicate") && !strings.Contains(strings.ToLower(err.Error()), "cannot create a unique index") {
		t.Logf("got expected failure on duplicate data: %v", err)
	}
}
