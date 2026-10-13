package handlers

// Security regression tests from the Phase 6 audit: registration keys are
// single-use and expirable, and agent claims fail loud (never degraded)
// when server-side credential decryption fails.

import (
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"serverhub/internal/vgbackup"
)

func TestVgRegisterSingleUseAndExpiry(t *testing.T) {
	db, r := vgAgentRouter(t)

	register := func(key string) int {
		w := doVg(t, r, "POST", "/server-hub/api/db-backups/agent/agents/register", map[string]any{
			"registration_key": key, "hostname": "h1",
		}, "", "")
		return w.Code
	}

	// Fresh key without expiry registers once...
	key1 := vgRandomHex(32)
	seedVgPendingAgent(t, db, key1)
	if code := register(key1); code != http.StatusOK {
		t.Fatalf("first register: want 200, got %d", code)
	}
	// ...and the same key is dead afterwards (cleared on success).
	if code := register(key1); code != http.StatusUnauthorized {
		t.Errorf("key replay: want 401, got %d", code)
	}

	// Expired minted keys are rejected.
	key2 := vgRandomHex(32)
	past := time.Now().Add(-time.Hour)
	h2 := vgSha256Hex(key2)
	expired := vgbackup.VgAgent{Name: "pending-exp", CreatedByUserID: 1, RegistrationKey: &h2, RegistrationExpiresAt: &past, Status: vgbackup.VgAgentOffline}
	if err := db.GDB.Create(&expired).Error; err != nil {
		t.Fatal(err)
	}
	if code := register(key2); code != http.StatusUnauthorized {
		t.Errorf("expired key: want 401, got %d", code)
	}

	// Unexpired minted keys still work.
	key3 := vgRandomHex(32)
	future := time.Now().Add(time.Hour)
	h3 := vgSha256Hex(key3)
	fresh := vgbackup.VgAgent{Name: "pending-fresh", CreatedByUserID: 1, RegistrationKey: &h3, RegistrationExpiresAt: &future, Status: vgbackup.VgAgentOffline}
	if err := db.GDB.Create(&fresh).Error; err != nil {
		t.Fatal(err)
	}
	if code := register(key3); code != http.StatusOK {
		t.Errorf("unexpired key: want 200, got %d", code)
	}
}

func TestVgClaimRunDecryptFailure(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "claim-agent")
	tgt := seedVgTarget(t, db)

	conn := vgbackup.VgDatabaseConnection{
		Name: "db", Type: vgbackup.VgConnectionPostgres, Host: "h", Port: 5432,
		Username: "u", EncryptedPassword: "!!!not-valid-ciphertext!!!",
		AgentID: agent.ID, CreatedByUserID: 1,
	}
	if err := db.GDB.Create(&conn).Error; err != nil {
		t.Fatal(err)
	}
	job := vgbackup.VgBackupJob{
		Name: "db-job", AgentID: agent.ID, StorageTargetID: tgt.ID, ConnectionID: &conn.ID,
		SourceType: vgbackup.VgSourcePostgres, SourceDatabase: "app",
		Mode: vgbackup.VgModeNormal, Enabled: true, CreatedByUserID: 1,
	}
	if err := db.GDB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	mkRun := func() uuid.UUID {
		run := vgbackup.VgBackupRun{BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: tgt.ID, Status: vgbackup.VgRunPending}
		if err := db.GDB.Create(&run).Error; err != nil {
			t.Fatal(err)
		}
		return run.ID
	}

	// Undecryptable credentials: claim fails loud and the run is FAILED
	// with an actionable message — never handed a connectionless config.
	badID := mkRun()
	w := doVg(t, r, "POST", "/server-hub/api/db-backups/agent/agent/runs/"+badID.String()+"/claim", nil, agent.ID.String(), token)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("bad-ciphertext claim: want 500, got %d %s", w.Code, w.Body.String())
	}
	var bad vgbackup.VgBackupRun
	if err := db.GDB.Where("id = ?", badID).First(&bad).Error; err != nil {
		t.Fatal(err)
	}
	if bad.Status != vgbackup.VgRunFailed || bad.FailureCategory != "CREDENTIAL_DECRYPT" {
		t.Errorf("run must be FAILED/CREDENTIAL_DECRYPT, got %s/%s", bad.Status, bad.FailureCategory)
	}
	if bad.ErrorMessage == "" {
		t.Error("failed run must carry an actionable error message")
	}

	// Healthy credentials still claim fine with the password injected.
	good, err := vgEncrypt(vgTestEncKey, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	db.GDB.Model(&vgbackup.VgDatabaseConnection{}).Where("id = ?", conn.ID).Update("encrypted_password", good)
	goodID := mkRun()
	w = doVg(t, r, "POST", "/server-hub/api/db-backups/agent/agent/runs/"+goodID.String()+"/claim", nil, agent.ID.String(), token)
	if w.Code != http.StatusOK {
		t.Fatalf("good claim: want 200, got %d %s", w.Code, w.Body.String())
	}
	var dto struct {
		Config struct {
			Connection *struct {
				Password string `json:"password"`
			} `json:"connection"`
		} `json:"config"`
	}
	decodeVg(t, w, &dto)
	if dto.Config.Connection == nil || dto.Config.Connection.Password != "s3cret" {
		t.Errorf("claimed config must carry the decrypted password: %+v", dto.Config.Connection)
	}
}

func TestVgClaimRestoreDecryptFailure(t *testing.T) {
	db, r := vgAgentRouter(t)
	token, agent := seedVgAgent(t, db, "restore-agent")
	tgt := seedVgTarget(t, db)

	conn := vgbackup.VgDatabaseConnection{
		Name: "db", Type: vgbackup.VgConnectionPostgres, Host: "h", Port: 5432,
		Username: "u", EncryptedPassword: "!!!not-valid-ciphertext!!!",
		AgentID: agent.ID, CreatedByUserID: 1,
	}
	if err := db.GDB.Create(&conn).Error; err != nil {
		t.Fatal(err)
	}
	job := vgbackup.VgBackupJob{
		Name: "db-job", AgentID: agent.ID, StorageTargetID: tgt.ID, ConnectionID: &conn.ID,
		SourceType: vgbackup.VgSourcePostgres, SourceDatabase: "app",
		Mode: vgbackup.VgModeNormal, Enabled: true, CreatedByUserID: 1,
	}
	if err := db.GDB.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	done := time.Now()
	run := vgbackup.VgBackupRun{
		BackupJobID: job.ID, AgentID: agent.ID, StorageTargetID: tgt.ID,
		Status: vgbackup.VgRunCompleted, CompletedAt: &done,
	}
	if err := db.GDB.Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	mkRestore := func() uuid.UUID {
		rj := vgbackup.VgRestoreJob{
			BackupRunID: run.ID, AgentID: agent.ID, ConnectionID: &conn.ID,
			Status: vgbackup.VgRestorePending, TargetDatabase: "app_copy",
		}
		if err := db.GDB.Create(&rj).Error; err != nil {
			t.Fatal(err)
		}
		return rj.ID
	}

	rid := mkRestore()
	w := doVg(t, r, "POST", "/server-hub/api/db-backups/agent/agent/restores/"+rid.String()+"/claim", nil, agent.ID.String(), token)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("bad-credential restore claim: want 500, got %d %s", w.Code, w.Body.String())
	}
	var got vgbackup.VgRestoreJob
	if err := db.GDB.Where("id = ?", rid).First(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != vgbackup.VgRestoreFailed || got.ErrorMessage == "" {
		t.Errorf("restore must be FAILED with a message, got %s/%q", got.Status, got.ErrorMessage)
	}

	// Unwrappable data key fails the same loud way.
	good, err := vgEncrypt(vgTestEncKey, "s3cret")
	if err != nil {
		t.Fatal(err)
	}
	db.GDB.Model(&vgbackup.VgDatabaseConnection{}).Where("id = ?", conn.ID).Update("encrypted_password", good)
	db.GDB.Model(&vgbackup.VgBackupRun{}).Where("id = ?", run.ID).Update("data_key_encrypted", "!!!bad!!!")
	rid2 := mkRestore()
	w = doVg(t, r, "POST", "/server-hub/api/db-backups/agent/agent/restores/"+rid2.String()+"/claim", nil, agent.ID.String(), token)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("bad-datakey restore claim: want 500, got %d %s", w.Code, w.Body.String())
	}
	// Fresh struct: reusing `got` would AND its old primary key into the query.
	var got2 vgbackup.VgRestoreJob
	if err := db.GDB.Where("id = ?", rid2).First(&got2).Error; err != nil {
		t.Fatal(err)
	}
	if got2.Status != vgbackup.VgRestoreFailed {
		t.Errorf("restore with bad data key must be FAILED, got %s", got2.Status)
	}
}
