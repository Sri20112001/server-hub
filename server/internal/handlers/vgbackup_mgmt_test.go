package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/middleware"
	"serverhub/internal/testdb"
	"serverhub/internal/vgbackup"
)

func vgMgmtRouter(t *testing.T) (*database.DB, *config.Config, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)

	// Seed users for viewer, operator, admin, and a second operator
	for _, u := range [][3]string{
		{"admin", "testpass123", "admin"},
		{"op1", "testpass123", "operator"},
		{"op2", "testpass123", "operator"},
		{"view1", "testpass123", "viewer"},
	} {
		hash, _ := bcrypt.GenerateFromPassword([]byte(u[1]), bcrypt.MinCost)
		if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?,?)`,
			u[0], string(hash), u[2]); err != nil {
			t.Fatal(err)
		}
	}

	cfg := &config.Config{
		JWTSecret:     "test-jwt-secret-min-32-chars-long!!",
		EncryptionKey: testEncKey,
	}

	r := gin.New()
	authH := &AuthHandler{DB: db, Cfg: cfg}
	r.POST("/api/auth/login", authH.Login)

	authd := middleware.AuthRequired(cfg.JWTSecret)
	dbbViewer := r.Group("/server-hub/api/db-backups", authd, middleware.RequireRole(middleware.RoleViewer))
	dbbOperator := r.Group("/server-hub/api/db-backups", authd, middleware.RequireRole(middleware.RoleOperator))
	dbbAdmin := r.Group("/server-hub/api/db-backups", authd, middleware.RequireRole(middleware.RoleAdmin))

	vgMgmtH := &VgMgmtHandler{DB: db, EncKey: vgTestEncKey, Broker: events.NewBroker()}

	// Dashboard
	dbbViewer.GET("/dashboard", vgMgmtH.Dashboard)

	// Storage targets
	dbbViewer.GET("/storage-targets", vgMgmtH.ListStorageTargets)
	dbbViewer.GET("/storage-targets/:id", vgMgmtH.GetStorageTarget)
	dbbOperator.POST("/storage-targets", vgMgmtH.CreateStorageTarget)
	dbbOperator.PUT("/storage-targets/:id", vgMgmtH.UpdateStorageTarget)
	dbbOperator.POST("/storage-targets/test", vgMgmtH.TestStorageTarget)
	dbbAdmin.DELETE("/storage-targets/:id", vgMgmtH.DeleteStorageTarget)

	// Database connections
	dbbViewer.GET("/connections", vgMgmtH.ListConnections)
	dbbViewer.GET("/connections/:id", vgMgmtH.GetConnection)
	dbbOperator.POST("/connections", vgMgmtH.CreateConnection)
	dbbOperator.PUT("/connections/:id", vgMgmtH.UpdateConnection)
	dbbOperator.POST("/connections/test", vgMgmtH.TestConnection)
	dbbAdmin.DELETE("/connections/:id", vgMgmtH.DeleteConnection)

	// Policies
	dbbViewer.GET("/policies", vgMgmtH.ListPolicies)
	dbbViewer.GET("/policies/:id", vgMgmtH.GetPolicy)
	dbbOperator.POST("/policies", vgMgmtH.CreatePolicy)
	dbbOperator.PUT("/policies/:id", vgMgmtH.UpdatePolicy)
	dbbAdmin.DELETE("/policies/:id", vgMgmtH.DeletePolicy)

	// Backup jobs
	dbbViewer.GET("/jobs", vgMgmtH.ListBackupJobs)
	dbbViewer.GET("/jobs/:id", vgMgmtH.GetBackupJob)
	dbbOperator.POST("/jobs", vgMgmtH.CreateBackupJob)
	dbbOperator.PUT("/jobs/:id", vgMgmtH.UpdateBackupJob)
	dbbOperator.POST("/jobs/:id/enable", vgMgmtH.EnableBackupJob)
	dbbOperator.POST("/jobs/:id/disable", vgMgmtH.DisableBackupJob)
	dbbOperator.POST("/jobs/:id/run", vgMgmtH.RunNow)
	dbbOperator.POST("/jobs/:id/preflight", vgMgmtH.Preflight)
	dbbAdmin.DELETE("/jobs/:id", vgMgmtH.DeleteBackupJob)

	// Backup runs & artifacts
	dbbViewer.GET("/runs", vgMgmtH.ListBackupRuns)
	dbbViewer.GET("/runs/:id", vgMgmtH.GetBackupRun)
	dbbViewer.GET("/runs/:id/artifacts", vgMgmtH.ListArtifacts)
	dbbOperator.POST("/runs/:id/cancel", vgMgmtH.CancelBackupRun)
	dbbOperator.POST("/runs/:id/verify", vgMgmtH.VerifyRun)

	// Restore jobs
	dbbViewer.GET("/restores", vgMgmtH.ListRestores)
	dbbViewer.GET("/restores/:id", vgMgmtH.GetRestore)
	dbbOperator.POST("/restores", vgMgmtH.CreateRestore)

	// Agent inventory & enrollment tokens
	dbbViewer.GET("/agents", vgMgmtH.ListAgents)
	dbbViewer.GET("/agents/:id", vgMgmtH.GetAgent)
	dbbAdmin.POST("/agents/token", vgMgmtH.GenerateRegistrationToken)
	dbbAdmin.POST("/agents/:id/rotate-token", vgMgmtH.RotateAgentToken)
	dbbAdmin.POST("/agents/:id/revoke", vgMgmtH.RevokeAgent)
	dbbViewer.GET("/machines", vgMgmtH.ListMachines)
	dbbViewer.GET("/machines/:id", vgMgmtH.GetMachine)
	dbbViewer.GET("/enrollment-tokens", vgMgmtH.ListEnrollmentTokens)
	dbbAdmin.POST("/enrollment-tokens", vgMgmtH.GenerateEnrollmentToken)
	dbbAdmin.DELETE("/enrollment-tokens/:id", vgMgmtH.RevokeEnrollmentToken)

	return db, cfg, r
}

func parseJSON(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var out map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode json %q: %v", w.Body.String(), err)
	}
	return out
}

func parseJSONArray(t *testing.T, w *httptest.ResponseRecorder) []map[string]interface{} {
	t.Helper()
	var out []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatalf("failed to decode json array %q: %v", w.Body.String(), err)
	}
	return out
}

// TestVgMgmtRBACMatrix tests Viewer / Operator / Admin permissions across
// management operations.
func TestVgMgmtRBACMatrix(t *testing.T) {
	_, _, r := vgMgmtRouter(t)

	adminC := loginAs(t, r, "admin", "testpass123")
	opC := loginAs(t, r, "op1", "testpass123")
	viewC := loginAs(t, r, "view1", "testpass123")

	// 1. Unauthenticated requests are rejected with 401
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/jobs", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated request, got %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/dashboard", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for unauthenticated dashboard, got %d", w.Code)
	}

	// 2. Viewer can read dashboard and lists
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/dashboard", nil, viewC); w.Code != http.StatusOK {
		t.Fatalf("viewer dashboard: want 200, got %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/jobs", nil, viewC); w.Code != http.StatusOK {
		t.Fatalf("viewer jobs: want 200, got %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/policies", nil, viewC); w.Code != http.StatusOK {
		t.Fatalf("viewer policies: want 200, got %d", w.Code)
	}

	// 3. Viewer mutations are rejected with 403 Forbidden
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/policies", map[string]interface{}{
		"name": "view-pol", "retention_days": 7,
	}, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer create policy: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/enrollment-tokens", nil, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer create token: want 403, got %d", w.Code)
	}

	// 4. Operator can mutate policies, connections, storage targets, jobs
	wPol := doReq(t, r, "POST", "/server-hub/api/db-backups/policies", map[string]interface{}{
		"name": "op-policy", "cron_expr": "0 0 * * *", "retention_days": 14,
	}, opC)
	if wPol.Code != http.StatusCreated {
		t.Fatalf("operator create policy: want 201, got %d %s", wPol.Code, wPol.Body.String())
	}
	polRes := parseJSON(t, wPol)
	polID := polRes["id"].(string)

	// Operator is forbidden from admin-only deletions and agent token generation
	if w := doReq(t, r, "DELETE", "/server-hub/api/db-backups/policies/"+polID, nil, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator delete policy: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/enrollment-tokens", nil, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator create enrollment token: want 403, got %d", w.Code)
	}

	// 5. Admin can delete policy and generate enrollment tokens
	wTok := doReq(t, r, "POST", "/server-hub/api/db-backups/enrollment-tokens", nil, adminC)
	if wTok.Code != http.StatusCreated {
		t.Fatalf("admin mint enrollment token: want 201, got %d %s", wTok.Code, wTok.Body.String())
	}
	tokRes := parseJSON(t, wTok)
	// Mint response carries agent_id + one-time enrollment_token (never a
	// row id); resolve the row id through the metadata list.
	if tokRes["enrollment_token"] == nil || tokRes["enrollment_token"] == "" {
		t.Fatalf("expected plaintext token once on creation")
	}
	wList := doReq(t, r, "GET", "/server-hub/api/db-backups/enrollment-tokens", nil, adminC)
	tokRows := parseJSONArray(t, wList)
	if len(tokRows) == 0 {
		t.Fatalf("minted token must appear in the metadata list")
	}
	tokID := tokRows[0]["id"].(string)

	// Admin revoke enrollment token (204, empty body by design).
	if w := doReq(t, r, "DELETE", "/server-hub/api/db-backups/enrollment-tokens/"+tokID, nil, adminC); w.Code != http.StatusNoContent {
		t.Fatalf("admin revoke enrollment token: want 204, got %d", w.Code)
	}

	// Admin delete policy (204, empty body by design).
	if w := doReq(t, r, "DELETE", "/server-hub/api/db-backups/policies/"+polID, nil, adminC); w.Code != http.StatusNoContent {
		t.Fatalf("admin delete policy: want 204, got %d", w.Code)
	}
}

// TestVgMgmtOwnershipIsolation tests cross-user isolation and parent-child ownership.
func TestVgMgmtOwnershipIsolation(t *testing.T) {
	db, _, r := vgMgmtRouter(t)

	adminC := loginAs(t, r, "admin", "testpass123")
	op1C := loginAs(t, r, "op1", "testpass123")
	op2C := loginAs(t, r, "op2", "testpass123")

	// op1 creates a storage target
	wTarget := doReq(t, r, "POST", "/server-hub/api/db-backups/storage-targets", map[string]interface{}{
		"name": "op1-local",
		"type": "LOCAL",
		"path": "/var/backups",
	}, op1C)
	if wTarget.Code != http.StatusCreated {
		t.Fatalf("op1 create target: want 201, got %d %s", wTarget.Code, wTarget.Body.String())
	}
	targetID := parseJSON(t, wTarget)["id"].(string)

	// op2 cannot GET op1's target -> 404 (ownership masked)
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/storage-targets/"+targetID, nil, op2C); w.Code != http.StatusNotFound {
		t.Fatalf("op2 get op1 target: want 404, got %d", w.Code)
	}

	// op2 cannot UPDATE op1's target -> 404
	if w := doReq(t, r, "PUT", "/server-hub/api/db-backups/storage-targets/"+targetID, map[string]interface{}{
		"name": "hacked-target",
	}, op2C); w.Code != http.StatusNotFound {
		t.Fatalf("op2 update op1 target: want 404, got %d", w.Code)
	}

	// Admin CAN GET op1's target (admin bypass)
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/storage-targets/"+targetID, nil, adminC); w.Code != http.StatusOK {
		t.Fatalf("admin get op1 target: want 200, got %d", w.Code)
	}

	// Seed agent belonging to op1
	agentID := uuid.New()
	targetUUID, _ := uuid.Parse(targetID)
	agent := vgbackup.VgAgent{
		VgBase:          vgbackup.VgBase{ID: agentID},
		Name:            "agent-op1",
		Status:          vgbackup.VgAgentOnline,
		CreatedByUserID: 2, // op1 user ID
	}
	db.GDB.Create(&agent)

	// op1 creates job using their target
	wJob := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", map[string]interface{}{
		"name":              "op1-job",
		"agent_id":          agentID.String(),
		"storage_target_id": targetID,
		"source_type":       "FILESYSTEM",
		"source_path":       "/etc/config",
	}, op1C)
	if wJob.Code != http.StatusCreated {
		t.Fatalf("op1 create job: want 201, got %d %s", wJob.Code, wJob.Body.String())
	}
	jobID := parseJSON(t, wJob)["id"].(string)

	// op2 attempts to create a job pointing to op1's storage_target_id -> rejected
	wOp2Job := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", map[string]interface{}{
		"name":              "op2-job-theft",
		"agent_id":          agentID.String(),
		"storage_target_id": targetID,
		"source_type":       "FILESYSTEM",
		"source_path":       "/var/data",
	}, op2C)
	if wOp2Job.Code != http.StatusBadRequest {
		t.Fatalf("op2 using op1 storage target: want 400, got %d", wOp2Job.Code)
	}

	// op2 cannot GET op1's job
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/jobs/"+jobID, nil, op2C); w.Code != http.StatusNotFound {
		t.Fatalf("op2 get op1 job: want 404, got %d", w.Code)
	}

	// op2 cannot trigger RunNow on op1's job
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs/"+jobID+"/run", nil, op2C); w.Code != http.StatusNotFound {
		t.Fatalf("op2 run op1 job: want 404, got %d", w.Code)
	}

	// op1 triggers RunNow -> creates a Run
	wRun := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs/"+jobID+"/run", nil, op1C)
	if wRun.Code != http.StatusCreated {
		t.Fatalf("op1 run job: want 201, got %d %s", wRun.Code, wRun.Body.String())
	}
	runID := parseJSON(t, wRun)["id"].(string)

	// op2 cannot view or cancel op1's run
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/runs/"+runID, nil, op2C); w.Code != http.StatusNotFound {
		t.Fatalf("op2 get op1 run: want 404, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/runs/"+runID+"/cancel", nil, op2C); w.Code != http.StatusNotFound {
		t.Fatalf("op2 cancel op1 run: want 404, got %d", w.Code)
	}

	// Parent record validation for Restore:
	// Mark run as COMPLETED
	now := time.Now()
	db.GDB.Model(&vgbackup.VgBackupRun{}).Where("id = ?", runID).Updates(map[string]interface{}{
		"status":       vgbackup.VgRunCompleted,
		"completed_at": &now,
	})

	// op2 cannot create a restore on op1's run -> 404 (ownership masked;
	// the request carries a well-formed agent id so it reaches the gate
	// instead of failing shape validation first).
	wRestoreOp2 := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", map[string]interface{}{
		"backup_run_id": runID,
		"agent_id":      agentID.String(),
		"confirmed":     true,
	}, op2C)
	if wRestoreOp2.Code != http.StatusNotFound {
		t.Fatalf("op2 restore op1 run: want 404, got %d", wRestoreOp2.Code)
	}

	// op1 creates restore without confirmed=true -> 400
	wRestoreUnconf := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", map[string]interface{}{
		"backup_run_id": runID,
		"agent_id":      agentID.String(),
		"confirmed":     false,
	}, op1C)
	if wRestoreUnconf.Code != http.StatusBadRequest {
		t.Fatalf("op1 restore unconfirmed: want 400, got %d", wRestoreUnconf.Code)
	}

	// op1 creates restore with confirmed=true -> 201 Created
	wRestore := doReq(t, r, "POST", "/server-hub/api/db-backups/restores", map[string]interface{}{
		"backup_run_id": runID,
		"agent_id":      agentID.String(),
		"confirmed":     true,
	}, op1C)
	if wRestore.Code != http.StatusCreated {
		t.Fatalf("op1 restore confirmed: want 201, got %d %s", wRestore.Code, wRestore.Body.String())
	}
	_ = targetUUID
}

// TestVgMgmtSecretRedaction tests that passwords, encrypted data keys, and token hashes
// are never leaked in list or detail responses.
func TestVgMgmtSecretRedaction(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	viewC := loginAs(t, r, "view1", "testpass123")

	// Create a database connection with password
	agent := vgbackup.VgAgent{
		Name:            "db-agent",
		Status:          vgbackup.VgAgentOnline,
		CreatedByUserID: 2,
	}
	db.GDB.Create(&agent)

	secretPassword := "SuperSecretDatabasePassword123!"
	wConn := doReq(t, r, "POST", "/server-hub/api/db-backups/connections", map[string]interface{}{
		"name":     "prod-db",
		"type":     "POSTGRES",
		"host":     "10.0.0.1",
		"port":     5432,
		"username": "pguser",
		"password": secretPassword,
		"agent_id": agent.ID.String(),
	}, opC)
	if wConn.Code != http.StatusCreated {
		t.Fatalf("create connection: want 201, got %d %s", wConn.Code, wConn.Body.String())
	}

	// Check detail response body does not contain the plaintext password or encrypted_password
	connDetail := parseJSON(t, wConn)
	if _, hasPass := connDetail["password"]; hasPass {
		t.Fatalf("response leaked 'password' field")
	}
	if _, hasEnc := connDetail["encrypted_password"]; hasEnc {
		t.Fatalf("response leaked 'encrypted_password' field")
	}
	if bytes.Contains(wConn.Body.Bytes(), []byte(secretPassword)) {
		t.Fatalf("response contains raw password bytes")
	}

	// Check list response as viewer
	wList := doReq(t, r, "GET", "/server-hub/api/db-backups/connections", nil, viewC)
	if wList.Code != http.StatusOK {
		t.Fatalf("list connections: want 200, got %d", wList.Code)
	}
	if bytes.Contains(wList.Body.Bytes(), []byte(secretPassword)) {
		t.Fatalf("list response contains raw password bytes")
	}

	// Check token list does not leak token hash
	adminC := loginAs(t, r, "admin", "testpass123")
	wTokens := doReq(t, r, "GET", "/server-hub/api/db-backups/enrollment-tokens", nil, adminC)
	if wTokens.Code != http.StatusOK {
		t.Fatalf("list tokens: want 200, got %d", wTokens.Code)
	}
	if bytes.Contains(wTokens.Body.Bytes(), []byte("token_hash")) {
		t.Fatalf("token list leaked token_hash")
	}
}

// TestVgMgmtInvalidUUIDAndMalformedFields tests parameter validation and malformed UUIDs.
func TestVgMgmtInvalidUUIDAndMalformedFields(t *testing.T) {
	_, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")

	// Invalid UUID in path
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/jobs/not-a-uuid", nil, opC); w.Code != http.StatusBadRequest {
		t.Fatalf("get job invalid uuid: want 400, got %d", w.Code)
	}
	if w := doReq(t, r, "PUT", "/server-hub/api/db-backups/jobs/1234-invalid", map[string]interface{}{}, opC); w.Code != http.StatusBadRequest {
		t.Fatalf("put job invalid uuid: want 400, got %d", w.Code)
	}

	// Missing required fields on job creation
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", map[string]interface{}{
		"name": "", // empty name
	}, opC); w.Code != http.StatusBadRequest {
		t.Fatalf("job missing fields: want 400, got %d", w.Code)
	}

	// Nonexistent resources
	nonexistent := uuid.New().String()
	if w := doReq(t, r, "GET", "/server-hub/api/db-backups/jobs/"+nonexistent, nil, opC); w.Code != http.StatusNotFound {
		t.Fatalf("get nonexistent job: want 404, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs/"+nonexistent+"/run", nil, opC); w.Code != http.StatusNotFound {
		t.Fatalf("run nonexistent job: want 404, got %d", w.Code)
	}
}

// TestVgMgmtJobEnableDisableAndPreflight tests job enable, disable, and preflight inspection.
func TestVgMgmtJobEnableDisableAndPreflight(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")

	target := vgbackup.VgStorageTarget{
		Name:            "preflight-target",
		Type:            vgbackup.VgStorageLocal,
		Path:            t.TempDir(),
		CreatedByUserID: 2,
	}
	db.GDB.Create(&target)

	agent := vgbackup.VgAgent{
		Name:            "preflight-agent",
		Status:          vgbackup.VgAgentOnline,
		CreatedByUserID: 2,
	}
	db.GDB.Create(&agent)

	wJob := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs", map[string]interface{}{
		"name":              "preflight-job",
		"agent_id":          agent.ID.String(),
		"storage_target_id": target.ID.String(),
		"source_type":       "FILESYSTEM",
		"source_path":       "/tmp",
	}, opC)
	if wJob.Code != http.StatusCreated {
		t.Fatalf("create job: want 201, got %d", wJob.Code)
	}
	jobID := parseJSON(t, wJob)["id"].(string)

	// Disable job
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs/"+jobID+"/disable", nil, opC); w.Code != http.StatusOK {
		t.Fatalf("disable job: want 200, got %d", w.Code)
	}
	var checkJob vgbackup.VgBackupJob
	db.GDB.First(&checkJob, "id = ?", jobID)
	if checkJob.Enabled {
		t.Fatalf("expected job to be disabled")
	}

	// Enable job
	if w := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs/"+jobID+"/enable", nil, opC); w.Code != http.StatusOK {
		t.Fatalf("enable job: want 200, got %d", w.Code)
	}
	db.GDB.First(&checkJob, "id = ?", jobID)
	if !checkJob.Enabled {
		t.Fatalf("expected job to be enabled")
	}

	// Preflight check
	wPref := doReq(t, r, "POST", "/server-hub/api/db-backups/jobs/"+jobID+"/preflight", nil, opC)
	if wPref.Code != http.StatusOK {
		t.Fatalf("preflight: want 200, got %d %s", wPref.Code, wPref.Body.String())
	}
	prefRes := parseJSON(t, wPref)
	if prefRes["checks"] == nil {
		t.Fatalf("expected checks in preflight response")
	}
}

// TestUpdateBackupJobConnectionAgentValidation verifies that updating a database backup
// job with a connection that belongs to a different agent is rejected with 400.
func TestUpdateBackupJobConnectionAgentValidation(t *testing.T) {
	db, _, r := vgMgmtRouter(t)
	opC := loginAs(t, r, "op1", "testpass123")
	op1 := mgUserID(t, db, "op1")

	agent1 := mgSeedAgent(t, db, op1, "agent-1")
	agent2 := mgSeedAgent(t, db, op1, "agent-2")
	target := mgSeedTarget(t, db, op1, "target-1", "/safe/backups")

	// Create connection belonging to agent2
	conn2 := vgbackup.VgDatabaseConnection{
		Name:            "conn-agent2",
		Type:            vgbackup.VgConnectionPostgres,
		Host:            "localhost",
		Port:            5432,
		AgentID:         agent2.ID,
		CreatedByUserID: op1,
	}
	if err := db.GDB.Create(&conn2).Error; err != nil {
		t.Fatal(err)
	}

	// Create database job belonging to agent1
	job := vgbackup.VgBackupJob{
		Name:            "db-job-1",
		AgentID:         agent1.ID,
		StorageTargetID: target.ID,
		SourceType:      vgbackup.VgSourcePostgres,
		SourceDatabase:  "mydb",
		Mode:            vgbackup.VgModeNormal,
		CreatedByUserID: op1,
		Enabled:         true,
	}
	if err := db.GDB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	// Attempt to update job with conn2 (which belongs to agent2 != agent1)
	wUp := doReq(t, r, "PUT", "/server-hub/api/db-backups/jobs/"+job.ID.String(), map[string]interface{}{
		"name":          "db-job-updated",
		"connection_id": conn2.ID.String(),
	}, opC)
	if wUp.Code != http.StatusBadRequest {
		t.Errorf("update job with mismatched connection agent: want 400, got %d %s", wUp.Code, wUp.Body.String())
	}
}

