package vgbackup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/robfig/cron/v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Scheduler fires due backup jobs, refires eligible failed runs,
// marks abandoned running runs as failed, and safely prunes expired local backup artifacts.
type Scheduler struct {
	db       *gorm.DB
	interval time.Duration
	// deleteRoot, when set, is the only server-local tree retention may
	// remove files from (see deleteRunFiles). Empty disables file deletion
	// entirely: expired rows are still pruned, but bytes are left for the
	// operator (or bucket lifecycle rules) instead of guessed at.
	deleteRoot string
	parser     cron.Parser
	lastTick   time.Time

	mu      sync.Mutex
	running bool
	cancel  context.CancelFunc
	done    chan struct{}
	// tickMu serializes Tick executions so overlapping ticks (slow sweep
	// overrunning the interval, or concurrent callers in tests) never
	// interleave firing, retry, retention, and baseline passes.
	tickMu sync.Mutex
}

// SchedulerConfig configures the background scheduler.
type SchedulerConfig struct {
	Interval time.Duration
	// LocalDeleteRoot optionally enables artifact file deletion, confined
	// to this absolute server-local path. Empty (default) prunes database
	// rows only and never touches the filesystem.
	LocalDeleteRoot string
}

// NewScheduler creates a background scheduler instance.
func NewScheduler(db *gorm.DB, cfg SchedulerConfig) *Scheduler {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 30 * time.Second
	}
	return &Scheduler{
		db:         db,
		interval:   interval,
		deleteRoot: cfg.LocalDeleteRoot,
		parser: cron.NewParser(
			cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor,
		),
	}
}

// Start boots the background loop if not already running.
func (s *Scheduler) Start(parentCtx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("scheduler already running")
	}
	if s.db == nil {
		return fmt.Errorf("scheduler: database not initialized")
	}

	ctx, cancel := context.WithCancel(parentCtx)
	s.cancel = cancel
	s.running = true
	s.done = make(chan struct{})
	s.lastTick = time.Now()

	if s.deleteRoot != "" {
		log.Printf("scheduler: retention file deletion confined to %s", filepath.Clean(s.deleteRoot))
	} else {
		log.Printf("scheduler: retention file deletion disabled (no delete root configured) — expired rows pruned, files kept")
	}

	go s.run(ctx)
	return nil
}

// Stop cleanly terminates the scheduler goroutine.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	done := s.done
	s.running = false
	s.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

func (s *Scheduler) run(ctx context.Context) {
	defer close(s.done)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			s.Tick(ctx, now)
		}
	}
}

// Tick executes a single scheduler run: cron firing, retries, stale runs, retention, and size baselines.
func (s *Scheduler) Tick(ctx context.Context, now time.Time) {
	s.tickMu.Lock()
	defer s.tickMu.Unlock()
	if ctx.Err() != nil {
		return
	}
	prev := s.lastTick
	s.lastTick = now

	if !prev.IsZero() {
		s.tickJobs(ctx, prev, now)
	}
	s.CheckStaleRuns(ctx, now)
	s.EnforceRetention(ctx, now)
	s.VerifyCompletedRuns(ctx, now)
	s.UpdateSizeBaselines(ctx, now)
}

func (s *Scheduler) tickJobs(ctx context.Context, prev, now time.Time) {
	if s.db == nil {
		return
	}
	var jobs []VgBackupJob
	// No enabled pre-filter here: EffectivePolicy resolves the governing
	// flag (an attached policy's Enabled wins over the job row), so the
	// sweep must see every job and let eff.Enabled decide. Filtering on
	// the job row would strand policy-enabled jobs whose row is paused
	// and fire nothing the policy disabled.
	if err := s.db.WithContext(ctx).Preload("Schedule").Preload("Policy").
		Find(&jobs).Error; err != nil {
		log.Printf("scheduler: list enabled jobs failed: %v", err)
		return
	}

	for i := range jobs {
		if ctx.Err() != nil {
			return
		}
		job := &jobs[i]
		eff := EffectivePolicy(job)
		if !eff.Enabled {
			continue
		}

		if err := s.maybeFireRetry(ctx, job, eff, now); err != nil {
			log.Printf("scheduler: retry check failed for job %s: %v", job.ID, err)
		}

		if eff.CronExpr == "" {
			continue
		}
		boundary, ok := s.boundaryFor(eff.CronExpr, eff.Timezone, job.ID, prev, now)
		if !ok {
			continue
		}
		s.Fire(ctx, job, boundary)
	}
}

func (s *Scheduler) boundaryFor(cronExpr, timezone string, jobID uuid.UUID, prev, now time.Time) (time.Time, bool) {
	if cronExpr == "" {
		return time.Time{}, false
	}
	sched, err := s.parser.Parse(cronExpr)
	if err != nil {
		return time.Time{}, false
	}
	loc := time.UTC
	if timezone != "" {
		if l, err := time.LoadLocation(timezone); err == nil {
			loc = l
		}
	}
	next := sched.Next(prev.In(loc))
	if next.After(now.In(loc)) {
		return time.Time{}, false
	}
	return next, true
}

func (s *Scheduler) maybeFireRetry(ctx context.Context, job *VgBackupJob, eff ResolvedPolicy, now time.Time) error {
	if !eff.Enabled || eff.MaxRetries <= 0 {
		return nil
	}

	var inflight int64
	if err := s.db.WithContext(ctx).Model(&VgBackupRun{}).
		Where("backup_job_id = ? AND status IN ?", job.ID,
			[]VgBackupRunStatus{
				VgRunPending, VgRunRunning,
				VgRunUploading, VgRunVerifying,
			}).
		Count(&inflight).Error; err != nil {
		return err
	}
	if inflight > 0 {
		return nil
	}

	var latest VgBackupRun
	if err := s.db.WithContext(ctx).Where("backup_job_id = ?", job.ID).
		Order("created_at DESC").First(&latest).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil
		}
		return err
	}
	if latest.Status != VgRunFailed {
		return nil
	}
	if latest.RetryAttempt >= eff.MaxRetries {
		return nil
	}

	delay := eff.RetryDelaySeconds
	if delay <= 0 {
		delay = 300
	}
	if latest.CompletedAt == nil || now.Sub(*latest.CompletedAt) < time.Duration(delay)*time.Second {
		return nil
	}

	retryID := latest.ID
	run := VgBackupRun{
		BackupJobID:     job.ID,
		AgentID:         job.AgentID,
		StorageTargetID: job.StorageTargetID,
		Status:          VgRunPending,
		SourceType:      string(job.SourceType),
		RetryOfRunID:    &retryID,
		RetryAttempt:    latest.RetryAttempt + 1,
	}
	// Dedupe inside a transaction: lock the failed run row first so two
	// concurrent ticks serialize here, then re-check that no retry row
	// exists yet. (GORM renders ON CONFLICT ... WHERE ... DO NOTHING in an
	// order Postgres rejects, so the lock + re-check carries the guarantee
	// instead of the partial unique index, which remains as a backstop.)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked VgBackupRun
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", latest.ID).First(&locked).Error; err != nil {
			return err
		}
		var existing int64
		if err := tx.Model(&VgBackupRun{}).
			Where("retry_of_run_id = ? AND deleted_at IS NULL", latest.ID).
			Count(&existing).Error; err != nil {
			return err
		}
		if existing > 0 {
			return nil
		}
		return tx.Create(&run).Error
	})
}

// Fire creates a PENDING run for the given schedule boundary.
// It is protected against duplicate execution across concurrent ticks and restarts.
func (s *Scheduler) Fire(ctx context.Context, job *VgBackupJob, scheduledFor time.Time) {
	var inflight int64
	s.db.WithContext(ctx).Model(&VgBackupRun{}).
		Where("backup_job_id = ? AND status IN ?", job.ID,
			[]VgBackupRunStatus{
				VgRunPending, VgRunRunning,
				VgRunUploading, VgRunVerifying,
			}).
		Count(&inflight)
	if inflight > 0 {
		return
	}

	boundary := scheduledFor.UTC()
	run := VgBackupRun{
		BackupJobID:     job.ID,
		AgentID:         job.AgentID,
		StorageTargetID: job.StorageTargetID,
		Status:          VgRunPending,
		SourceType:      string(job.SourceType),
		ScheduledFor:    &boundary,
	}

	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "backup_job_id"},
			{Name: "scheduled_for"},
		},
		DoNothing: true,
	}).Create(&run)
	if res.Error != nil {
		log.Printf("scheduler: create run failed for job %s: %v", job.ID, res.Error)
		return
	}
}

const (
	staleRunAfter  = 15 * time.Minute
	agentGoneAfter = 3 * time.Minute
)

// CheckStaleRuns marks active runs as FAILED/CANCELLED when the executing agent has gone offline.
func (s *Scheduler) CheckStaleRuns(ctx context.Context, now time.Time) {
	if s.db == nil {
		return
	}
	cutoff := now.Add(-staleRunAfter)
	var runs []VgBackupRun
	if err := s.db.WithContext(ctx).Where("status IN ? AND updated_at < ?",
		[]VgBackupRunStatus{VgRunRunning, VgRunUploading, VgRunVerifying}, cutoff).
		Find(&runs).Error; err != nil {
		return
	}

	for _, run := range runs {
		if ctx.Err() != nil {
			return
		}
		var agent VgAgent
		if err := s.db.WithContext(ctx).Where("id = ?", run.AgentID).First(&agent).Error; err != nil {
			continue
		}
		if agent.LastSeenAt != nil && now.Sub(*agent.LastSeenAt) < agentGoneAfter {
			continue
		}

		status := VgRunFailed
		errMsg := "agent stopped reporting; run marked failed by background scheduler"
		if run.CancelRequested {
			status = VgRunCancelled
			errMsg = "agent stopped reporting after cancel request; run marked cancelled"
		}

		duration := int64(0)
		if run.StartedAt != nil {
			duration = int64(now.Sub(*run.StartedAt).Seconds())
		}

		s.db.WithContext(ctx).Model(&VgBackupRun{}).
			Where("id = ? AND status = ?", run.ID, run.Status).
			Updates(map[string]interface{}{
				"status":           status,
				"completed_at":     &now,
				"error_message":    errMsg,
				"failure_category": "AGENT_OFFLINE",
				"duration_seconds": duration,
			})
	}
}

// EnforceRetention cleans up expired completed, failed, and cancelled backup runs,
// deleting local storage files only when within a safe validated storage root.
func (s *Scheduler) EnforceRetention(ctx context.Context, now time.Time) {
	if s.db == nil {
		return
	}
	var jobs []VgBackupJob
	// Retention resolves policy-first like every other execution path:
	// an attached policy's RetentionDays governs, otherwise the job's
	// inline value. Reading the job row directly would silently apply
	// the wrong lifetime (the row keeps its creation-time default once
	// a policy is attached).
	if err := s.db.WithContext(ctx).Preload("Policy").Find(&jobs).Error; err != nil {
		return
	}

	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		eff := EffectivePolicy(&job)
		if eff.RetentionDays <= 0 {
			continue
		}
		cutoff := now.AddDate(0, 0, -eff.RetentionDays)

		var expiredRuns []VgBackupRun
		s.db.WithContext(ctx).Where("backup_job_id = ? AND status IN ? AND completed_at IS NOT NULL AND completed_at < ?",
			job.ID,
			[]VgBackupRunStatus{VgRunCompleted, VgRunFailed, VgRunCancelled},
			cutoff).Find(&expiredRuns)

		for _, run := range expiredRuns {
			if ctx.Err() != nil {
				return
			}
			// Check if run belongs to any currently active restore
			var activeRestores int64
			s.db.WithContext(ctx).Model(&VgRestoreJob{}).
				Where("backup_run_id = ? AND status IN ?", run.ID,
					[]VgRestoreStatus{VgRestorePending, VgRestoreRunning}).
				Count(&activeRestores)
			if activeRestores > 0 {
				continue
			}

			s.deleteRunFiles(ctx, run)

			// Delete chunks -> artifacts -> run in order
			var artifacts []VgBackupArtifact
			s.db.WithContext(ctx).Where("backup_run_id = ?", run.ID).Find(&artifacts)
			for _, a := range artifacts {
				s.db.WithContext(ctx).Where("artifact_id = ?", a.ID).Delete(&VgBackupChunk{})
			}
			s.db.WithContext(ctx).Where("backup_run_id = ?", run.ID).Delete(&VgBackupArtifact{})
			s.db.WithContext(ctx).Delete(&run)
		}
	}
}

func (s *Scheduler) deleteRunFiles(ctx context.Context, run VgBackupRun) {
	var target VgStorageTarget
	if err := s.db.WithContext(ctx).Where("id = ?", run.StorageTargetID).First(&target).Error; err != nil {
		return
	}
	if target.Type != VgStorageLocal {
		return
	}
	if target.Path == "" || !filepath.IsAbs(target.Path) {
		return
	}

	// Conservative file deletion: artifact paths are AGENT-local strings
	// that only sometimes name server-local files. Without an explicitly
	// configured server-side delete root there is no safe way to tell the
	// two apart, so only database rows are pruned and bytes are left for
	// the operator (or bucket lifecycle rules). This is deliberate.
	deleteRoot := ""
	if s.deleteRoot != "" && filepath.IsAbs(s.deleteRoot) {
		deleteRoot = filepath.Clean(s.deleteRoot)
	}

	cleanRoot := filepath.Clean(target.Path)
	seen := map[string]bool{}
	paths := []string{run.StoragePath}

	var artifacts []VgBackupArtifact
	s.db.WithContext(ctx).Where("backup_run_id = ?", run.ID).Find(&artifacts)
	for _, a := range artifacts {
		paths = append(paths, a.StoragePath)
	}

	for _, p := range paths {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true

		if err := validateSafeStoragePath(cleanRoot, p); err != nil {
			log.Printf("retention: unsafe storage path %q skipped: %v", p, err)
			continue
		}
		if deleteRoot == "" {
			log.Printf("retention: file deletion disabled (no delete root configured), keeping %q", p)
			continue
		}
		if err := validateSafeStoragePath(deleteRoot, p); err != nil {
			log.Printf("retention: path %q outside delete root %q, keeping file", p, deleteRoot)
			continue
		}
		_ = os.Remove(p)
	}
}

// validateSafeStoragePath ensures path p is inside root, does not contain '..',
// and does not escape root via symlinks.
func validateSafeStoragePath(root, p string) error {
	if !filepath.IsAbs(p) {
		return fmt.Errorf("path is not absolute")
	}
	cleanP := filepath.Clean(p)
	cleanRoot := filepath.Clean(root)

	// Check raw path prefix
	if cleanP != cleanRoot && !strings.HasPrefix(cleanP, cleanRoot+string(filepath.Separator)) {
		return fmt.Errorf("path escapes storage root")
	}

	// Verify symlinks if the target file or ancestor exists
	realRoot, err := filepath.EvalSymlinks(cleanRoot)
	if err != nil {
		return fmt.Errorf("eval symlink root: %w", err)
	}

	// If file exists, check its evaluated symlink
	if realP, err := filepath.EvalSymlinks(cleanP); err == nil {
		if realP != realRoot && !strings.HasPrefix(realP, realRoot+string(filepath.Separator)) {
			return fmt.Errorf("symlink escapes storage root")
		}
	} else if os.IsNotExist(err) {
		// If file doesn't exist, check parent dir
		parent := filepath.Dir(cleanP)
		if realParent, perr := filepath.EvalSymlinks(parent); perr == nil {
			if realParent != realRoot && !strings.HasPrefix(realParent, realRoot+string(filepath.Separator)) {
				return fmt.Errorf("parent symlink escapes storage root")
			}
		}
	}

	return nil
}

// verifyBatchLimit caps how many COMPLETED runs one verification pass
// settles: hashing is I/O-bound and the tick mutex is held, so the pass
// must stay a bounded background chore, never an unbounded sweep.
const verifyBatchLimit = 50

// VerifyCompletedRuns settles PENDING artifact verification for COMPLETED
// runs whose resolved policy enables verification. This is the automatic
// half of the verification contract:
//
//   - artifact uploaded (COMPLETED) = the agent *reported* success. It
//     proves nothing by itself.
//   - artifact integrity verified (VERIFIED) = the server re-hashed bytes
//     it could read and the checksum matched. Only server-readable LOCAL
//     artifacts can reach this state here.
//   - restore-verified = a restore drill replayed the backup into an
//     isolated target and the agent reported success. NOT implemented:
//     drills need scratch destinations plus a verdict store (see the
//     report); nothing here may be read as restore proof.
//
// Anything that cannot be checked server-side (remote-agent paths, S3/SMB
// targets, missing checksums) stays PENDING for a later pass or the manual
// Verify button — an unreadable file is "not checked", never a failure.
func (s *Scheduler) VerifyCompletedRuns(ctx context.Context, now time.Time) {
	if s.db == nil {
		return
	}
	var runs []VgBackupRun
	if err := s.db.WithContext(ctx).Preload("BackupJob.Policy").
		Where("status = ?", VgRunCompleted).
		Order("completed_at ASC").Limit(verifyBatchLimit).
		Find(&runs).Error; err != nil {
		return
	}
	for i := range runs {
		if ctx.Err() != nil {
			return
		}
		run := &runs[i]
		if !EffectivePolicy(&run.BackupJob).VerificationEnabled {
			continue
		}
		var target VgStorageTarget
		if err := s.db.WithContext(ctx).Where("id = ?", run.StorageTargetID).First(&target).Error; err != nil {
			continue
		}
		if target.Type != VgStorageLocal {
			continue
		}
		var arts []VgBackupArtifact
		s.db.WithContext(ctx).
			Where("backup_run_id = ? AND verification_status = ?", run.ID, VgArtifactUnverified).
			Find(&arts)
		for _, a := range arts {
			if ctx.Err() != nil {
				return
			}
			p := a.StoragePath
			if p == "" {
				p = run.StoragePath
			}
			expected := a.Checksum
			if expected == "" {
				expected = run.Checksum
			}
			if p == "" || expected == "" {
				continue
			}
			sum, err := sha256File(p)
			if err != nil {
				// Not server-readable (remote-agent path): stay PENDING.
				continue
			}
			status := VgArtifactVerified
			if sum != strings.ToLower(expected) {
				status = VgArtifactVerifyFail
				log.Printf("scheduler: artifact %s checksum mismatch (run %s)", a.ID, run.ID)
			}
			s.db.WithContext(ctx).Model(&VgBackupArtifact{}).Where("id = ?", a.ID).
				Updates(map[string]interface{}{"verification_status": status, "verified_at": &now})
		}
	}
}

// sha256File returns the hex SHA-256 of the file at path.
func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// UpdateSizeBaselines updates the rolling size baselines used for anomaly detection.
func (s *Scheduler) UpdateSizeBaselines(ctx context.Context, now time.Time) {
	if s.db == nil {
		return
	}
	var jobs []VgBackupJob
	if err := s.db.WithContext(ctx).Find(&jobs).Error; err != nil {
		return
	}

	for _, job := range jobs {
		if ctx.Err() != nil {
			return
		}
		var sizes []int64
		s.db.WithContext(ctx).Model(&VgBackupRun{}).
			Where("backup_job_id = ? AND status = ? AND bytes_uploaded > 0", job.ID, VgRunCompleted).
			Order("completed_at DESC").Limit(10).
			Pluck("bytes_uploaded", &sizes)
		if len(sizes) < 3 {
			continue
		}
		var sum int64
		for _, sz := range sizes {
			sum += sz
		}
		avg := sum / int64(len(sizes))

		var baseline VgBackupSizeBaseline
		if s.db.WithContext(ctx).Where("backup_job_id = ?", job.ID).First(&baseline).Error != nil {
			s.db.WithContext(ctx).Create(&VgBackupSizeBaseline{
				BackupJobID:   job.ID,
				AvgBytes:      avg,
				SampleCount:   len(sizes),
				LastUpdatedAt: now,
			})
		} else {
			s.db.WithContext(ctx).Model(&baseline).Updates(map[string]interface{}{
				"avg_bytes":       avg,
				"sample_count":    len(sizes),
				"last_updated_at": now,
			})
		}
	}
}
