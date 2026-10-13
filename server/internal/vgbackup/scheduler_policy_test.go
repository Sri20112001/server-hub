package vgbackup_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"serverhub/internal/testdb"
	"serverhub/internal/vgbackup"
)

// TestRetentionUsesPolicyRetention proves the retention sweep resolves
// settings policy-first: an attached policy's RetentionDays governs, not
// the job row's creation-time default.
func TestRetentionUsesPolicyRetention(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB
	ctx := context.Background()

	mkInfra := func(name string) (vgbackup.VgAgent, vgbackup.VgStorageTarget) {
		agent := vgbackup.VgAgent{Name: name + "-agent", Status: vgbackup.VgAgentOnline, CreatedByUserID: 1}
		if err := gdb.Create(&agent).Error; err != nil {
			t.Fatal(err)
		}
		target := vgbackup.VgStorageTarget{Name: name + "-tgt", Type: vgbackup.VgStorageLocal, Path: "/tmp/" + name, CreatedByUserID: 1}
		if err := gdb.Create(&target).Error; err != nil {
			t.Fatal(err)
		}
		return agent, target
	}
	mkOldRun := func(job vgbackup.VgBackupJob, agent vgbackup.VgAgent, target vgbackup.VgStorageTarget) {
		done := time.Now().Add(-48 * time.Hour)
		run := vgbackup.VgBackupRun{
			BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: target.ID,
			Status: vgbackup.VgRunCompleted, CompletedAt: &done,
		}
		if err := gdb.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
	}
	runCount := func(jobID uuid.UUID) int64 {
		var n int64
		if err := gdb.Model(&vgbackup.VgBackupRun{}).Where("backup_job_id = ?", jobID).Count(&n).Error; err != nil {
			t.Fatal(err)
		}
		return n
	}

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{})

	// Case 1: inline says keep 3650d, attached policy says keep 1d →
	// the 2-day-old run must be pruned (policy wins).
	agent1, target1 := mkInfra("pol")
	policy := vgbackup.VgBackupPolicy{Name: "keep-1d", RetentionDays: 1, Enabled: true, CreatedByUserID: 1}
	if err := gdb.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	job1 := vgbackup.VgBackupJob{
		Name: "policy-job", AgentID: agent1.ID, StorageTargetID: target1.ID,
		PolicyID: &policy.ID, SourceType: vgbackup.VgSourceFilesystem, SourcePath: "/data",
		RetentionDays: 3650, Enabled: true, CreatedByUserID: 1,
	}
	if err := gdb.Create(&job1).Error; err != nil {
		t.Fatal(err)
	}
	mkOldRun(job1, agent1, target1)

	// Case 2: inline says keep 1d, attached policy says keep 3650d →
	// the 2-day-old run must be kept (policy wins).
	agent2, target2 := mkInfra("pol2")
	policy2 := vgbackup.VgBackupPolicy{Name: "keep-long", RetentionDays: 3650, Enabled: true, CreatedByUserID: 1}
	if err := gdb.Create(&policy2).Error; err != nil {
		t.Fatal(err)
	}
	job2 := vgbackup.VgBackupJob{
		Name: "policy-job-2", AgentID: agent2.ID, StorageTargetID: target2.ID,
		PolicyID: &policy2.ID, SourceType: vgbackup.VgSourceFilesystem, SourcePath: "/data",
		RetentionDays: 1, Enabled: true, CreatedByUserID: 1,
	}
	if err := gdb.Create(&job2).Error; err != nil {
		t.Fatal(err)
	}
	mkOldRun(job2, agent2, target2)

	sched.EnforceRetention(ctx, time.Now())

	if n := runCount(job1.ID); n != 0 {
		t.Errorf("policy retention 1d must prune the old run despite inline 3650d, %d rows remain", n)
	}
	if n := runCount(job2.ID); n != 1 {
		t.Errorf("policy retention 3650d must keep the old run despite inline 1d, got %d rows", n)
	}
}

// TestTickRespectsPolicyEnabled proves the firing sweep lets the attached
// policy's Enabled govern: a row-paused job with an enabled policy still
// fires, and a row-enabled job with a disabled policy does not.
func TestTickRespectsPolicyEnabled(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB
	ctx := context.Background()

	agent := vgbackup.VgAgent{Name: "tick-agent", Status: vgbackup.VgAgentOnline, CreatedByUserID: 1}
	if err := gdb.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	target := vgbackup.VgStorageTarget{Name: "tick-tgt", Type: vgbackup.VgStorageLocal, Path: "/tmp/tick", CreatedByUserID: 1}
	if err := gdb.Create(&target).Error; err != nil {
		t.Fatal(err)
	}

	onPolicy := vgbackup.VgBackupPolicy{Name: "on", CronExpr: "* * * * *", Timezone: "UTC", Enabled: true, CreatedByUserID: 1}
	if err := gdb.Create(&onPolicy).Error; err != nil {
		t.Fatal(err)
	}
	offPolicy := vgbackup.VgBackupPolicy{Name: "off", CronExpr: "* * * * *", Timezone: "UTC", Enabled: false, CreatedByUserID: 1}
	if err := gdb.Create(&offPolicy).Error; err != nil {
		t.Fatal(err)
	}
	// GORM omits false bools carrying a `default:true` tag on Create, so
	// force the intended values explicitly (same workaround as the API).
	gdb.Model(&offPolicy).Update("enabled", false)

	rowPaused := vgbackup.VgBackupJob{
		Name: "row-paused", AgentID: agent.ID, StorageTargetID: target.ID,
		PolicyID: &onPolicy.ID, SourceType: vgbackup.VgSourceFilesystem, SourcePath: "/data",
		Enabled: false, CreatedByUserID: 1,
	}
	if err := gdb.Create(&rowPaused).Error; err != nil {
		t.Fatal(err)
	}
	gdb.Model(&rowPaused).Update("enabled", false)
	policyOff := vgbackup.VgBackupJob{
		Name: "policy-off", AgentID: agent.ID, StorageTargetID: target.ID,
		PolicyID: &offPolicy.ID, SourceType: vgbackup.VgSourceFilesystem, SourcePath: "/data",
		Enabled: true, CreatedByUserID: 1,
	}
	if err := gdb.Create(&policyOff).Error; err != nil {
		t.Fatal(err)
	}

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{})
	now := time.Now()
	sched.Tick(ctx, now.Add(-2*time.Minute)) // prime lastTick
	sched.Tick(ctx, now)

	var n int64
	if err := gdb.Model(&vgbackup.VgBackupRun{}).Where("backup_job_id = ?", rowPaused.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("row-paused job with enabled policy must fire, got %d runs", n)
	}
	if err := gdb.Model(&vgbackup.VgBackupRun{}).Where("backup_job_id = ?", policyOff.ID).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("row-enabled job with disabled policy must not fire, got %d runs", n)
	}
}
