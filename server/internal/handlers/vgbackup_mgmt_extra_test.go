package handlers

// Extended management-API tests: machines, credential operations, storage
// and connection probing, restore dedupe, run lifecycle, validation,
// ownership dashboards, and portable path handling.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"serverhub/internal/database"
	"serverhub/internal/vgbackup"
)

func mgSha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func mgUserID(t *testing.T, db *database.DB, username string) uint {
	t.Helper()
	var id uint
	if err := db.QueryRow(`SELECT id FROM users WHERE username=?`, username).Scan(&id); err != nil {
		t.Fatalf("user %s: %v", username, err)
	}
	return id
}

func mgSeedAgent(t *testing.T, db *database.DB, ownerID uint, name string) vgbackup.VgAgent {
	t.Helper()
	a := vgbackup.VgAgent{Name: name, CreatedByUserID: ownerID, Status: vgbackup.VgAgentOnline}
	if err := db.GDB.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	return a
}

func mgSeedTarget(t *testing.T, db *database.DB, ownerID uint, name, path string) vgbackup.VgStorageTarget {
	t.Helper()
	tgt := vgbackup.VgStorageTarget{Name: name, Type: vgbackup.VgStorageLocal, Path: path, CreatedByUserID: ownerID}
	if err := db.GDB.Create(&tgt).Error; err != nil {
		t.Fatal(err)
	}
	return tgt
}

func mgSeedJob(t *testing.T, db *database.DB, ownerID uint, agentID, targetID uuid.UUID) vgbackup.VgBackupJob {
	t.Helper()
	job := vgbackup.VgBackupJob{
		Name: "job", AgentID: agentID, StorageTargetID: targetID,
		SourceType: vgbackup.VgSourceFilesystem, Mode: vgbackup.VgModeNormal,
		Enabled: true, CreatedByUserID: ownerID,
	}
	if err := db.GDB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	return job
}

func TestVgMgmtMachines(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	op1C := loginAs(t, r, "op1", "testpass123")
	op2C := loginAs(t, r, "op2", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "m-agent")
	machine := vgbackup.VgMachine{AgentID: agent.ID, Hostname: "dbhost", OS: "linux"}
	if err := db.GDB.Create(&machine).Error; err != nil {
		t.Fatal(err)
	}

	w := doReq(t, r, "GET", "/server-hub/api/db-backups/machines", nil, op1C)
	if w.Code != http.StatusOK {
		t.Fatalf("list machines: want 200, got %d", w.Code)
	}
	w = doReq(t, r, "GET", "/server-hub/api/db-backups/machines/"+machine.ID.String(), nil, op1C)
	if w.Code != http.StatusOK {
		t.Fatalf("get machine: want 200, got %d", w.Code)
	}
	// Other owner's machine invisible.
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/machines/"+machine.ID.String(), nil, op2C); w.Code != http.StatusNotFound {
		t.Errorf("foreign machine: want 404, got %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/machines/nope", nil, op1C); w.Code != http.StatusBadRequest {
		t.Errorf("malformed id: want 400, got %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/machines", nil, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("unauth: want 401, got %d", w.Code)
	}
}

func TestVgMgmtRotateAndRegToken(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	adminC := loginAs(t, r, "admin", "testpass123")
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "rot-agent")

	// Operator cannot rotate (admin only).
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/agents/"+agent.ID.String()+"/rotate-token", nil, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator rotate: want 403, got %d", w.Code)
	}
	// Admin rotates: new secret returned once, hash rotated in storage.
	w := doReq(t, r, "POST", "/server-hub/api/db-backups/agents/"+agent.ID.String()+"/rotate-token", nil, adminC)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: want 200, got %d %s", w.Code, w.Body.String())
	}
	res := parseJSON(t, w)
	if res["agent_token"] == nil || res["agent_token"] == "" {
		t.Fatal("rotated token must be returned once")
	}
	var after vgbackup.VgAgent
	db.GDB.First(&after, agent.ID)
	if after.TokenRotatedAt == nil {
		t.Error("rotation timestamp must be recorded")
	}
	// Registration token mint: secret once, digest-only storage.
	w2 := doReq(t, r, "POST", "/server-hub/api/db-backups/agents/token", nil, adminC)
	if w2.Code != http.StatusCreated {
		t.Fatalf("mint reg token: want 201, got %d %s", w2.Code, w2.Body.String())
	}
	res2 := parseJSON(t, w2)
	key, _ := res2["registration_key"].(string)
	if key == "" {
		t.Fatal("registration key must be returned once")
	}
	var pending vgbackup.VgAgent
	if err := db.GDB.Where("registration_key = ?", vgSha256Hex(key)).First(&pending).Error; err != nil {
		t.Fatalf("pending agent must be findable by key digest: %v", err)
	}
	if pending.TokenHash != nil {
		t.Error("pending agent must have no token hash yet")
	}
	// Revoked agent rotation answers 404 (existence-safe).
	now := time.Now()
	db.GDB.Model(&vgbackup.VgAgent{}).Where("id = ?", agent.ID).Update("revoked_at", &now)
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/agents/"+agent.ID.String()+"/rotate-token", nil, adminC); w.Code != http.StatusNotFound {
		t.Errorf("revoked rotate: want 404, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/agents/nope/rotate-token", nil, adminC); w.Code != http.StatusBadRequest {
		t.Errorf("malformed id: want 400, got %d", w.Code)
	}
}

func TestVgMgmtTargetTestEndpoint(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	op1C := loginAs(t, r, "op1", "testpass123")
	op2C := loginAs(t, r, "op2", "testpass123")
	op1 := mgUserID(t, db, "op1")

	root := t.TempDir()
	tgt := mgSeedTarget(t, db, op1, "probe-me", root)
	probe := func(id string, cookies []*http.Cookie) int {
		w := doReq(t, r, "POST", "/server-hub/api/db-backups/storage-targets/test", map[string]string{"id": id}, cookies)
		return w.Code
	}
	if code := probe(tgt.ID.String(), op1C); code != http.StatusOK {
		t.Fatalf("probe real dir: want 200, got %d", code)
	}
	// Probe file must be cleaned up.
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if len(e.Name()) > 20 && e.Name()[:21] == ".serverhub-storage-t" {
			t.Errorf("probe file left behind: %q", e.Name())
		}
	}

	missing := mgSeedTarget(t, db, op1, "missing", filepath.Join(root, "no-such-dir"))
	if code := probe(missing.ID.String(), op1C); code != http.StatusBadGateway {
		t.Errorf("missing dir: want 502, got %d", code)
	}
	s3tgt := vgbackup.VgStorageTarget{Name: "s3", Type: vgbackup.VgStorageS3, Bucket: "b", CreatedByUserID: op1}
	db.GDB.Create(&s3tgt)
	if code := probe(s3tgt.ID.String(), op1C); code != http.StatusBadRequest {
		t.Errorf("s3 probe: want 400, got %d", code)
	}
	if code := probe(tgt.ID.String(), op2C); code != http.StatusNotFound {
		t.Errorf("foreign target: want 404, got %d", code)
	}
	if code := probe("", op1C); code != http.StatusBadRequest {
		t.Errorf("missing id: want 400, got %d", code)
	}
	// Symlinked target dir rejected (probe writes must not escape).
	linkDir := filepath.Join(root, "linkdir")
	realDir := filepath.Join(root, "realdir")
	if err := os.MkdirAll(realDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realDir, linkDir); err == nil {
		sym := mgSeedTarget(t, db, op1, "symlink", linkDir)
		if code := probe(sym.ID.String(), op1C); code != http.StatusBadRequest {
			t.Errorf("symlinked dir: want 400, got %d", code)
		}
	} else {
		t.Logf("symlink creation unavailable, skipping: %v", err)
	}
}

func mgSeedConnection(t *testing.T, db *database.DB, ownerID uint, agentID uuid.UUID, name string) vgbackup.VgDatabaseConnection {
	t.Helper()
	enc, err := vgEncrypt(vgTestEncKey, "pw")
	if err != nil {
		t.Fatal(err)
	}
	c := vgbackup.VgDatabaseConnection{
		Name: "c", Type: vgbackup.VgConnectionPostgres, Host: "127.0.0.1", Port: 5432,
		Username: "u", EncryptedPassword: enc, AgentID: agentID, CreatedByUserID: ownerID,
	}
	c.Name = name
	if err := db.GDB.Create(&c).Error; err != nil {
		t.Fatal(err)
	}
	return c
}

func TestVgMgmtConnectionTestEndpoint(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op2C := loginAs(t, r, "op2", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "conn-agent")
	// Live listener: TCP dial succeeds.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port
	live := mgSeedConnection(t, db, op1, agent.ID, "live")
	db.GDB.Model(&live).Updates(map[string]interface{}{"host": "127.0.0.1", "port": port})
	test := func(id string, cookies []*http.Cookie) (int, map[string]interface{}) {
		w := doReq(t, r, "POST", "/server-hub/api/db-backups/connections/test", map[string]string{"id": id}, cookies)
		var out map[string]interface{}
		_ = json.Unmarshal(w.Body.Bytes(), &out)
		return w.Code, out
	}
	// Accept connections in background so dial succeeds.
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	if code, out := test(live.ID.String(), opC); code != http.StatusOK || out["status"] != "ok" {
		t.Errorf("live TCP target: want 200 ok, got %d %v", code, out)
	}
	// Closed port → 502 with sanitized message (no DSN/credential echo).
	dead := mgSeedConnection(t, db, op1, agent.ID, "dead")
	db.GDB.Model(&dead).Updates(map[string]interface{}{"host": "127.0.0.1", "port": 1})
	if code, out := test(dead.ID.String(), opC); code != http.StatusBadGateway {
		t.Errorf("closed port: want 502, got %d", code)
	} else if s, _ := out["error"].(string); strings.Contains(s, "127.0.0.1") || strings.Contains(s, "postgres:") {
		t.Errorf("error must not echo connection details: %q", s)
	}
	if code, _ := test(live.ID.String(), op2C); code != http.StatusNotFound {
		t.Errorf("foreign connection: want 404, got %d", code)
	}
}

func TestVgMgmtConnectionNameConflict(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op2C := loginAs(t, r, "op2", "testpass123")
	op1 := mgUserID(t, db, "op1")
	op2 := mgUserID(t, db, "op2")

	agent := mgSeedAgent(t, db, op1, "conn-agent")
	body := func(name string) map[string]interface{} {
		return map[string]interface{}{
			"name": name, "type": "POSTGRES", "agent_id": agent.ID.String(),
			"host": "db", "username": "u", "password": "p",
		}
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/connections", body("dup"), opC); w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/connections", body("dup"), opC); w.Code != http.StatusConflict {
		t.Errorf("same-owner duplicate: want 409, got %d", w.Code)
	}
	// Same name under a different owner is a different namespace: allowed.
	agent2 := mgSeedAgent(t, db, op2, "conn-agent-2")
	b2 := body("dup")
	b2["agent_id"] = agent2.ID.String()
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/connections", b2, op2C); w.Code != http.StatusCreated {
		t.Errorf("cross-owner same name: want 201, got %d %s", w.Code, w.Body.String())
	}
}

func TestVgMgmtCreateRestoreDedupe(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op2C := loginAs(t, r, "op2", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "restore-agent")
	tgt := mgSeedTarget(t, db, op1, "restore-tgt", t.TempDir())
	job := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	run := vgbackup.VgBackupRun{BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: tgt.ID, Status: vgbackup.VgRunCompleted}
	db.GDB.Create(&run)
	pendRun := vgbackup.VgBackupRun{BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: tgt.ID, Status: vgbackup.VgRunRunning}
	db.GDB.Create(&pendRun)
	body := map[string]interface{}{
		"backup_run_id": run.ID.String(), "agent_id": agent.ID.String(),
		"destination_path": "/tmp/restore", "confirmed": true,
	}
	w := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", body, opC)
	if w.Code != http.StatusCreated {
		t.Fatalf("create restore: want 201, got %d %s", w.Code, w.Body.String())
	}
	first := parseJSON(t, w)["id"].(string)
	// Double submit returns the live row instead of queueing twice.
	w2 := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", body, opC)
	if w2.Code != http.StatusOK {
		t.Fatalf("duplicate restore: want 200, got %d", w2.Code)
	}
	if parseJSON(t, w2)["id"].(string) != first {
		t.Error("duplicate must return the live restore, not a new one")
	}
	// Gates.
	bad := map[string]interface{}{
		"backup_run_id": pendRun.ID.String(), "agent_id": agent.ID.String(),
		"destination_path": "/tmp/x",
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", bad, opC); w.Code != http.StatusBadRequest {
		t.Errorf("unconfirmed: want 400, got %d", w.Code)
	}
	bad["confirmed"] = true
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", bad, opC); w.Code != http.StatusBadRequest {
		t.Errorf("non-completed run: want 400, got %d", w.Code)
	}
	// Cross-user: op2 referencing op1's run/agent must not succeed.
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", body, op2C); w.Code == http.StatusCreated {
		t.Errorf("cross-user restore must not be created, got 201")
	}
}

func TestVgMgmtRunNowAndCancel(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "run-agent")
	tgt := mgSeedTarget(t, db, op1, "run-tgt", t.TempDir())
	job := mgSeedJob(t, db, op1, agent.ID, tgt.ID)

	runURL := "/server-hub/api/db-backups/jobs/" + job.ID.String() + "/run"
	w := doReq(t, r, "POST", runURL, nil, opC)
	if w.Code != http.StatusCreated {
		t.Fatalf("run-now: want 201, got %d %s", w.Code, w.Body.String())
	}
	runID := parseJSON(t, w)["id"].(string)
	// Second run while one is active conflicts.
	if w := doReq(t, r, "POST", runURL, nil, opC); w.Code != http.StatusConflict {
		t.Errorf("double run: want 409, got %d", w.Code)
	}
	cancelURL := "/server-hub/api/db-backups/runs/" + runID + "/cancel"
	if w := doReq(t, r, "POST", cancelURL, nil, opC); w.Code != http.StatusOK {
		t.Fatalf("cancel pending: want 200, got %d", w.Code)
	}
	// Idempotent re-cancel.
	if w := doReq(t, r, "POST", cancelURL, nil, opC); w.Code != http.StatusOK {
		t.Errorf("re-cancel: want 200, got %d", w.Code)
	}
	// Terminal runs cannot be cancelled.
	db.GDB.Model(&vgbackup.VgBackupRun{}).Where("id = ?", runID).Update("status", vgbackup.VgRunCompleted)
	if w := doReq(t, r, "POST", cancelURL, nil, opC); w.Code != http.StatusBadRequest {
		t.Errorf("cancel completed: want 400, got %d", w.Code)
	}
}

func TestVgMgmtPolicyValidation(t *testing.T) {
	_, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")

	bad := []map[string]interface{}{
		{"name": "x", "cron_expr": "not-a-cron"},
		{"name": "x", "strategy": "INCREMENTAL"},
		{"name": "x", "retention_days": 99999},
		{"name": "x", "max_retries": 99},
		{"name": "x", "mode": "TURBO"},
		{"cron_expr": "0 0 * * *"},
	}
	for i, b := range bad {
		if w := doReq(t, r, "POST", "/server-hub/api/db-backups/policies", b, opC); w.Code != http.StatusBadRequest {
			t.Errorf("case %d: want 400, got %d %s", i, w.Code, w.Body.String())
		}
	}
	// Disabled stays disabled (zero-value repair path).
	w := doReq(t, r, "POST", "/server-hub/api/db-backups/policies", map[string]interface{}{
		"name": "off-pol", "enabled": false, "retention_days": 7,
	}, opC)
	if w.Code != http.StatusCreated {
		t.Fatalf("create disabled: want 201, got %d", w.Code)
	}
	id := parseJSON(t, w)["id"].(string)
	w2 := doReq(t, r, "GET", "/server-hub/api/db-backups/policies/"+id, nil, opC)
	if enabled, _ := parseJSON(t, w2)["policy"].(map[string]interface{})["enabled"].(bool); enabled {
		t.Error("explicitly disabled policy must stay disabled")
	}
}

func TestVgMgmtJobValidation(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")
	op2 := mgUserID(t, db, "op2")

	agent := mgSeedAgent(t, db, op1, "job-agent")
	agent2 := mgSeedAgent(t, db, op2, "job-agent-2")
	tgt := mgSeedTarget(t, db, op1, "job-tgt", t.TempDir())
	base := func() map[string]interface{} {
		return map[string]interface{}{
			"name": "j", "agent_id": agent.ID.String(), "storage_target_id": tgt.ID.String(),
			"source_type": "FILESYSTEM", "source_path": "/data",
		}
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", base(), opC); w.Code != http.StatusCreated {
		t.Fatalf("baseline create: want 201, got %d %s", w.Code, w.Body.String())
	}
	b := base()
	b["agent_id"] = "not-a-uuid"
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", b, opC); w.Code != http.StatusBadRequest {
		t.Errorf("bad agent id: want 400, got %d", w.Code)
	}
	b = base()
	b["agent_id"] = agent2.ID.String()
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", b, opC); w.Code != http.StatusBadRequest {
		t.Errorf("foreign agent: want 400, got %d", w.Code)
	}
	b = base()
	b["source_type"] = "POSTGRES"
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", b, opC); w.Code != http.StatusBadRequest {
		t.Errorf("db job without database: want 400, got %d", w.Code)
	}
	b = base()
	b["export_format"] = "JSON"
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", b, opC); w.Code != http.StatusBadRequest {
		t.Errorf("export format on non-mongo: want 400, got %d", w.Code)
	}
	// Connection bound to another agent rejected.
	conn := mgSeedConnection(t, db, op1, agent2.ID, "mismatch")
	b = base()
	b["source_type"] = "POSTGRES"
	b["source_database"] = "app"
	b["connection_id"] = conn.ID.String()
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", b, opC); w.Code != http.StatusBadRequest {
		t.Errorf("connection-agent mismatch: want 400, got %d", w.Code)
	}
}

func TestVgMgmtRunListFilters(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "list-agent")
	tgt := mgSeedTarget(t, db, op1, "list-tgt", t.TempDir())
	jobA := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	jobB := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	mkRun := func(jobID uuid.UUID, st vgbackup.VgBackupRunStatus) {
		db.GDB.Create(&vgbackup.VgBackupRun{BackupJobID: jobID, AgentID: agent.ID, StorageTargetID: tgt.ID, Status: st})
	}
	mkRun(jobA.ID, vgbackup.VgRunCompleted)
	mkRun(jobA.ID, vgbackup.VgRunFailed)
	mkRun(jobB.ID, vgbackup.VgRunCompleted)

	get := func(q string) (int, map[string]interface{}) {
		w := doReq(t, r, "GET", "/server-hub/api/db-backups/runs"+q, nil, opC)
		return w.Code, parseJSON(t, w)
	}
	if code, out := get("?status=COMPLETED"); code != http.StatusOK || int(out["total"].(float64)) != 2 {
		t.Errorf("status filter: want total=2, got %d %v", code, out)
	}
	if code, out := get("?job_id=" + jobB.ID.String()); code != http.StatusOK || int(out["total"].(float64)) != 1 {
		t.Errorf("job filter: want total=1, got %d %v", code, out)
	}
	if code, out := get("?limit=1&page=2"); code != http.StatusOK {
		t.Errorf("pagination: want 200, got %d", code)
	} else {
		for _, k := range []string{"data", "total", "page", "limit"} {
			if _, ok := out[k]; !ok {
				t.Errorf("envelope missing %q: %v", k, out)
			}
		}
		if int(out["page"].(float64)) != 2 || len(out["data"].([]interface{})) != 1 {
			t.Errorf("page 2 must hold 1 row: %v", out)
		}
	}
}

func TestVgMgmtVerifyEndpoint(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "verify-agent")
	tgt := mgSeedTarget(t, db, op1, "verify-tgt", t.TempDir())
	job := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	content := []byte("verify-me-bytes")
	sum := mgSha256Hex(content)
	fp := filepath.Join(tgt.Path, "run.tar")
	if err := os.WriteFile(fp, content, 0600); err != nil {
		t.Fatal(err)
	}
	run := vgbackup.VgBackupRun{
		BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: tgt.ID,
		Status: vgbackup.VgRunCompleted, StoragePath: fp, Checksum: sum,
	}
	db.GDB.Create(&run)
	verifyURL := "/server-hub/api/db-backups/runs/" + run.ID.String() + "/verify"
	if w := doReq(t, r, "POST", verifyURL, nil, opC); w.Code != http.StatusOK {
		t.Fatalf("verify match: want 200, got %d %s", w.Code, w.Body.String())
	} else if parseJSON(t, w)["match"] != true {
		t.Errorf("checksum must match: %s", w.Body.String())
	}
	db.GDB.Model(&vgbackup.VgBackupRun{}).Where("id = ?", run.ID).Update("checksum", "deadbeef")
	if w := doReq(t, r, "POST", verifyURL, nil, opC); w.Code != http.StatusOK {
		t.Fatalf("verify mismatch: want 200, got %d", w.Code)
	} else if parseJSON(t, w)["match"] != false {
		t.Errorf("tampered checksum must mismatch: %s", w.Body.String())
	}
	// Guards.
	running := vgbackup.VgBackupRun{BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: tgt.ID, Status: vgbackup.VgRunRunning}
	db.GDB.Create(&running)
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/runs/"+running.ID.String()+"/verify", nil, opC); w.Code != http.StatusBadRequest {
		t.Errorf("non-completed: want 400, got %d", w.Code)
	}
	s3tgt := vgbackup.VgStorageTarget{Name: "s3v", Type: vgbackup.VgStorageS3, Bucket: "b", CreatedByUserID: op1}
	db.GDB.Create(&s3tgt)
	s3run := vgbackup.VgBackupRun{BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: s3tgt.ID, Status: vgbackup.VgRunCompleted, Checksum: sum}
	db.GDB.Create(&s3run)
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/runs/"+s3run.ID.String()+"/verify", nil, opC); w.Code != http.StatusBadRequest {
		t.Errorf("non-local: want 400, got %d", w.Code)
	}
}

func TestVgMgmtEnrollmentMintBounds(t *testing.T) {
	_, _, r := vgMgmtRouter(t)
	adminC := loginAs(t, r, "admin", "testpass123")

	w := doReq(t, r, "POST", "/server-hub/api/db-backups/enrollment-tokens", map[string]interface{}{"expires_minutes": 2000}, adminC)
	if w.Code != http.StatusBadRequest {
		t.Errorf("ttl out of range: want 400, got %d", w.Code)
	}
	w = doReq(t, r, "POST", "/server-hub/api/db-backups/enrollment-tokens", map[string]interface{}{}, adminC)
	if w.Code != http.StatusCreated {
		t.Fatalf("default mint: want 201, got %d %s", w.Code, w.Body.String())
	}
}

func TestVgMgmtPortablePaths(t *testing.T) {
	_, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")

	mk := func(path string) int {
		w := doReq(t, r, "POST", "/server-hub/api/db-backups/storage-targets", map[string]interface{}{
			"name": "p-" + strings.NewReplacer("/", "_", "\\", "_", ".", "_").Replace(path) + string(rune(len(path))),
			"type": "LOCAL", "path": path,
		}, opC)
		return w.Code
	}
	if code := mk("relative/path"); code != http.StatusBadRequest {
		t.Errorf("relative path: want 400, got %d", code)
	}
	// Slash-absolute holds on every OS (agent-local paths are slash-spelled).
	if code := mk("/var/backups/portable"); code != http.StatusCreated {
		t.Errorf("slash-absolute: want 201, got %d", code)
	}
}

func TestVgMgmtStorageInUse(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	adminC := loginAs(t, r, "admin", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent := mgSeedAgent(t, db, op1, "inuse-agent")
	tgt := mgSeedTarget(t, db, op1, "inuse-tgt", t.TempDir())
	mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	if w := doReq(t, r, "DELETE", "/server-hub/api/db-backups/storage-targets/"+tgt.ID.String(), nil, adminC); w.Code != http.StatusConflict {
		t.Errorf("in-use target: want 409, got %d", w.Code)
	}
	// Policy delete detaches instead of orphaning.
	pol := vgbackup.VgBackupPolicy{Name: "detach-me", CreatedByUserID: op1}
	db.GDB.Create(&pol)
	job2 := mgSeedJob(t, db, op1, agent.ID, tgt.ID)
	db.GDB.Model(&job2).Update("policy_id", pol.ID)
	if w := doReq(t, r, "DELETE", "/server-hub/api/db-backups/policies/"+pol.ID.String(), nil, adminC); w.Code != http.StatusNoContent {
		t.Fatalf("policy delete: want 204, got %d", w.Code)
	}
	var check vgbackup.VgBackupJob
	db.GDB.First(&check, job2.ID)
	if check.PolicyID != nil {
		t.Error("attached job must fall back to NULL policy on delete")
	}
}

func TestVgMgmtDashboardOwnership(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op2C := loginAs(t, r, "op2", "testpass123")
	adminC := loginAs(t, r, "admin", "testpass123")
	op1 := mgUserID(t, db, "op1")
	op2 := mgUserID(t, db, "op2")

	agent1 := mgSeedAgent(t, db, op1, "dash-a1")
	tgt1 := mgSeedTarget(t, db, op1, "dash-t1", t.TempDir())
	mgSeedJob(t, db, op1, agent1.ID, tgt1.ID)
	agent2 := mgSeedAgent(t, db, op2, "dash-a2")
	tgt2 := mgSeedTarget(t, db, op2, "dash-t2", t.TempDir())
	mgSeedJob(t, db, op2, agent2.ID, tgt2.ID)
	mgSeedJob(t, db, op2, agent2.ID, tgt2.ID)

	get := func(c []*http.Cookie) map[string]interface{} {
		w := doReq(t, r, "GET", "/server-hub/api/db-backups/dashboard", nil, c)
		if w.Code != http.StatusOK {
			t.Fatalf("dashboard: want 200, got %d", w.Code)
		}
		return parseJSON(t, w)
	}
	if total := get(opC)["total_jobs"].(float64); total != 1 {
		t.Errorf("op1 must see exactly own job, got %v", total)
	}
	if total := get(op2C)["total_jobs"].(float64); total != 2 {
		t.Errorf("op2 must see exactly own jobs, got %v", total)
	}
	if total := get(adminC)["total_jobs"].(float64); total != 3 {
		t.Errorf("admin must see all jobs, got %v", total)
	}
	for _, k := range []string{"enabled_jobs", "agents_online", "agents_offline", "successful_runs", "failed_runs", "total_bytes", "recent_runs"} {
		if _, ok := get(opC)[k]; !ok {
			t.Errorf("dashboard missing %q", k)
		}
	}
}
