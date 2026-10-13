package vgbackup_test

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"serverhub/internal/testdb"
	"serverhub/internal/vgbackup"
)

// TestSchedulerDuplicateTicksAndConcurrentClaims tests that concurrent scheduler
// Fire invocations on the same job with the same scheduled_for time
// are protected by the partial unique index idx_vg_runs_job_scheduled and do not
// insert duplicate runs.
func TestSchedulerDuplicateTicksAndConcurrentClaims(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	agent := vgbackup.VgAgent{
		Name:            "test-agent",
		Status:          vgbackup.VgAgentOnline,
		CreatedByUserID: 1,
	}
	if err := gdb.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}

	target := vgbackup.VgStorageTarget{
		Name:            "local-storage",
		Type:            vgbackup.VgStorageLocal,
		Path:            "/tmp/backups",
		CreatedByUserID: 1,
	}
	if err := gdb.Create(&target).Error; err != nil {
		t.Fatal(err)
	}

	conn := vgbackup.VgDatabaseConnection{
		Name:            "prod-pg",
		Type:            vgbackup.VgConnectionPostgres,
		Host:            "127.0.0.1",
		Port:            5432,
		Username:        "postgres",
		AgentID:         agent.ID,
		CreatedByUserID: 1,
	}
	if err := gdb.Create(&conn).Error; err != nil {
		t.Fatal(err)
	}

	policy := vgbackup.VgBackupPolicy{
		Name:            "daily-policy",
		CronExpr:        "0 0 * * *",
		RetentionDays:   7,
		Enabled:         true,
		CreatedByUserID: 1,
	}
	if err := gdb.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}

	job := vgbackup.VgBackupJob{
		Name:            "nightly-backup",
		AgentID:         agent.ID,
		ConnectionID:    &conn.ID,
		StorageTargetID: target.ID,
		PolicyID:        &policy.ID,
		SourceType:      vgbackup.VgSourcePostgres,
		Enabled:         true,
		CreatedByUserID: 1,
	}
	if err := gdb.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{
		Interval: 100 * time.Millisecond,
	})

	scheduledFor := time.Now().Truncate(time.Minute)

	// Launch 10 concurrent goroutines claiming the exact same scheduled run
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sched.Fire(context.Background(), &job, scheduledFor)
		}()
	}
	wg.Wait()

	// Verify only exactly 1 run was inserted with this scheduled_for
	var runCount int64
	if err := gdb.Model(&vgbackup.VgBackupRun{}).Where("backup_job_id = ? AND scheduled_for = ?", job.ID, scheduledFor.UTC()).Count(&runCount).Error; err != nil {
		t.Fatal(err)
	}
	if runCount != 1 {
		t.Fatalf("expected exactly 1 scheduled run, got %d", runCount)
	}
}

// TestSchedulerScheduleDueTimeHandling tests cron boundary matching and due-time
// handling under Tick.
func TestSchedulerScheduleDueTimeHandling(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{
		Interval: 100 * time.Millisecond,
	})

	agent := vgbackup.VgAgent{
		Name:            "test-agent-2",
		Status:          vgbackup.VgAgentOnline,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&agent)

	target := vgbackup.VgStorageTarget{
		Name:            "local-storage-2",
		Type:            vgbackup.VgStorageLocal,
		Path:            "/tmp/backups",
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&target)

	conn := vgbackup.VgDatabaseConnection{
		Name:            "prod-pg-2",
		Type:            vgbackup.VgConnectionPostgres,
		Host:            "127.0.0.1",
		Port:            5432,
		Username:        "postgres",
		AgentID:         agent.ID,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&conn)

	// Cron: every minute (* * * * *)
	policy := vgbackup.VgBackupPolicy{
		Name:            "every-minute",
		CronExpr:        "* * * * *",
		RetentionDays:   3,
		Enabled:         true,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&policy)

	job := vgbackup.VgBackupJob{
		Name:            "minutely-job",
		AgentID:         agent.ID,
		ConnectionID:    &conn.ID,
		StorageTargetID: target.ID,
		PolicyID:        &policy.ID,
		SourceType:      vgbackup.VgSourcePostgres,
		Enabled:         true,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&job)

	// Tick across a minute boundary
	prevTime := time.Date(2026, 10, 11, 14, 29, 30, 0, time.UTC)
	fixedTime := time.Date(2026, 10, 11, 14, 30, 15, 0, time.UTC)

	// Set lastTick so the first tick advances across 14:30:00
	sched.Tick(context.Background(), prevTime)
	sched.Tick(context.Background(), fixedTime)

	var runs []vgbackup.VgBackupRun
	gdb.Where("backup_job_id = ?", job.ID).Find(&runs)
	if len(runs) != 1 {
		t.Fatalf("expected 1 run fired by Tick, got %d", len(runs))
	}
	expectedScheduled := time.Date(2026, 10, 11, 14, 30, 0, 0, time.UTC)
	if runs[0].ScheduledFor == nil || !runs[0].ScheduledFor.Equal(expectedScheduled) {
		t.Fatalf("expected scheduled_for %v, got %v", expectedScheduled, runs[0].ScheduledFor)
	}

	// Repeated tick within the same minute should NOT fire a duplicate run
	sched.Tick(context.Background(), fixedTime.Add(20*time.Second))
	gdb.Where("backup_job_id = ?", job.ID).Find(&runs)
	if len(runs) != 1 {
		t.Fatalf("expected still 1 run, got %d", len(runs))
	}
}

// TestSchedulerStaleRunRecovery tests that in-progress runs whose agent is offline
// or stopped reporting are marked failed.
func TestSchedulerStaleRunRecovery(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{
		Interval: 100 * time.Millisecond,
	})

	// Agent with last_seen_at > 10 minutes ago (offline)
	oldSeen := time.Now().Add(-10 * time.Minute)
	agent := vgbackup.VgAgent{
		Name:            "offline-agent",
		Status:          vgbackup.VgAgentOffline,
		LastSeenAt:      &oldSeen,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&agent)

	target := vgbackup.VgStorageTarget{
		Name:            "local-storage-3",
		Type:            vgbackup.VgStorageLocal,
		Path:            "/tmp/backups",
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&target)

	conn := vgbackup.VgDatabaseConnection{
		Name:            "prod-pg-3",
		Type:            vgbackup.VgConnectionPostgres,
		Host:            "127.0.0.1",
		Port:            5432,
		Username:        "postgres",
		AgentID:         agent.ID,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&conn)

	job := vgbackup.VgBackupJob{
		Name:            "stale-check-job",
		AgentID:         agent.ID,
		ConnectionID:    &conn.ID,
		StorageTargetID: target.ID,
		SourceType:      vgbackup.VgSourcePostgres,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&job)

	// Create a run that started 1 hour ago and is still RUNNING
	oldTime := time.Now().Add(-1 * time.Hour)
	staleRun := vgbackup.VgBackupRun{
		BackupJobID:     job.ID,
		AgentID:         agent.ID,
		StorageTargetID: target.ID,
		Status:          vgbackup.VgRunRunning,
		StartedAt:       &oldTime,
	}
	_ = gdb.Create(&staleRun)

	// Force updated_at back so CheckStaleRuns identifies it as stale
	gdb.Model(&staleRun).Update("updated_at", oldTime)

	// Call CheckStaleRuns
	sched.CheckStaleRuns(context.Background(), time.Now())

	var updated vgbackup.VgBackupRun
	if err := gdb.First(&updated, "id = ?", staleRun.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updated.Status != vgbackup.VgRunFailed {
		t.Fatalf("expected run status failed, got %s", updated.Status)
	}
	if updated.ErrorMessage == "" {
		t.Fatalf("expected error message on stale run, got empty")
	}
}

// TestSchedulerRetentionAndPathSafety tests retention deletion with path safety
// (traversal prevention, active restore protection).
func TestSchedulerRetentionAndPathSafety(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	tempDir := t.TempDir()
	backupsRoot := filepath.Join(tempDir, "backups")
	if err := os.MkdirAll(backupsRoot, 0755); err != nil {
		t.Fatal(err)
	}

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{
		Interval: 100 * time.Millisecond,
		// File deletion is opt-in and confined: the temp target dir doubles
		// as the explicit delete root for this test.
		LocalDeleteRoot: backupsRoot,
	})

	agent := vgbackup.VgAgent{
		Name:            "safe-agent",
		Status:          vgbackup.VgAgentOnline,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&agent)

	target := vgbackup.VgStorageTarget{
		Name:            "safe-storage",
		Type:            vgbackup.VgStorageLocal,
		Path:            backupsRoot,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&target)

	job := vgbackup.VgBackupJob{
		Name:            "retention-job",
		AgentID:         agent.ID,
		StorageTargetID: target.ID,
		RetentionDays:   1, // 1 day retention
		SourceType:      vgbackup.VgSourcePostgres,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&job)

	// 1. Create an expired completed run (> 2 days ago)
	oldCompletedAt := time.Now().Add(-48 * time.Hour)
	safeFile := filepath.Join(backupsRoot, "valid-old-backup.tar.gz")
	if err := os.WriteFile(safeFile, []byte("backup-data"), 0644); err != nil {
		t.Fatal(err)
	}

	expiredRun := vgbackup.VgBackupRun{
		BackupJobID:     job.ID,
		AgentID:         agent.ID,
		StorageTargetID: target.ID,
		Status:          vgbackup.VgRunCompleted,
		StoragePath:     safeFile,
		CompletedAt:     &oldCompletedAt,
	}
	_ = gdb.Create(&expiredRun)

	artSafe := vgbackup.VgBackupArtifact{
		BackupRunID:     expiredRun.ID,
		Name:            "artifact1.tar.gz",
		StoragePath:     safeFile,
		StorageTargetID: target.ID,
		Size:            11,
	}
	_ = gdb.Create(&artSafe)

	// 2. Create another artifact that attempts directory traversal: filepath outside root
	outsideFile := filepath.Join(tempDir, "outside-secret.txt")
	if err := os.WriteFile(outsideFile, []byte("super-secret"), 0644); err != nil {
		t.Fatal(err)
	}
	artTraversal := vgbackup.VgBackupArtifact{
		BackupRunID:     expiredRun.ID,
		Name:            "evil.txt",
		StoragePath:     outsideFile,
		StorageTargetID: target.ID,
		Size:            12,
	}
	_ = gdb.Create(&artTraversal)

	// Execute retention sweep
	sched.EnforceRetention(context.Background(), time.Now())

	// Safe artifact file should have been deleted
	if _, err := os.Stat(safeFile); !os.IsNotExist(err) {
		t.Errorf("safe file should have been deleted by retention, but still exists")
	}

	// Traversal file outside root MUST NOT be deleted
	if _, err := os.Stat(outsideFile); err != nil {
		t.Errorf("file outside storage root was deleted or inaccessible: %v", err)
	}

	// Verify run and artifact rows were deleted in DB
	var checkRun vgbackup.VgBackupRun
	if err := gdb.First(&checkRun, "id = ?", expiredRun.ID).Error; err == nil {
		t.Errorf("expected expired run to be deleted in db")
	}
}

// TestSchedulerActiveRestorePreventsRetention tests that artifacts referenced by
// pending or in-progress restores are never deleted by retention.
func TestSchedulerActiveRestorePreventsRetention(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{
		Interval: 100 * time.Millisecond,
	})

	tempDir := t.TempDir()
	backupsRoot := filepath.Join(tempDir, "backups-restore")
	_ = os.MkdirAll(backupsRoot, 0755)

	agent := vgbackup.VgAgent{
		Name:            "restore-agent",
		Status:          vgbackup.VgAgentOnline,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&agent)

	target := vgbackup.VgStorageTarget{
		Name:            "restore-storage",
		Type:            vgbackup.VgStorageLocal,
		Path:            backupsRoot,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&target)

	job := vgbackup.VgBackupJob{
		Name:            "restore-job",
		AgentID:         agent.ID,
		StorageTargetID: target.ID,
		RetentionDays:   1,
		SourceType:      vgbackup.VgSourcePostgres,
		CreatedByUserID: 1,
	}
	_ = gdb.Create(&job)

	oldTime := time.Now().Add(-72 * time.Hour)
	filePath := filepath.Join(backupsRoot, "in-use.tar.gz")
	_ = os.WriteFile(filePath, []byte("active-restore-data"), 0644)

	expiredRun := vgbackup.VgBackupRun{
		BackupJobID:     job.ID,
		AgentID:         agent.ID,
		StorageTargetID: target.ID,
		Status:          vgbackup.VgRunCompleted,
		StoragePath:     filePath,
		CompletedAt:     &oldTime,
	}
	_ = gdb.Create(&expiredRun)

	art := vgbackup.VgBackupArtifact{
		BackupRunID:     expiredRun.ID,
		Name:            "in-use.tar.gz",
		StoragePath:     filePath,
		StorageTargetID: target.ID,
		Size:            19,
	}
	_ = gdb.Create(&art)

	// Create an active restore referencing this run
	restoreJob := vgbackup.VgRestoreJob{
		BackupRunID: expiredRun.ID,
		AgentID:     agent.ID,
		Status:      vgbackup.VgRestoreRunning,
	}
	_ = gdb.Create(&restoreJob)

	// Run retention
	sched.EnforceRetention(context.Background(), time.Now())

	// File should STILL exist because of the active restore
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		t.Fatalf("file should NOT have been deleted while restore is in progress")
	}

	var checkRun vgbackup.VgBackupRun
	if err := gdb.First(&checkRun, "id = ?", expiredRun.ID).Error; err != nil {
		t.Fatalf("run should still exist in db while restore is active")
	}
}

// TestSchedulerLifecycleAndGracefulShutdown tests start, duplicate start guard,
// and graceful shutdown.
func TestSchedulerLifecycleAndGracefulShutdown(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{
		Interval: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := sched.Start(ctx); err != nil {
		t.Fatalf("failed to start scheduler: %v", err)
	}

	// Duplicate start should error cleanly
	if err := sched.Start(ctx); err == nil {
		t.Fatalf("expected error on duplicate start, got nil")
	}

	// Stop cleanly
	sched.Stop()

	// Stopping again should be safe and idempotent
	sched.Stop()
}
