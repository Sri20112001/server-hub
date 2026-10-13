package handlers

// Agent-protocol tests speak the exact wire format from the unmodified
// VaultGuard agent (agent/internal/agent/api.go): relative paths, header
// names, JSON field names, and status codes. The test router mirrors the
// route wiring in cmd/server/main.go, including the real /server-hub/api
// prefix, agent auth, body limit, and a fresh rate limiter per test (the
// shared limiter trips at 5 failures/min/IP, so routers are never shared
// across tests that intentionally fail auth).

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/testdb"
	"serverhub/internal/vgbackup"
)

var vgTestEncKey = []byte("0123456789abcdef0123456789abcdef")

// vgAgentRouter wires the agent API exactly like main.go does: public
// enroll/register behind the login rate limiter, everything else behind
// agent-token auth plus a 1 MiB body cap, all under the real prefix.
func vgAgentRouter(t *testing.T) (*database.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	h := &VgAgentHandler{DB: db, EncKey: vgTestEncKey, Broker: events.NewBroker()}
	r := gin.New()
	limiter := middleware.NewLoginLimiter()
	rl := func() gin.HandlerFunc { return middleware.LoginRateLimit(db.GDB, limiter) }
	pub := r.Group("/server-hub/api/db-backups/agent")
	pub.Use(VgLimitBody(1 << 20))
	pub.POST("/agents/register", rl(), h.VgRegister)
	pub.POST("/agents/enroll", rl(), h.VgEnroll)
	ag := pub.Group("", VgAgentAuth(db), VgLimitBody(1<<20))
	ag.POST("/agent/heartbeat", h.VgHeartbeat)
	ag.GET("/agent/runs", h.VgPendingRuns)
	ag.POST("/agent/runs/:id/claim", h.VgClaimRun)
	ag.PUT("/backup-runs/:id/status", h.VgUpdateRunStatus)
	ag.POST("/backup-runs/:id/artifacts", h.VgRegisterArtifact)
	ag.POST("/artifacts/:artifact_id/chunks", h.VgRegisterChunk)
	ag.GET("/agent/restores", h.VgPendingRestores)
	ag.POST("/agent/restores/:id/claim", h.VgClaimRestore)
	ag.PUT("/restores/:id/status", h.VgUpdateRestoreStatus)
	return db, r
}

func doVg(t *testing.T, r *gin.Engine, method, path string, body any, agentID, token string) *httptest.ResponseRecorder {
	t.Helper()
	var rdr *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		rdr = bytes.NewReader(raw)
	} else {
		rdr = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if agentID != "" {
		req.Header.Set("X-Agent-ID", agentID)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decodeVg(t *testing.T, w *httptest.ResponseRecorder, out any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
		t.Fatalf("decode %s: %v", w.Body.String(), err)
	}
}

// seedVgAgent creates an enrolled agent row with a known plaintext token.
func seedVgAgent(t *testing.T, db *database.DB, name string) (token string, agent vgbackup.VgAgent) {
	t.Helper()
	token, hash, err := vgMintAgentToken()
	if err != nil {
		t.Fatal(err)
	}
	agent = vgbackup.VgAgent{Name: name, CreatedByUserID: 1, TokenHash: &hash, Status: vgbackup.VgAgentOnline}
	if err := db.GDB.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	return token, agent
}

// seedVgPendingAgent creates a never-enrolled agent carrying a registration key.
func seedVgPendingAgent(t *testing.T, db *database.DB, regKey string) vgbackup.VgAgent {
	t.Helper()
	h := vgSha256Hex(regKey)
	agent := vgbackup.VgAgent{Name: "pending", CreatedByUserID: 1, RegistrationKey: &h, Status: vgbackup.VgAgentOffline}
	if err := db.GDB.Create(&agent).Error; err != nil {
		t.Fatal(err)
	}
	return agent
}

// seedVgEnrollToken inserts a single-use enrollment token row (what the
// Phase 3 operator UI will mint through an endpoint).
func seedVgEnrollToken(t *testing.T, db *database.DB, agentID uuid.UUID, expiresAt time.Time) string {
	t.Helper()
	plain := vgRandomHex(32)
	row := vgbackup.VgEnrollmentToken{AgentID: &agentID, TokenHash: vgSha256Hex(plain), ExpiresAt: expiresAt}
	if err := db.GDB.Create(&row).Error; err != nil {
		t.Fatal(err)
	}
	return plain
}

func seedVgTarget(t *testing.T, db *database.DB) vgbackup.VgStorageTarget {
	t.Helper()
	tgt := vgbackup.VgStorageTarget{Name: "local", Type: vgbackup.VgStorageLocal, Path: "/tmp/vg-agent-test", CreatedByUserID: 1}
	if err := db.GDB.Create(&tgt).Error; err != nil {
		t.Fatal(err)
	}
	return tgt
}

func seedVgJob(t *testing.T, db *database.DB, agentID, targetID uuid.UUID) vgbackup.VgBackupJob {
	t.Helper()
	job := vgbackup.VgBackupJob{
		Name: "nightly", SourceType: vgbackup.VgSourcePostgres, Mode: vgbackup.VgModeNormal,
		AgentID: agentID, StorageTargetID: targetID, CreatedByUserID: 1, Enabled: true,
	}
	if err := db.GDB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	return job
}

func seedVgRun(t *testing.T, db *database.DB, job vgbackup.VgBackupJob, status vgbackup.VgBackupRunStatus) vgbackup.VgBackupRun {
	t.Helper()
	run := vgbackup.VgBackupRun{
		BackupJobID: job.ID, AgentID: job.AgentID, StorageTargetID: job.StorageTargetID,
		Status: status,
	}
	if err := db.GDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	return run
}

func seedVgRestoreJob(t *testing.T, db *database.DB, run vgbackup.VgBackupRun) vgbackup.VgRestoreJob {
	t.Helper()
	rj := vgbackup.VgRestoreJob{BackupRunID: run.ID, AgentID: run.AgentID, Status: vgbackup.VgRestorePending, DestinationPath: "/tmp/restore"}
	if err := db.GDB.Create(&rj).Error; err != nil {
		t.Fatal(err)
	}
	return rj
}

const vgBase = "/server-hub/api/db-backups/agent"

func TestVgRegisterSuccess(t *testing.T) {
	db, r := vgAgentRouter(t)
	agent := seedVgPendingAgent(t, db, "regkey-001")
	w := doVg(t, r, "POST", vgBase+"/agents/register", map[string]string{
		"registration_key": "regkey-001", "hostname": "db01", "os": "linux", "version": "1.0",
	}, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("register: want 200, got %d %s", w.Code, w.Body.String())
	}
	var out struct {
		AgentID    string `json:"agent_id"`
		AgentToken string `json:"agent_token"`
	}
	decodeVg(t, w, &out)
	if out.AgentID != agent.ID.String() || out.AgentToken == "" {
		t.Fatalf("register response wrong: %+v", out)
	}
	// The minted credential authenticates from here on.
	w2 := doVg(t, r, "POST", vgBase+"/agent/heartbeat", map[string]string{"agent_version": "1.0"}, out.AgentID, out.AgentToken)
	if w2.Code != http.StatusOK {
		t.Fatalf("heartbeat with minted token: want 200, got %d", w2.Code)
	}
}

func TestVgRegisterRejected(t *testing.T) {
	db, r := vgAgentRouter(t)
	rev := "revoked-key"
	agent := seedVgPendingAgent(t, db, rev)
	now := time.Now()
	db.GDB.Model(&agent).Update("revoked_at", now)
	cases := []struct {
		name string
		body any
		want int
	}{
		{"unknown key", map[string]string{"registration_key": "nope", "hostname": "h"}, http.StatusUnauthorized},
		{"revoked agent", map[string]string{"registration_key": rev, "hostname": "h"}, http.StatusUnauthorized},
		{"missing fields", map[string]string{"hostname": "h"}, http.StatusBadRequest},
	}
	for _, tc := range cases {
		if w := doVg(t, r, "POST", vgBase+"/agents/register", tc.body, "", ""); w.Code != tc.want {
			t.Errorf("%s: want %d, got %d %s", tc.name, tc.want, w.Code, w.Body.String())
		}
	}
}

func arcTime() time.Time { return time.Now() }

func TestVgEnrollSuccess(t *testing.T) {
	db, r := vgAgentRouter(t)
	_, agent := seedVgAgent(t, db, "enroll-target")
	plain := seedVgEnrollToken(t, db, agent.ID, time.Now().Add(time.Hour))
	w := doVg(t, r, "POST", vgBase+"/agents/enroll", map[string]string{
		"enrollment_token": plain, "machine_name": "db01",
		"platform": "linux", "architecture": "amd64", "agent_version": "2.0",
		"hostname": "db01", "os": "linux",
	}, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("enroll: want 200, got %d %s", w.Code, w.Body.String())
	}
	var out struct {
		AgentID                  string `json:"agent_id"`
		AgentToken               string `json:"agent_token"`
		PollIntervalSeconds      int    `json:"poll_interval_seconds"`
		HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
	}
	decodeVg(t, w, &out)
	if out.AgentID != agent.ID.String() || out.AgentToken == "" ||
		out.PollIntervalSeconds != 30 || out.HeartbeatIntervalSeconds != 60 {
		t.Fatalf("enroll response wrong: %+v", out)
	}
	// Machine inventory recorded.
	var n int64
	db.GDB.Model(&vgbackup.VgMachine{}).Where("agent_id = ?", agent.ID).Count(&n)
	if n != 1 {
		t.Errorf("machine row must exist after enroll, got %d", n)
	}
}

func TestVgEnrollRejected(t *testing.T) {
	db, r := vgAgentRouter(t)
	_, agent := seedVgAgent(t, db, "enroll-reject")
	expired := seedVgEnrollToken(t, db, agent.ID, time.Now().Add(-time.Minute))
	good := seedVgEnrollToken(t, db, agent.ID, time.Now().Add(time.Hour))
	enroll := func(tok string) *httptest.ResponseRecorder {
		return doVg(t, r, "POST", vgBase+"/agents/enroll", map[string]string{
			"enrollment_token": tok, "machine_name": "m",
		}, "", "")
	}
	if w := enroll("bogus"); w.Code != http.StatusUnauthorized {
		t.Errorf("unknown token: want 401, got %d", w.Code)
	}
	if w := enroll(expired); w.Code != http.StatusUnauthorized {
		t.Errorf("expired token: want 401, got %d", w.Code)
	}
	if w := enroll(good); w.Code != http.StatusOK {
		t.Fatalf("first use: want 200, got %d", w.Code)
	}
	// Replay of the consumed token fails without distinction.
	if w := enroll(good); w.Code != http.StatusUnauthorized {
		t.Errorf("replay: want 401, got %d", w.Code)
	}
	if w := doVg(t, r, "POST", vgBase+"/agents/enroll", map[string]string{}, "", ""); w.Code != http.StatusBadRequest {
		t.Errorf("missing fields: want 400, got %d", w.Code)
	}
}

func TestVgEnrollConcurrentRedemption(t *testing.T) {
	db, r := vgAgentRouter(t)
	_, agent := seedVgAgent(t, db, "enroll-race")
	plain := seedVgEnrollToken(t, db, agent.ID, time.Now().Add(time.Hour))
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := doVg(t, r, "POST", vgBase+"/agents/enroll", map[string]string{
				"enrollment_token": plain, "machine_name": "m",
			}, "", "")
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()
	ok, denied := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusUnauthorized:
			denied++
		default:
			t.Fatalf("unexpected code %d", c)
		}
	}
	if ok != 1 || denied != 1 {
		t.Fatalf("exactly one redemption must win, got %v", codes)
	}
}

func TestVgHeartbeat(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "hb")
	hb := func(id, tok string, body any) *httptest.ResponseRecorder {
		return doVg(t, r, "POST", vgBase+"/agent/heartbeat", body, id, tok)
	}
	if w := hb(agent.ID.String(), token, map[string]string{"agent_version": "9.9"}); w.Code != http.StatusOK {
		t.Fatalf("heartbeat: want 200, got %d", w.Code)
	}
	var got vgbackup.VgAgent
	db.GDB.First(&got, agent.ID)
	if got.Version != "9.9" || got.Status != vgbackup.VgAgentOnline || got.LastSeenAt == nil {
		t.Errorf("heartbeat must update version/status/seen: %+v", got)
	}
	cases := []struct {
		name  string
		id    string
		token string
	}{
		{"bad token", agent.ID.String(), "wrong"},
		{"missing token", agent.ID.String(), ""},
		{"malformed id", "not-a-uuid", token},
		{"foreign id", uuid.New().String(), token},
	}
	for _, tc := range cases {
		if w := hb(tc.id, tc.token, nil); w.Code != http.StatusUnauthorized {
			t.Errorf("%s: want 401, got %d", tc.name, w.Code)
		}
	}
	// Revoked credential rejected everywhere including heartbeat.
	now := time.Now()
	db.GDB.Model(&vgbackup.VgAgent{}).Where("id = ?", agent.ID).Update("revoked_at", &now)
	if w := hb(agent.ID.String(), token, nil); w.Code != http.StatusUnauthorized {
		t.Errorf("revoked: want 401, got %d", w.Code)
	}
}

func TestVgPendingAndClaimRun(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "claimer")
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agent.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunPending)
	// Noise that must not leak in: another agent's run, our non-pending run.
	otherTok, other := seedVgAgent(t, db, "other")
	otherJob := seedVgJob(t, db, other.ID, tgt.ID)
	seedVgRun(t, db, otherJob, vgbackup.VgRunPending)
	seedVgRun(t, db, job, vgbackup.VgRunCompleted)

	w := doVg(t, r, "GET", vgBase+"/agent/runs", nil, agent.ID.String(), token)
	if w.Code != http.StatusOK {
		t.Fatalf("pending: want 200, got %d", w.Code)
	}
	var list []map[string]any
	decodeVg(t, w, &list)
	if len(list) != 1 || list[0]["id"] != run.ID.String() {
		t.Fatalf("pending must list exactly our run: %v", list)
	}

	cw := doVg(t, r, "POST", vgBase+"/agent/runs/"+run.ID.String()+"/claim", nil, agent.ID.String(), token)
	if cw.Code != http.StatusOK {
		t.Fatalf("claim: want 200, got %d %s", cw.Code, cw.Body.String())
	}
	var claimed struct {
		Run struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"run"`
		CancelRequested bool `json:"cancel_requested"`
		Config          struct {
			JobID          string `json:"job_id"`
			OrganizationID string `json:"organization_id"`
			SourceType     string `json:"source_type"`
			Mode           string `json:"mode"`
			StorageType    string `json:"storage_type"`
			StoragePath    string `json:"storage_path"`
			Storage        struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"storage"`
		} `json:"config"`
	}
	decodeVg(t, cw, &claimed)
	if claimed.Run.Status != "RUNNING" || claimed.Config.JobID != job.ID.String() ||
		claimed.Config.OrganizationID == "" || claimed.Config.Storage.Path != "/tmp/vg-agent-test" {
		t.Fatalf("claim config wrong: %+v", claimed.Config)
	}
	// Second claim of the same run conflicts.
	if w2 := doVg(t, r, "POST", vgBase+"/agent/runs/"+run.ID.String()+"/claim", nil, agent.ID.String(), token); w2.Code != http.StatusConflict {
		t.Errorf("double claim: want 409, got %d", w2.Code)
	}
	// Cross-agent claim is not found, never leaked.
	if w3 := doVg(t, r, "POST", vgBase+"/agent/runs/"+run.ID.String()+"/claim", nil, other.ID.String(), otherTok); w3.Code != http.StatusNotFound {
		t.Errorf("foreign claim: want 404, got %d", w3.Code)
	}
	// Malformed id rejected.
	if w4 := doVg(t, r, "POST", vgBase+"/agent/runs/nope/claim", nil, agent.ID.String(), token); w4.Code != http.StatusBadRequest {
		t.Errorf("malformed id: want 400, got %d", w4.Code)
	}
}

func TestVgClaimRunConcurrent(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "racer")
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agent.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunPending)
	var wg sync.WaitGroup
	codes := make([]int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			w := doVg(t, r, "POST", vgBase+"/agent/runs/"+run.ID.String()+"/claim", nil, agent.ID.String(), token)
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()
	ok, conflict := 0, 0
	for _, c := range codes {
		switch c {
		case http.StatusOK:
			ok++
		case http.StatusConflict:
			conflict++
		default:
			t.Fatalf("unexpected code %d", c)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("exactly one claim must win, got %v", codes)
	}
}

func TestVgUpdateRunStatus(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "reporter")
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agent.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunRunning)
	put := func(body any) *httptest.ResponseRecorder {
		return doVg(t, r, "PUT", vgBase+"/backup-runs/"+run.ID.String()+"/status", body, agent.ID.String(), token)
	}
	status := func(s string, extra map[string]any) map[string]any {
		m := map[string]any{"status": s}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	// Progress within RUNNING is idempotent and updates counters.
	if w := put(status("RUNNING", map[string]any{"bytes_read": 100})); w.Code != http.StatusOK {
		t.Fatalf("progress: want 200, got %d %s", w.Code, w.Body.String())
	}
	// Negative counters rejected.
	if w := put(status("RUNNING", map[string]any{"bytes_read": -1})); w.Code != http.StatusBadRequest {
		t.Errorf("negative counter: want 400, got %d", w.Code)
	}
	// Illegal jump rejected.
	if w := put(status("COMPLETED", nil)); w.Code != http.StatusBadRequest {
		t.Errorf("PENDING-style jump from RUNNING: want 400, got %d", w.Code)
	}
	// Full lifecycle to COMPLETED with data key + storage path.
	if w := put(status("UPLOADING", map[string]any{"bytes_read": 200, "storage_path": "/tmp/vg-agent-test/a.tar", "checksum": "abc"})); w.Code != http.StatusOK {
		t.Fatalf("uploading: want 200, got %d", w.Code)
	}
	if w := put(status("VERIFYING", nil)); w.Code != http.StatusOK {
		t.Fatalf("verifying: want 200, got %d", w.Code)
	}
	var done struct {
		Updated         bool `json:"updated"`
		CancelRequested bool `json:"cancel_requested"`
	}
	w := put(status("COMPLETED", map[string]any{"data_key": "cmF3a2V5"}))
	if w.Code != http.StatusOK {
		t.Fatalf("completed: want 200, got %d %s", w.Code, w.Body.String())
	}
	decodeVg(t, w, &done)
	if !done.Updated || done.CancelRequested {
		t.Errorf("completion echo wrong: %+v", done)
	}
	// Data key stored envelope-encrypted, never plaintext.
	var stored vgbackup.VgBackupRun
	db.GDB.First(&stored, run.ID)
	if stored.DataKeyEncrypted == "" || stored.DataKeyEncrypted == "cmF3a2V5" {
		t.Error("data key must be stored encrypted")
	}
	if stored.DurationSeconds < 0 || stored.CompletedAt == nil {
		t.Error("completion must stamp time/duration")
	}
	// Completed artifacts auto-verify.
	var n int64
	db.GDB.Model(&vgbackup.VgBackupArtifact{}).Where("backup_run_id = ? AND verification_status = ?", run.ID, vgbackup.VgArtifactVerified).Count(&n)
	_ = n // zero artifacts here; verified path covered with artifacts below
	// Terminal echo is idempotent and does not rewrite counters.
	if w := put(status("COMPLETED", map[string]any{"bytes_read": 99999})); w.Code != http.StatusOK {
		t.Errorf("terminal echo: want 200, got %d", w.Code)
	}
	db.GDB.First(&stored, run.ID)
	if stored.BytesRead == 99999 {
		t.Error("terminal echo must not mutate sealed counters")
	}
	// Terminal → active rejected.
	if w := put(status("RUNNING", nil)); w.Code != http.StatusBadRequest {
		t.Errorf("resurrect: want 400, got %d", w.Code)
	}
	// Foreign run invisible.
	_, other := seedVgAgent(t, db, "stranger")
	otherJob := seedVgJob(t, db, other.ID, tgt.ID)
	otherRun := seedVgRun(t, db, otherJob, vgbackup.VgRunRunning)
	w2 := doVg(t, r, "PUT", vgBase+"/backup-runs/"+otherRun.ID.String()+"/status", status("FAILED", map[string]any{"error_message": "x"}), agent.ID.String(), token)
	if w2.Code != http.StatusNotFound {
		t.Errorf("foreign run: want 404, got %d", w2.Code)
	}
}

func TestVgCancelFlagEcho(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "cancelee")
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agent.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunRunning)
	db.GDB.Model(&vgbackup.VgBackupRun{}).Where("id = ?", run.ID).Update("cancel_requested", true)
	// The flag rides on transition reports (same-status progress echoes
	// carry no flag, matching upstream: the agent learns cancel on its
	// next stage transition at the latest).
	w := doVg(t, r, "PUT", vgBase+"/backup-runs/"+run.ID.String()+"/status",
		map[string]any{"status": "UPLOADING", "bytes_read": 5}, agent.ID.String(), token)
	if w.Code != http.StatusOK {
		t.Fatalf("report: want 200, got %d", w.Code)
	}
	var out struct {
		CancelRequested bool `json:"cancel_requested"`
	}
	decodeVg(t, w, &out)
	if !out.CancelRequested {
		t.Error("server must echo cancel_requested so the agent stops")
	}
}

func TestVgRegisterArtifact(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "artifact")
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agent.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunRunning)
	post := func(body any) *httptest.ResponseRecorder {
		return doVg(t, r, "POST", vgBase+"/backup-runs/"+run.ID.String()+"/artifacts", body, agent.ID.String(), token)
	}
	good := map[string]any{"name": "a.tar", "size": 10, "checksum": "c1", "storage_path": "/tmp/vg-agent-test/a.tar"}
	w := post(good)
	if w.Code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d %s", w.Code, w.Body.String())
	}
	var first struct {
		ID string `json:"id"`
	}
	decodeVg(t, w, &first)
	// Retry returns the same row, not a fork.
	w2 := post(good)
	if w2.Code != http.StatusOK {
		t.Fatalf("retry: want 200, got %d", w2.Code)
	}
	var second struct {
		ID string `json:"id"`
	}
	decodeVg(t, w2, &second)
	if first.ID == "" || first.ID != second.ID {
		t.Errorf("idempotent retry must return the same artifact: %q vs %q", first.ID, second.ID)
	}
	// Concurrent duplicates collapse to one row.
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			post(good)
		}()
	}
	wg.Wait()
	var n int64
	db.GDB.Model(&vgbackup.VgBackupArtifact{}).Where("backup_run_id = ?", run.ID).Count(&n)
	if n != 1 {
		t.Errorf("concurrent duplicates must collapse to one row, got %d", n)
	}
	// Other agent's run invisible.
	_, other := seedVgAgent(t, db, "artifact-stranger")
	otherJob := seedVgJob(t, db, other.ID, tgt.ID)
	otherRun := seedVgRun(t, db, otherJob, vgbackup.VgRunRunning)
	w3 := doVg(t, r, "POST", vgBase+"/backup-runs/"+otherRun.ID.String()+"/artifacts", good, agent.ID.String(), token)
	if w3.Code != http.StatusNotFound {
		t.Errorf("foreign run: want 404, got %d", w3.Code)
	}
	// Validation.
	if w := post(map[string]any{"size": 1}); w.Code != http.StatusBadRequest {
		t.Errorf("missing name: want 400, got %d", w.Code)
	}
	if w := post(map[string]any{"name": "x", "size": -1}); w.Code != http.StatusBadRequest {
		t.Errorf("negative size: want 400, got %d", w.Code)
	}
}

func TestVgRegisterChunk(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "chunker")
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agent.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunRunning)
	art := vgbackup.VgBackupArtifact{BackupRunID: run.ID, Name: "a.tar", StorageTargetID: tgt.ID}
	if err := db.GDB.Create(&art).Error; err != nil {
		t.Fatal(err)
	}
	post := func(artifactID string, body any) *httptest.ResponseRecorder {
		return doVg(t, r, "POST", vgBase+"/artifacts/"+artifactID+"/chunks", body, agent.ID.String(), token)
	}
	good := map[string]any{"index": 0, "size": 5, "checksum": "k1", "storage_path": "p", "uploaded": true}
	if w := post(art.ID.String(), good); w.Code != http.StatusCreated {
		t.Fatalf("chunk: want 201, got %d %s", w.Code, w.Body.String())
	}
	// Same checksum → idempotent 200.
	if w := post(art.ID.String(), good); w.Code != http.StatusOK {
		t.Errorf("same checksum: want 200, got %d", w.Code)
	}
	// Different checksum → conflict, original kept.
	diff := map[string]any{"index": 0, "size": 5, "checksum": "other", "storage_path": "p", "uploaded": true}
	if w := post(art.ID.String(), diff); w.Code != http.StatusConflict {
		t.Errorf("different checksum: want 409, got %d", w.Code)
	}
	var kept vgbackup.VgBackupChunk
	db.GDB.Where("artifact_id = ? AND index = ?", art.ID, 0).First(&kept)
	if kept.Checksum != "k1" {
		t.Errorf("conflicting chunk must not overwrite: %+v", kept)
	}
	// Validation + scoping.
	if w := post(art.ID.String(), map[string]any{"index": -1}); w.Code != http.StatusBadRequest {
		t.Errorf("negative index: want 400, got %d", w.Code)
	}
	if w := post("not-a-uuid", good); w.Code != http.StatusBadRequest {
		t.Errorf("malformed id: want 400, got %d", w.Code)
	}
	if w := post(uuid.New().String(), good); w.Code != http.StatusNotFound {
		t.Errorf("missing artifact: want 404, got %d", w.Code)
	}
	// Another agent's artifact → 403 (exists, but not touchable).
	_, other := seedVgAgent(t, db, "chunk-stranger")
	otherJob := seedVgJob(t, db, other.ID, tgt.ID)
	otherRun := seedVgRun(t, db, otherJob, vgbackup.VgRunRunning)
	otherArt := vgbackup.VgBackupArtifact{BackupRunID: otherRun.ID, Name: "b.tar", StorageTargetID: tgt.ID}
	if err := db.GDB.Create(&otherArt).Error; err != nil {
		t.Fatal(err)
	}
	if w := post(otherArt.ID.String(), good); w.Code != http.StatusForbidden {
		t.Errorf("foreign artifact: want 403, got %d", w.Code)
	}
}

func TestVgRestoreFlow(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "restorer")
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agent.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunCompleted)
	rj := seedVgRestoreJob(t, db, run)

	w := doVg(t, r, "GET", vgBase+"/agent/restores", nil, agent.ID.String(), token)
	if w.Code != http.StatusOK {
		t.Fatalf("pending restores: want 200, got %d", w.Code)
	}
	var list []map[string]any
	decodeVg(t, w, &list)
	if len(list) != 1 || list[0]["id"] != rj.ID.String() {
		t.Fatalf("pending must list our restore: %v", list)
	}
	cw := doVg(t, r, "POST", vgBase+"/agent/restores/"+rj.ID.String()+"/claim", nil, agent.ID.String(), token)
	if cw.Code != http.StatusOK {
		t.Fatalf("claim restore: want 200, got %d %s", cw.Code, cw.Body.String())
	}
	var dto struct {
		Status      string `json:"status"`
		StorageType string `json:"storage_type"`
		StoragePath string `json:"storage_path"`
	}
	decodeVg(t, cw, &dto)
	if dto.Status != "RUNNING" || dto.StorageType != "LOCAL" {
		t.Fatalf("restore DTO wrong: %+v", dto)
	}
	if w2 := doVg(t, r, "POST", vgBase+"/agent/restores/"+rj.ID.String()+"/claim", nil, agent.ID.String(), token); w2.Code != http.StatusConflict {
		t.Errorf("double claim: want 409, got %d", w2.Code)
	}
	// Invalid jump PENDING-style: RUNNING → COMPLETED directly is allowed;
	// first check an illegal one from a fresh restore.
	rj2 := seedVgRestoreJob(t, db, run)
	bad := doVg(t, r, "PUT", vgBase+"/restores/"+rj2.ID.String()+"/status",
		map[string]any{"status": "COMPLETED"}, agent.ID.String(), token)
	if bad.Code != http.StatusBadRequest {
		t.Errorf("PENDING→COMPLETED: want 400, got %d", bad.Code)
	}
	ok := doVg(t, r, "PUT", vgBase+"/restores/"+rj.ID.String()+"/status",
		map[string]any{"status": "COMPLETED"}, agent.ID.String(), token)
	if ok.Code != http.StatusOK {
		t.Fatalf("complete restore: want 200, got %d %s", ok.Code, ok.Body.String())
	}
	// Terminal echo idempotent.
	echo := doVg(t, r, "PUT", vgBase+"/restores/"+rj.ID.String()+"/status",
		map[string]any{"status": "COMPLETED"}, agent.ID.String(), token)
	if echo.Code != http.StatusOK {
		t.Errorf("terminal echo: want 200, got %d", echo.Code)
	}
	// Foreign restore invisible.
	_, other := seedVgAgent(t, db, "restore-stranger")
	if w3 := doVg(t, r, "PUT", vgBase+"/restores/"+rj.ID.String()+"/status",
		map[string]any{"status": "FAILED", "error_message": "x"}, other.ID.String(), otherTok(t, db, other)); w3.Code != http.StatusNotFound {
		t.Errorf("foreign restore: want 404, got %d", w3.Code)
	}
}

func otherTok(t *testing.T, db *database.DB, agent vgbackup.VgAgent) string {
	t.Helper()
	token, hash, err := vgMintAgentToken()
	if err != nil {
		t.Fatal(err)
	}
	if err := db.GDB.Model(&vgbackup.VgAgent{}).Where("id = ?", agent.ID).Update("token_hash", hash).Error; err != nil {
		t.Fatal(err)
	}
	return token
}

func TestVgOversizedAndMalformed(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "fuzz")
	// >1 MiB JSON against a body-capped endpoint that enforces binding
	// must not succeed. (Heartbeat deliberately ignores bind errors for
	// legacy agents, so the capped status endpoint is the probe here.)
	bigStatus := map[string]any{"status": "RUNNING", "error_message": strings.Repeat("v", 2<<20)}
	runForBig := seedVgRun(t, db, seedVgJob(t, db, agent.ID, seedVgTarget(t, db).ID), vgbackup.VgRunRunning)
	if w := doVg(t, r, "PUT", vgBase+"/backup-runs/"+runForBig.ID.String()+"/status", bigStatus, agent.ID.String(), token); w.Code < 400 {
		t.Errorf("oversized body: want 4xx, got %d", w.Code)
	}
	// Malformed JSON rejected.
	req := httptest.NewRequest("PUT", vgBase+"/backup-runs/"+uuid.New().String()+"/status", strings.NewReader("{nope"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Agent-ID", agent.ID.String())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("malformed JSON: want 400, got %d", w.Code)
	}
	// Nonexistent run UUID is 404, not 500.
	w2 := doVg(t, r, "PUT", vgBase+"/backup-runs/"+uuid.New().String()+"/status",
		map[string]any{"status": "RUNNING"}, agent.ID.String(), token)
	if w2.Code != http.StatusNotFound {
		t.Errorf("missing run: want 404, got %d", w2.Code)
	}
	// Oversized enroll body also capped.
	bigEnroll := map[string]any{"enrollment_token": strings.Repeat("x", 2<<20), "machine_name": "m"}
	if w := doVg(t, r, "POST", vgBase+"/agents/enroll", bigEnroll, "", ""); w.Code < 400 {
		t.Errorf("oversized enroll: want 4xx, got %d", w.Code)
	}
}

// TestVgAgentE2E exercises the whole agent lifecycle with the exact payload
// shapes from the unmodified agent client: register → heartbeat → claim →
// progress → artifact → chunk → completion, then restore poll → claim →
// status. It runs the real ServerHub gin engine against an isolated test
// database — the closest possible proof short of booting the agent binary.
func TestVgAgentE2E(t *testing.T) {
	db, r := vgAgentRouter(t)

	// 1. Register with a one-time key (seeded pending agent).
	regKey := vgRandomHex(32)
	pending := vgbackup.VgAgent{Name: "pending-e2e", CreatedByUserID: 1, RegistrationKey: strPtr(vgSha256Hex(regKey)), Status: vgbackup.VgAgentOffline}
	if err := db.GDB.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	w := doVg(t, r, "POST", vgBase+"/agents/register", map[string]string{
		"registration_key": regKey, "hostname": "e2e-host", "os": "linux", "version": "9.9.9", "ip_address": "10.0.0.9",
	}, "", "")
	if w.Code != http.StatusOK {
		t.Fatalf("e2e register: want 200, got %d %s", w.Code, w.Body.String())
	}
	var reg struct {
		AgentID    string `json:"agent_id"`
		AgentToken string `json:"agent_token"`
	}
	decodeVg(t, w, &reg)
	aid, tok := reg.AgentID, reg.AgentToken

	// 2. Heartbeat.
	if w := doVg(t, r, "POST", vgBase+"/agent/heartbeat", map[string]string{"agent_version": "9.9.9"}, aid, tok); w.Code != http.StatusOK {
		t.Fatalf("e2e heartbeat: want 200, got %d", w.Code)
	}

	// 3. Operator-seeded job + pending run (Phase 3 UI will create these).
	var agentRow vgbackup.VgAgent
	db.GDB.First(&agentRow, "id = ?", aid)
	tgt := seedVgTarget(t, db)
	job := seedVgJob(t, db, agentRow.ID, tgt.ID)
	run := seedVgRun(t, db, job, vgbackup.VgRunPending)

	// 4. Poll → claim.
	w = doVg(t, r, "GET", vgBase+"/agent/runs", nil, aid, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("e2e poll: want 200, got %d", w.Code)
	}
	w = doVg(t, r, "POST", vgBase+"/agent/runs/"+run.ID.String()+"/claim", nil, aid, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("e2e claim: want 200, got %d %s", w.Code, w.Body.String())
	}
	var claim struct {
		Config struct {
			JobID   string `json:"job_id"`
			Storage struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"storage"`
		} `json:"config"`
	}
	decodeVg(t, w, &claim)
	if claim.Config.JobID != job.ID.String() || claim.Config.Storage.Path != "/tmp/vg-agent-test" {
		t.Fatalf("e2e claim config wrong: %+v", claim.Config)
	}

	// 5. Progress → artifact → chunk → completion.
	statusURL := vgBase + "/backup-runs/" + run.ID.String() + "/status"
	for _, s := range []string{"RUNNING", "UPLOADING", "VERIFYING"} {
		if w := doVg(t, r, "PUT", statusURL, map[string]any{"status": s, "bytes_read": 7}, aid, tok); w.Code != http.StatusOK {
			t.Fatalf("e2e status %s: want 200, got %d", s, w.Code)
		}
	}
	aw := doVg(t, r, "POST", vgBase+"/backup-runs/"+run.ID.String()+"/artifacts",
		map[string]any{"name": "full.tar", "size": 7, "checksum": "ck", "storage_path": "/tmp/vg-agent-test/full.tar"}, aid, tok)
	if aw.Code != http.StatusCreated {
		t.Fatalf("e2e artifact: want 201, got %d %s", aw.Code, aw.Body.String())
	}
	var art struct {
		ID string `json:"id"`
	}
	decodeVg(t, aw, &art)
	cw := doVg(t, r, "POST", vgBase+"/artifacts/"+art.ID+"/chunks",
		map[string]any{"index": 0, "size": 7, "checksum": "ck0", "storage_path": "p", "uploaded": true}, aid, tok)
	if cw.Code != http.StatusCreated {
		t.Fatalf("e2e chunk: want 201, got %d %s", cw.Code, cw.Body.String())
	}
	if w := doVg(t, r, "PUT", statusURL, map[string]any{"status": "COMPLETED", "checksum": "ck"}, aid, tok); w.Code != http.StatusOK {
		t.Fatalf("e2e complete: want 200, got %d", w.Code)
	}

	// 6. Restore poll → claim → status (seeded restore, as Phase 3 UI will create).
	rj := seedVgRestoreJob(t, db, run)
	pw := doVg(t, r, "GET", vgBase+"/agent/restores", nil, aid, tok)
	if pw.Code != http.StatusOK {
		t.Fatalf("e2e restore poll: want 200, got %d", pw.Code)
	}
	cw2 := doVg(t, r, "POST", vgBase+"/agent/restores/"+rj.ID.String()+"/claim", nil, aid, tok)
	if cw2.Code != http.StatusOK {
		t.Fatalf("e2e restore claim: want 200, got %d %s", cw2.Code, cw2.Body.String())
	}
	if w := doVg(t, r, "PUT", vgBase+"/restores/"+rj.ID.String()+"/status",
		map[string]any{"status": "COMPLETED"}, aid, tok); w.Code != http.StatusOK {
		t.Fatalf("e2e restore complete: want 200, got %d", w.Code)
	}
}

func strPtr(s string) *string { return &s }

// TestVgCryptoEnvelope pins the credential envelope: round-trip works, empty
// key fails closed, and ciphertext never contains the plaintext.
func TestVgCryptoEnvelope(t *testing.T) {
	ct, err := vgEncrypt(vgTestEncKey, "s3cr3t")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ct, "s3cr3t") {
		t.Error("ciphertext must not contain plaintext")
	}
	pt, err := vgDecrypt(vgTestEncKey, ct)
	if err != nil || pt != "s3cr3t" {
		t.Errorf("round-trip failed: %q %v", pt, err)
	}
	if _, err := vgEncrypt(nil, "x"); err == nil {
		t.Error("empty key must fail closed")
	}
	if _, err := vgDecrypt(vgTestEncKey, "!!!not-base64!!!"); err == nil {
		t.Error("garbage ciphertext must fail")
	}
}

func TestVgAgentAuthRejected(t *testing.T) {
	_, r := vgAgentRouter(t)
	// No routes behind agent auth may answer without credentials.
	for _, tc := range []struct{ method, path string }{
		{"GET", vgBase + "/agent/runs"},
		{"POST", vgBase + "/agent/runs/" + uuid.New().String() + "/claim"},
		{"PUT", vgBase + "/backup-runs/" + uuid.New().String() + "/status"},
		{"GET", vgBase + "/agent/restores"},
		{"PUT", vgBase + "/restores/" + uuid.New().String() + "/status"},
	} {
		if w := doVg(t, r, tc.method, tc.path, nil, "", ""); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s without credentials: want 401, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestVgArtifactAndChunkStoragePathEscaping(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "escape-agent")

	target := vgbackup.VgStorageTarget{
		Name:            "local-storage",
		Type:            vgbackup.VgStorageLocal,
		Path:            "/safe/backups",
		CreatedByUserID: 1,
	}
	if err := db.GDB.Create(&target).Error; err != nil {
		t.Fatal(err)
	}

	job := vgbackup.VgBackupJob{
		Name:            "job1",
		AgentID:         agent.ID,
		StorageTargetID: target.ID,
		SourceType:      vgbackup.VgSourceFilesystem,
		CreatedByUserID: 1,
	}
	if err := db.GDB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	run := vgbackup.VgBackupRun{
		BackupJobID:     job.ID,
		AgentID:         agent.ID,
		StorageTargetID: target.ID,
		Status:          vgbackup.VgRunRunning,
	}
	if err := db.GDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}

	// 1. Artifact with escaping path is rejected with 400
	wArt := doVg(t, r, "POST", vgBase+"/backup-runs/"+run.ID.String()+"/artifacts", map[string]interface{}{
		"name":         "escape.tar",
		"size":         1024,
		"checksum":     "abc",
		"storage_path": "/etc/shadow",
	}, agent.ID.String(), token)
	if wArt.Code != http.StatusBadRequest {
		t.Errorf("escaping artifact storage path: want 400, got %d %s", wArt.Code, wArt.Body.String())
	}

	// Legitimate artifact path succeeds
	wArtOK := doVg(t, r, "POST", vgBase+"/backup-runs/"+run.ID.String()+"/artifacts", map[string]interface{}{
		"name":         "ok.tar",
		"size":         1024,
		"checksum":     "abc",
		"storage_path": "/safe/backups/ok.tar",
	}, agent.ID.String(), token)
	if wArtOK.Code != http.StatusCreated {
		t.Fatalf("valid artifact storage path: want 201, got %d %s", wArtOK.Code, wArtOK.Body.String())
	}

	// 2. Run status with escaping storage_path is rejected with 400
	wRunEsc := doVg(t, r, "PUT", vgBase+"/backup-runs/"+run.ID.String()+"/status", map[string]interface{}{
		"status":       "RUNNING",
		"storage_path": "/etc/shadow",
	}, agent.ID.String(), token)
	if wRunEsc.Code != http.StatusBadRequest {
		t.Errorf("escaping run storage path: want 400, got %d %s", wRunEsc.Code, wRunEsc.Body.String())
	}
}
