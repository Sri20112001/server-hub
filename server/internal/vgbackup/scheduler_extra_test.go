package vgbackup_test

// Retention delete-root modes: rows-only default vs confined file deletion.

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"serverhub/internal/testdb"
	"serverhub/internal/vgbackup"
)

func seedRetentionFixture(t *testing.T, gdb *gorm.DB, targetPath, artName, artContent string, completedAgo time.Duration) (runID uuid.UUID, artPath string) {
	t.Helper()
	agent := vgbackup.VgAgent{Name: "ret-agent", Status: vgbackup.VgAgentOnline, CreatedByUserID: 1}
	if err := gdb.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	target := vgbackup.VgStorageTarget{Name: "ret-local", Type: vgbackup.VgStorageLocal, Path: targetPath, CreatedByUserID: 1}
	if err := gdb.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	job := vgbackup.VgBackupJob{
		Name: "ret-job", AgentID: agent.ID, StorageTargetID: target.ID,
		SourceType: vgbackup.VgSourceFilesystem, Enabled: true,
		RetentionDays: 30, CreatedByUserID: 1,
	}
	if err := gdb.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	completed := time.Now().Add(-completedAgo)
	run := vgbackup.VgBackupRun{
		BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: target.ID,
		Status: vgbackup.VgRunCompleted, CompletedAt: &completed,
		StoragePath: filepath.Join(targetPath, artName),
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	artPath = filepath.Join(targetPath, artName)
	if err := os.WriteFile(artPath, []byte(artContent), 0600); err != nil {
		t.Fatal(err)
	}
	art := vgbackup.VgBackupArtifact{BackupRunID: run.ID, Name: artName, Size: int64(len(artContent)), StoragePath: artPath, StorageTargetID: target.ID}
	if err := gdb.Create(&art).Error; err != nil {
		t.Fatal(err)
	}
	return run.ID, artPath
}

func runGone(t *testing.T, gdb *gorm.DB, runID uuid.UUID) bool {
	t.Helper()
	var n int64
	gdb.Model(&vgbackup.VgBackupRun{}).Where("id = ?", runID).Count(&n)
	return n == 0
}

// Default (no delete root): expired rows pruned, files always kept.
func TestRetentionRowsOnlyByDefault(t *testing.T) {
	db := testdb.Open(t)
	root := t.TempDir()
	runID, artPath := seedRetentionFixture(t, db.GDB, root, "a.tar", "bytes", 31*24*time.Hour)

	sched := vgbackup.NewScheduler(db.GDB, vgbackup.SchedulerConfig{Interval: time.Second})
	sched.EnforceRetention(context.Background(), time.Now())

	if !runGone(t, db.GDB, runID) {
		t.Error("expired run row must be pruned")
	}
	if _, err := os.Stat(artPath); err != nil {
		t.Errorf("file must be kept without a delete root: %v", err)
	}
}

// Configured root: files strictly inside it are deleted with their rows.
func TestRetentionDeletesInsideRoot(t *testing.T) {
	db := testdb.Open(t)
	root := t.TempDir()
	runID, artPath := seedRetentionFixture(t, db.GDB, root, "a.tar", "bytes", 31*24*time.Hour)

	sched := vgbackup.NewScheduler(db.GDB, vgbackup.SchedulerConfig{Interval: time.Second, LocalDeleteRoot: root})
	sched.EnforceRetention(context.Background(), time.Now())

	if !runGone(t, db.GDB, runID) {
		t.Error("expired run row must be pruned")
	}
	if _, err := os.Stat(artPath); !os.IsNotExist(err) {
		t.Error("in-root artifact file must be deleted")
	}
}

// Outside-root artifact paths are never deleted, even with a root set.
func TestRetentionSkipsOutsideRoot(t *testing.T) {
	db := testdb.Open(t)
	root := t.TempDir()
	outside := t.TempDir()
	outPath := filepath.Join(outside, "stray.tar")
	if err := os.WriteFile(outPath, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	runID, _ := seedRetentionFixture(t, db.GDB, root, "a.tar", "bytes", 31*24*time.Hour)
	// Point the run's own storage path outside the target root too.
	db.GDB.Model(&vgbackup.VgBackupRun{}).Where("id = ?", runID).Update("storage_path", outPath)

	sched := vgbackup.NewScheduler(db.GDB, vgbackup.SchedulerConfig{Interval: time.Second, LocalDeleteRoot: root})
	sched.EnforceRetention(context.Background(), time.Now())

	if !runGone(t, db.GDB, runID) {
		t.Error("expired run row must be pruned")
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("outside-root file must be kept: %v", err)
	}
}

// Non-expired runs are never touched.
func TestRetentionKeepsFreshRuns(t *testing.T) {
	db := testdb.Open(t)
	root := t.TempDir()
	runID, artPath := seedRetentionFixture(t, db.GDB, root, "a.tar", "bytes", time.Hour)

	sched := vgbackup.NewScheduler(db.GDB, vgbackup.SchedulerConfig{Interval: time.Second, LocalDeleteRoot: root})
	sched.EnforceRetention(context.Background(), time.Now())

	if runGone(t, db.GDB, runID) {
		t.Error("fresh run must be kept")
	}
	if _, err := os.Stat(artPath); err != nil {
		t.Errorf("fresh artifact file must be kept: %v", err)
	}
}

// Concurrent full ticks against one due job collapse into exactly one
// run: the tick mutex serializes passes while the in-flight guard and the
// boundary unique index collapse anything that still races.
func TestSchedulerConcurrentTicksOneRun(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	agent := vgbackup.VgAgent{Name: "tick-agent", Status: vgbackup.VgAgentOnline, CreatedByUserID: 1}
	if err := gdb.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	target := vgbackup.VgStorageTarget{Name: "t", Type: vgbackup.VgStorageLocal, Path: t.TempDir(), CreatedByUserID: 1}
	if err := gdb.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	job := vgbackup.VgBackupJob{
		Name: "tick-job", AgentID: agent.ID, StorageTargetID: target.ID,
		SourceType: vgbackup.VgSourceFilesystem, Enabled: true, CreatedByUserID: 1,
	}
	if err := gdb.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	// Every-minute schedule: any tick window straddling a minute boundary fires.
	sched := vgbackup.VgBackupSchedule{BackupJobID: job.ID, CronExpr: "* * * * *", Timezone: "UTC"}
	if err := gdb.Create(&sched).Error; err != nil {
		t.Fatal(err)
	}

	s := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{Interval: time.Second})
	// Arm lastTick, then fire 8 concurrent ticks into the next minute:
	// every pass computes the same boundary and exactly one run survives
	// (in-flight guard + boundary unique index).
	base := time.Now()
	s.Tick(context.Background(), base)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Tick(context.Background(), base.Add(61*time.Second))
		}()
	}
	wg.Wait()

	var n int64
	gdb.Model(&vgbackup.VgBackupRun{}).Where("backup_job_id = ?", job.ID).Count(&n)
	if n != 1 {
		t.Fatalf("concurrent ticks must yield exactly one run, got %d", n)
	}
}

// Repeated ticks (restarts, double ticks) refire a failed run exactly
// once: the second tick sees the first tick's PENDING retry as inflight.
// Cross-process races collapse on the partial unique index instead (same
// mechanism as cron-boundary Fire, which has a dedicated concurrency test).
func TestSchedulerRetryTickIdempotent(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB

	agent := vgbackup.VgAgent{Name: "retry-agent", Status: vgbackup.VgAgentOnline, CreatedByUserID: 1}
	if err := gdb.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	target := vgbackup.VgStorageTarget{Name: "t", Type: vgbackup.VgStorageLocal, Path: t.TempDir(), CreatedByUserID: 1}
	if err := gdb.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	policy := vgbackup.VgBackupPolicy{Name: "p", MaxRetries: 3, RetryDelaySeconds: 1, Enabled: true, CreatedByUserID: 1}
	if err := gdb.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	job := vgbackup.VgBackupJob{
		Name: "retry-job", AgentID: agent.ID, StorageTargetID: target.ID,
		PolicyID: &policy.ID, SourceType: vgbackup.VgSourcePostgres,
		Enabled: true, CreatedByUserID: 1,
	}
	if err := gdb.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	completed := time.Now().Add(-time.Hour)
	failed := vgbackup.VgBackupRun{
		BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: target.ID,
		Status: vgbackup.VgRunFailed, CompletedAt: &completed, RetryAttempt: 0,
	}
	if err := gdb.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{Interval: time.Second})
	ctx := context.Background()
	sched.Tick(ctx, time.Now()) // arms lastTick; fires nothing (restart safety)
	sched.Tick(ctx, time.Now()) // fires the retry
	sched.Tick(ctx, time.Now()) // sees PENDING inflight; must not refire

	var n int64
	gdb.Model(&vgbackup.VgBackupRun{}).Where("retry_of_run_id = ?", failed.ID).Count(&n)
	if n != 1 {
		t.Fatalf("repeated ticks must yield exactly one retry, got %d", n)
	}
}
