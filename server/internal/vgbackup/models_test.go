package vgbackup

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Every backup table must carry the vg_ prefix so a name can never collide
// with a ServerHub table, and every model must declare its table
// explicitly (no GORM pluralization surprises).
func TestVgTableNames(t *testing.T) {
	tables := map[string]string{}
	for _, m := range Models() {
		tn, ok := m.(interface{ TableName() string })
		if !ok {
			t.Fatalf("%T has no TableName", m)
		}
		name := tn.TableName()
		if !strings.HasPrefix(name, "vg_") {
			t.Errorf("%T table %q missing vg_ prefix", m, name)
		}
		if prev, dup := tables[name]; dup {
			t.Errorf("duplicate table name %q (%s)", name, prev)
		}
		tables[name] = "*"
	}
	if len(tables) != len(Models()) {
		t.Errorf("Models() returned %d entries but %d unique tables", len(Models()), len(tables))
	}
}

// Ported tables must not shadow ServerHub tables. This denylist mirrors
// the tables database.OpenDatabase creates; if ServerHub adds a table that
// collides with a vg_ name, this fails loudly instead of corrupting data.
func TestVgTablesDoNotCollideWithServerHub(t *testing.T) {
	reserved := []string{
		"users", "projects", "services", "deployments", "secrets",
		"gateway_routes", "audit_logs", "operations", "backups",
		"server_snapshots", "app_logs", "db_servers", "db_registrations",
		"app_settings", "refresh_tokens", "server_groups", "managed_servers",
		"agent_tokens", "server_metrics", "alerts", "in_app_notifications",
		"health_checks", "health_check_results", "notification_groups",
		"notification_group_members", "notification_rules",
		"notification_rule_states", "delivery_policies", "delivery_states",
		"delivery_outbox", "delivery_attempts", "provider_states",
		"digest_batches", "maintenance_windows",
		"nv_users", "nv_sessions", "nv_activity", "nv_app_logs",
	}
	ours := map[string]bool{}
	for _, m := range Models() {
		ours[m.(interface{ TableName() string }).TableName()] = true
	}
	for _, r := range reserved {
		if ours[r] {
			t.Errorf("backup table %q collides with a ServerHub table", r)
		}
	}
}

func TestEffectivePolicy_Inline(t *testing.T) {
	job := &VgBackupJob{
		Mode: VgModeNormal, Encrypted: true, RetentionDays: 14, Enabled: true,
		Schedule: &VgBackupSchedule{CronExpr: "0 2 * * *", Timezone: "UTC"},
	}
	got := EffectivePolicy(job)
	if got.FromPolicy || got.CronExpr != "0 2 * * *" || got.Mode != VgModeNormal ||
		!got.Encrypted || got.RetentionDays != 14 || !got.Enabled {
		t.Errorf("inline fallback wrong: %+v", got)
	}
	if !got.VerificationEnabled {
		t.Error("inline jobs must default verification on")
	}
}

func TestEffectivePolicy_Attached(t *testing.T) {
	pid := uuid.New()
	job := &VgBackupJob{
		PolicyID: &pid,
		Policy: &VgBackupPolicy{
			Mode: VgModeCompressed, Encrypted: true, RetentionDays: 60,
			Enabled: false, CronExpr: "0 3 * * *",
		},
		Mode: VgModeNormal, Enabled: true,
		Schedule: &VgBackupSchedule{CronExpr: "0 2 * * *"},
	}
	got := EffectivePolicy(job)
	if !got.FromPolicy || got.PolicyID == nil || *got.PolicyID != pid {
		t.Errorf("policy not detected: %+v", got)
	}
	if got.Mode != VgModeCompressed || got.RetentionDays != 60 || got.Enabled {
		t.Errorf("policy must win wholesale: %+v", got)
	}
	if got.CronExpr != "0 3 * * *" {
		t.Errorf("policy cron must win: %+v", got)
	}
}

func TestCanTransition(t *testing.T) {
	ok := [][2]VgBackupRunStatus{
		{VgRunPending, VgRunRunning}, {VgRunPending, VgRunCancelled},
		{VgRunRunning, VgRunUploading}, {VgRunRunning, VgRunFailed},
		{VgRunUploading, VgRunVerifying}, {VgRunVerifying, VgRunCompleted},
	}
	for _, tc := range ok {
		if !CanTransition(tc[0], tc[1]) {
			t.Errorf("CanTransition(%s → %s) must hold", tc[0], tc[1])
		}
	}
	bad := [][2]VgBackupRunStatus{
		{VgRunPending, VgRunCompleted}, {VgRunCompleted, VgRunFailed},
		{VgRunFailed, VgRunRunning}, {VgRunCancelled, VgRunPending},
		{VgRunCompleted, VgRunCompleted},
	}
	for _, tc := range bad {
		if CanTransition(tc[0], tc[1]) {
			t.Errorf("CanTransition(%s → %s) must not hold", tc[0], tc[1])
		}
	}
}

func TestCanRestoreTransition(t *testing.T) {
	if !CanRestoreTransition(VgRestorePending, VgRestoreRunning) {
		t.Error("pending → running must hold")
	}
	if !CanRestoreTransition(VgRestoreRunning, VgRestoreCompleted) {
		t.Error("running → completed must hold")
	}
	if CanRestoreTransition(VgRestoreCompleted, VgRestoreFailed) {
		t.Error("terminal states must never transition out")
	}
	if CanRestoreTransition(VgRestoreRunning, VgRestoreRunning) {
		t.Error("same-state is not a transition")
	}
}

func TestAgentLifecycle(t *testing.T) {
	now := time.Now()
	recent := now.Add(-time.Minute)
	old := now.Add(-time.Hour)
	hash := "h"
	mk := func() *VgAgent { return &VgAgent{} }

	if got := mk().Lifecycle(); got != VgLifecyclePending {
		t.Errorf("no token → PENDING, got %s", got)
	}
	rev := &VgAgent{TokenHash: &hash}
	revTime := now
	rev.RevokedAt = &revTime
	if got := rev.Lifecycle(); got != VgLifecycleRevoked {
		t.Errorf("revoked → REVOKED, got %s", got)
	}
	if got := (&VgAgent{TokenHash: &hash, LastSeenAt: &recent}).Lifecycle(); got != VgLifecycleOnline {
		t.Errorf("fresh heartbeat → ONLINE, got %s", got)
	}
	if got := (&VgAgent{TokenHash: &hash, LastSeenAt: &old}).Lifecycle(); got != VgLifecycleOffline {
		t.Errorf("stale heartbeat → OFFLINE, got %s", got)
	}
	if got := (&VgAgent{TokenHash: &hash}).Lifecycle(); got != VgLifecycleOffline {
		t.Errorf("never heartbeated → OFFLINE, got %s", got)
	}
}

func TestVgIsAbsPath(t *testing.T) {
	// Slash-absolute holds on every OS (agent-local paths are slash-spelled,
	// and the server may run on a different OS than the agent).
	for _, p := range []string{"/var/backups", "/a", "/"} {
		if !VgIsAbsPath(p) {
			t.Errorf("VgIsAbsPath(%q) must hold on any OS", p)
		}
	}
	for _, p := range []string{"", "relative/path", ".", "..", "C:rel"} {
		if VgIsAbsPath(p) {
			t.Errorf("VgIsAbsPath(%q) must not hold", p)
		}
	}
}

func TestDatabaseListAndMetadata(t *testing.T) {
	c := &VgDatabaseConnection{DatabaseNames: "a\nb\n\n"}
	if got := c.DatabaseList(); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("DatabaseList = %q", got)
	}
	c.DatabaseMetadata = MarshalDatabaseMetadata([]DatabaseInfo{{Name: "a", SizeBytes: 10, TableCount: 2}})
	meta := c.MetadataList()
	if len(meta) != 1 || meta[0].SizeBytes != 10 {
		t.Errorf("MetadataList = %+v", meta)
	}
	merged := MergeMetadataNames(meta, []string{"a", "c"})
	if len(merged) != 2 || merged[1].SizeBytes != UnknownSize {
		t.Errorf("MergeMetadataNames = %+v", merged)
	}
	if got := (&VgDatabaseConnection{}).MetadataList(); len(got) != 0 {
		t.Errorf("empty metadata must yield empty list, got %+v", got)
	}
}
