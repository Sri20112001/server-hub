package handlers

// Regression tests for the integration-audit fixes: job/policy write-time
// validation (source type, mode, cron, timezone, retention, SLA targets,
// filesystem source_path), job execution-identity immutability on update,
// and heartbeat-derived presence in the dashboard.

import (
	"net/http"
	"testing"
	"time"

	"serverhub/internal/vgbackup"
)

func TestVgMgmtJobCreateValidation(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "val-agent")
	tgt := mgSeedTarget(t, db, op1, "val-tgt", t.TempDir())
	base := func() map[string]interface{} {
		return map[string]interface{}{
			"name": "v", "agent_id": agent.ID.String(), "storage_target_id": tgt.ID.String(),
			"source_type": "FILESYSTEM", "source_path": "/data",
		}
	}
	// Baseline valid create (with cron + timezone).
	b := base()
	b["cron_expr"] = "0 2 * * *"
	b["timezone"] = "America/New_York"
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", b, opC); w.Code != http.StatusCreated {
		t.Fatalf("baseline create: want 201, got %d %s", w.Code, w.Body.String())
	}

	bad := []struct {
		name   string
		mutate func(map[string]interface{})
	}{
		{"unknown source_type", func(m map[string]interface{}) { m["source_type"] = "TAPE" }},
		{"lowercase source_type rejected (exact uppercase literals only)", func(m map[string]interface{}) { m["source_type"] = "filesystem" }},
		{"unknown mode", func(m map[string]interface{}) { m["mode"] = "TURBO" }},
		{"negative retention", func(m map[string]interface{}) { m["retention_days"] = -5 }},
		{"huge retention", func(m map[string]interface{}) { m["retention_days"] = 99999 }},
		{"bad cron", func(m map[string]interface{}) { m["cron_expr"] = "not-a-cron" }},
		{"bad timezone", func(m map[string]interface{}) { m["timezone"] = "Mars/Olympus" }},
		{"negative SLA", func(m map[string]interface{}) { m["sla_target_minutes"] = -1 }},
		{"huge RPO", func(m map[string]interface{}) { m["rpo_target_minutes"] = 99999999 }},
		{"filesystem without path", func(m map[string]interface{}) { delete(m, "source_path") }},
		{"dbf without path", func(m map[string]interface{}) { m["source_type"] = "DBF"; delete(m, "source_path") }},
	}
	for _, tc := range bad {
		m := base()
		tc.mutate(m)
		if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", m, opC); w.Code != http.StatusBadRequest {
			t.Errorf("%s: want 400, got %d %s", tc.name, w.Code, w.Body.String())
		}
	}
}

func TestVgMgmtJobUpdateImmutability(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "imm-agent")
	agent2 := mgSeedAgent(t, db, op1, "imm-agent-2")
	tgt := mgSeedTarget(t, db, op1, "imm-tgt", t.TempDir())
	tgt2 := mgSeedTarget(t, db, op1, "imm-tgt-2", t.TempDir())
	job := mgSeedJob(t, db, op1, agent.ID, tgt.ID)

	full := func() map[string]interface{} {
		return map[string]interface{}{
			"name": "job", "agent_id": agent.ID.String(), "storage_target_id": tgt.ID.String(),
			"source_type": "FILESYSTEM", "source_path": "/data", "mode": "NORMAL",
		}
	}
	put := func(m map[string]interface{}) int {
		w := doReq(t, r, "PUT", "/server-hub/api/db-backups/jobs/"+job.ID.String(), m, opC)
		return w.Code
	}

	m := full()
	m["agent_id"] = agent2.ID.String()
	if code := put(m); code != http.StatusBadRequest {
		t.Errorf("agent reassignment: want 400, got %d", code)
	}
	m = full()
	m["storage_target_id"] = tgt2.ID.String()
	if code := put(m); code != http.StatusBadRequest {
		t.Errorf("storage reassignment: want 400, got %d", code)
	}
	m = full()
	m["source_type"] = "POSTGRES"
	m["source_database"] = "app"
	if code := put(m); code != http.StatusBadRequest {
		t.Errorf("source_type change: want 400, got %d", code)
	}
	m = full()
	m["cron_expr"] = "bogus"
	if code := put(m); code != http.StatusBadRequest {
		t.Errorf("bad cron on update: want 400, got %d", code)
	}
	m = full()
	m["mode"] = "TURBO"
	if code := put(m); code != http.StatusBadRequest {
		t.Errorf("bad mode on update: want 400, got %d", code)
	}

	// Same identity values + blank mode/path keep the stored row intact.
	m = full()
	m["mode"] = ""
	m["source_path"] = "/data"
	if code := put(m); code != http.StatusOK {
		t.Fatalf("set path: want 200, got %d", code)
	}
	m = full()
	m["mode"] = ""
	m["source_path"] = ""
	if code := put(m); code != http.StatusOK {
		t.Fatalf("no-op update: want 200, got %d", code)
	}
	var got vgbackup.VgBackupJob
	if err := db.GDB.Where("id = ?", job.ID).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.AgentID != agent.ID || got.StorageTargetID != tgt.ID || got.SourceType != vgbackup.VgSourceFilesystem {
		t.Errorf("identity mutated by no-op update: %+v", got)
	}
	if got.Mode != vgbackup.VgModeNormal {
		t.Errorf("blank mode must keep stored mode, got %q", got.Mode)
	}
	if got.SourcePath != "/data" {
		t.Errorf("blank source_path must keep stored path, got %q", got.SourcePath)
	}
}

func TestVgMgmtPolicyTimezoneValidation(t *testing.T) {
	_, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")

	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/policies", map[string]interface{}{
		"name": "bad-tz", "timezone": "Mars/Olympus",
	}, opC); w.Code != http.StatusBadRequest {
		t.Errorf("bad policy timezone: want 400, got %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/policies", map[string]interface{}{
		"name": "good-tz", "timezone": "America/Chicago", "cron_expr": "0 3 * * *",
	}, opC); w.Code != http.StatusCreated {
		t.Errorf("good policy timezone: want 201, got %d %s", w.Code, w.Body.String())
	}
}

func TestVgMgmtDashboardPresence(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	now := time.Now()
	fresh := mgSeedAgent(t, db, op1, "fresh-agent")
	db.GDB.Model(&vgbackup.VgAgent{}).Where("id = ?", fresh.ID).
		Updates(map[string]interface{}{"status": vgbackup.VgAgentOnline, "last_seen_at": &now})
	stale := mgSeedAgent(t, db, op1, "stale-agent")
	old := now.Add(-10 * time.Minute)
	// Stored flag says ONLINE but the heartbeat is stale: presence must
	// derive from recency, so this agent counts as offline.
	db.GDB.Model(&vgbackup.VgAgent{}).Where("id = ?", stale.ID).
		Updates(map[string]interface{}{"status": vgbackup.VgAgentOnline, "last_seen_at": &old})
	revoked := mgSeedAgent(t, db, op1, "revoked-agent")
	db.GDB.Model(&vgbackup.VgAgent{}).Where("id = ?", revoked.ID).
		Updates(map[string]interface{}{"status": vgbackup.VgAgentOffline, "last_seen_at": &now, "revoked_at": &now})

	w := doReq(t, r, "GET", "/server-hub/api/db-backups/dashboard", nil, opC)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: want 200, got %d", w.Code)
	}
	out := parseJSON(t, w)
	if got := int(out["agents_online"].(float64)); got != 1 {
		t.Errorf("agents_online: want 1 (only the fresh agent), got %d", got)
	}
	if got := int(out["agents_offline"].(float64)); got != 2 {
		t.Errorf("agents_offline: want 2 (stale + revoked), got %d", got)
	}
}

func TestVgMgmtDeletePolicyMaterializes(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	adminC := loginAs(t, r, "admin", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "mat-agent")
	tgt := mgSeedTarget(t, db, op1, "mat-tgt", t.TempDir())
	policy := vgbackup.VgBackupPolicy{
		Name: "mat-policy", Strategy: vgbackup.VgStrategyFull,
		CronExpr: "0 3 * * *", Timezone: "UTC", Enabled: true,
		Mode:             vgbackup.VgModeCompressed,
		Encrypted:        true,
		RetentionDays:    7,
		SLATargetMinutes: 60,
		MaxRetries:       3,
		CreatedByUserID:  op1,
	}
	if err := db.GDB.Create(&policy).Error; err != nil {
		t.Fatal(err)
	}
	job := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	db.GDB.Model(&vgbackup.VgBackupJob{}).Where("id = ?", job.ID).Updates(map[string]interface{}{
		"policy_id": policy.ID, "retention_days": 30, "mode": vgbackup.VgModeNormal,
	})
	// Job's own schedule row differs from the policy's: detach must adopt
	// the policy's schedule so the job keeps firing as before.
	db.GDB.Create(&vgbackup.VgBackupSchedule{BackupJobID: job.ID, CronExpr: "0 1 * * *", Timezone: "UTC"})

	w := doReq(t, r, "DELETE", "/server-hub/api/db-backups/policies/"+policy.ID.String(), nil, adminC)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete policy: want 204, got %d %s", w.Code, w.Body.String())
	}
	var got vgbackup.VgBackupJob
	if err := db.GDB.Where("id = ?", job.ID).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.PolicyID != nil {
		t.Error("job must be detached (policy_id nil)")
	}
	if got.RetentionDays != 7 || got.Mode != vgbackup.VgModeCompressed || !got.Encrypted ||
		got.SLATargetMinutes != 60 {
		t.Errorf("inline settings must materialize the policy, got %+v", got)
	}
	var sched vgbackup.VgBackupSchedule
	if err := db.GDB.Where("backup_job_id = ?", job.ID).First(&sched).Error; err != nil {
		t.Fatal(err)
	}
	if sched.CronExpr != "0 3 * * *" {
		t.Errorf("schedule row must adopt the policy cron, got %q", sched.CronExpr)
	}
}

func TestVgMgmtDashboardBreaches(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "breach-agent")
	tgt := mgSeedTarget(t, db, op1, "breach-tgt", t.TempDir())

	// Job A: RPO 60min, last success 2h ago → RPO breach.
	jobA := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	db.GDB.Model(&vgbackup.VgBackupJob{}).Where("id = ?", jobA.ID).Update("rpo_target_minutes", 60)
	old := time.Now().Add(-2 * time.Hour)
	db.GDB.Create(&vgbackup.VgBackupRun{
		BackupJobID: jobA.ID, AgentID: agent.ID, StorageTargetID: tgt.ID,
		Status: vgbackup.VgRunCompleted, CompletedAt: &old, DurationSeconds: 30,
	})

	// Job B: SLA 1min, last success took 3600s → SLA breach (fresh → no RPO).
	jobB := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	db.GDB.Model(&vgbackup.VgBackupJob{}).Where("id = ?", jobB.ID).Updates(map[string]interface{}{
		"sla_target_minutes": 1, "rpo_target_minutes": 10080,
	})
	now := time.Now()
	db.GDB.Create(&vgbackup.VgBackupRun{
		BackupJobID: jobB.ID, AgentID: agent.ID, StorageTargetID: tgt.ID,
		Status: vgbackup.VgRunCompleted, CompletedAt: &now, DurationSeconds: 3600,
	})

	// Job C: no targets → never breaches.
	mgSeedJob(t, db, op1, agent.ID, tgt.ID)

	w := doReq(t, r, "GET", "/server-hub/api/db-backups/dashboard", nil, opC)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: want 200, got %d", w.Code)
	}
	out := parseJSON(t, w)
	rpo, ok := out["rpo_breaches"].([]interface{})
	if !ok || len(rpo) != 1 {
		t.Fatalf("want exactly 1 RPO breach, got %v", out["rpo_breaches"])
	}
	if rpo[0].(map[string]interface{})["job_id"] != jobA.ID.String() {
		t.Errorf("RPO breach must name job A, got %v", rpo[0])
	}
	sla, ok := out["sla_breaches"].([]interface{})
	if !ok || len(sla) != 1 {
		t.Fatalf("want exactly 1 SLA breach, got %v", out["sla_breaches"])
	}
	if sla[0].(map[string]interface{})["job_id"] != jobB.ID.String() {
		t.Errorf("SLA breach must name job B, got %v", sla[0])
	}
}
