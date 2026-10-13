package vgbackup_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"serverhub/internal/testdb"
	"serverhub/internal/vgbackup"
)

func sha256Of(t *testing.T, data []byte) string {
	t.Helper()
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// TestVerifyCompletedRuns settles the verification contract: server-readable
// LOCAL artifacts with matching checksums become VERIFIED, mismatches become
// FAILED, and anything unverifiable stays PENDING (never a false failure).
func TestVerifyCompletedRuns(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB
	ctx := context.Background()
	now := time.Now()

	root := t.TempDir()
	agent := vgbackup.VgAgent{Name: "verify-agent", Status: vgbackup.VgAgentOnline, CreatedByUserID: 1}
	if err := gdb.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	target := vgbackup.VgStorageTarget{Name: "verify-tgt", Type: vgbackup.VgStorageLocal, Path: root, CreatedByUserID: 1}
	if err := gdb.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	job := vgbackup.VgBackupJob{
		Name: "verify-job", AgentID: agent.ID, StorageTargetID: target.ID,
		SourceType: vgbackup.VgSourceFilesystem, SourcePath: "/data",
		Mode: vgbackup.VgModeNormal, Enabled: true, CreatedByUserID: 1,
	}
	if err := gdb.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	done := now.Add(-time.Hour)
	run := vgbackup.VgBackupRun{
		BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: target.ID,
		Status: vgbackup.VgRunCompleted, CompletedAt: &done,
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatal(err)
	}

	goodData := []byte("real-backup-bytes")
	goodPath := filepath.Join(root, "good.bak")
	if err := os.WriteFile(goodPath, goodData, 0600); err != nil {
		t.Fatal(err)
	}
	badData := []byte("tampered-bytes")
	badPath := filepath.Join(root, "bad.bak")
	if err := os.WriteFile(badPath, badData, 0600); err != nil {
		t.Fatal(err)
	}
	mkArtifact := func(name, path, sum string) vgbackup.VgBackupArtifact {
		a := vgbackup.VgBackupArtifact{
			BackupRunID: run.ID, Name: name, StoragePath: path,
			Checksum: sum, StorageTargetID: target.ID,
		}
		if err := gdb.Create(&a).Error; err != nil {
			t.Fatal(err)
		}
		return a
	}
	good := mkArtifact("good.bak", goodPath, sha256Of(t, goodData))
	bad := mkArtifact("bad.bak", badPath, sha256Of(t, []byte("original-bytes")))
	remote := mkArtifact("remote.bak", filepath.Join(string(filepath.Separator), "nonexistent", "remote.bak"), sha256Of(t, goodData))
	nocheck := mkArtifact("nocheck.bak", goodPath, "")

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{})
	sched.VerifyCompletedRuns(ctx, now)

	statusOf := func(id interface{}) (string, bool) {
		var a vgbackup.VgBackupArtifact
		if err := gdb.Where("id = ?", id).First(&a).Error; err != nil {
			t.Fatal(err)
		}
		return a.VerificationStatus, a.VerifiedAt != nil
	}
	if st, ts := statusOf(good.ID); st != vgbackup.VgArtifactVerified || !ts {
		t.Errorf("matching artifact must be VERIFIED with timestamp, got %s/%v", st, ts)
	}
	if st, ts := statusOf(bad.ID); st != vgbackup.VgArtifactVerifyFail || !ts {
		t.Errorf("mismatching artifact must be FAILED with timestamp, got %s/%v", st, ts)
	}
	if st, ts := statusOf(remote.ID); st != vgbackup.VgArtifactUnverified || ts {
		t.Errorf("unreadable artifact must stay PENDING without timestamp, got %s/%v", st, ts)
	}
	if st, _ := statusOf(nocheck.ID); st != vgbackup.VgArtifactUnverified {
		t.Errorf("checksum-less artifact must stay PENDING, got %s", st)
	}

	// Second pass is stable: settled rows are not revisited.
	sched.VerifyCompletedRuns(ctx, now)
	if st, _ := statusOf(good.ID); st != vgbackup.VgArtifactVerified {
		t.Errorf("re-pass must keep VERIFIED, got %s", st)
	}
}

// TestVerifyRespectsDisabledFlag proves verification_enabled=false opts a
// job's artifacts out of the automatic pass (manual Verify still works).
func TestVerifyRespectsDisabledFlag(t *testing.T) {
	db := testdb.Open(t)
	gdb := db.GDB
	ctx := context.Background()
	now := time.Now()

	root := t.TempDir()
	agent := vgbackup.VgAgent{Name: "noverify-agent", Status: vgbackup.VgAgentOnline, CreatedByUserID: 1}
	if err := gdb.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	target := vgbackup.VgStorageTarget{Name: "noverify-tgt", Type: vgbackup.VgStorageLocal, Path: root, CreatedByUserID: 1}
	if err := gdb.Create(&target).Error; err != nil {
		t.Fatal(err)
	}
	policy := vgbackup.VgBackupPolicy{Name: "noverify", CreatedByUserID: 1}
	if err := gdb.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	// GORM drops explicit false on Create (default:true tag): force it.
	gdb.Model(&policy).Update("verification_enabled", false)
	job := vgbackup.VgBackupJob{
		Name: "noverify-job", AgentID: agent.ID, StorageTargetID: target.ID,
		PolicyID: &policy.ID, SourceType: vgbackup.VgSourceFilesystem, SourcePath: "/data",
		Mode: vgbackup.VgModeNormal, Enabled: true, CreatedByUserID: 1,
	}
	if err := gdb.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	data := []byte("bytes")
	p := filepath.Join(root, "f.bak")
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	done := now.Add(-time.Hour)
	run := vgbackup.VgBackupRun{
		BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: target.ID,
		Status: vgbackup.VgRunCompleted, CompletedAt: &done,
	}
	if err := gdb.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	art := vgbackup.VgBackupArtifact{
		BackupRunID: run.ID, Name: "f.bak", StoragePath: p,
		Checksum: sha256Of(t, data), StorageTargetID: target.ID,
	}
	if err := gdb.Create(&art).Error; err != nil {
		t.Fatal(err)
	}

	sched := vgbackup.NewScheduler(gdb, vgbackup.SchedulerConfig{})
	sched.VerifyCompletedRuns(ctx, now)

	var got vgbackup.VgBackupArtifact
	if err := gdb.Where("id = ?", art.ID).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.VerificationStatus != vgbackup.VgArtifactUnverified {
		t.Errorf("disabled verification must leave artifact PENDING, got %s", got.VerificationStatus)
	}
}
