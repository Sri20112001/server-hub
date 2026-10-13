package vgbackup

import (
	"gorm.io/gorm"
)

// Models returns every backup-domain table in dependency-safe creation
// order (referenced tables first: agents before machines/jobs, jobs before
// runs/schedules, runs before artifacts, artifacts before chunks).
func Models() []any {
	return []any{
		&VgAgent{},
		&VgStorageTarget{},
		&VgS3Region{},
		&VgDatabaseConnection{},
		&VgBackupPolicy{},
		&VgBackupJob{},
		&VgBackupSchedule{},
		&VgMachine{},
		&VgEnrollmentToken{},
		&VgBackupRun{},
		&VgBackupArtifact{},
		&VgBackupChunk{},
		&VgRestoreJob{},
		&VgBackupSizeBaseline{},
	}
}

// EnsureSchema creates any missing vg_* table and leaves existing ones
// untouched — the same additive strategy as database.OpenDatabase.
//
// LIMITATION (documented, not fixed here): like the rest of ServerHub,
// HasTable+CreateTable never alters an already-existing table. If a vg_*
// table's columns ever need to change, add an idempotent
// ALTER TABLE ... ADD COLUMN IF NOT EXISTS upgrade next to
// ensureNotificationSchema/ensureMonitoringSchema in package database and
// call it from OpenDatabase. Never drop, truncate, or recreate tables to
// "migrate": backup history and agent credentials must survive upgrades.
func EnsureSchema(gdb *gorm.DB) error {
	for _, m := range Models() {
		if gdb.Migrator().HasTable(m) {
			continue
		}
		if err := gdb.Migrator().CreateTable(m); err != nil {
			return err
		}
	}
	return ensureVgIndexes(gdb)
}

// ensureVgIndexes backfills uniqueness constraints the idempotency paths
// depend on, for tables created before the tags existed (fresh CreateTable
// builds them from the model tags; this covers the rest). All statements
// are safe to re-run on every boot.
func ensureVgIndexes(gdb *gorm.DB) error {
	for _, q := range []string{
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_vg_artifact_run_name ON vg_backup_artifacts(backup_run_id, name)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_vg_chunk_artifact_index ON vg_backup_chunks(artifact_id, "index")`,
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_vg_runs_job_scheduled ON vg_backup_runs(backup_job_id, scheduled_for)`,
		// One retry row per failed run (see maybeFireRetry): concurrent
		// ticks collapse instead of queueing duplicate refires.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_vg_runs_retry_once ON vg_backup_runs(backup_job_id, retry_of_run_id) WHERE retry_of_run_id IS NOT NULL`,
		// Duplicate guard for restore creation: concurrent double submits
		// collapse instead of queueing the same destructive op twice.
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_vg_restores_pending_dedupe ON vg_restore_jobs(backup_run_id, agent_id, destination_path, target_database) WHERE status = 'PENDING'`,
		// Connection names are unique per owner (mirrors upstream's
		// per-organization uniqueness under single-tenant ownership).
		`CREATE UNIQUE INDEX IF NOT EXISTS idx_vg_conn_owner_name ON vg_database_connections(created_by_user_id, name)`,
	} {
		if err := gdb.Exec(q).Error; err != nil {
			return err
		}
	}
	return nil
}
