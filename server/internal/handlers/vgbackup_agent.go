package handlers

// VaultGuard agent HTTP protocol, served in-process by ServerHub.
//
// This file ports the agent-facing contract of the VaultGuard backup
// backend verbatim: paths (relative to the agent base URL), methods,
// request/response shapes, status codes, and error semantics all match, so
// unmodified agents work by pointing AGENT_SERVER at
// <serverhub>/server-hub/api/db-backups/agent. What changed is only the
// server side: ServerHub owns persistence (vg_* tables), auth plumbing,
// and notifications.
//
// Deliberately NOT ported: user login/org APIs (ServerHub owns identity),
// the gRPC stream and WS hub (no current agent uses them — cancel works
// through the polled cancel_requested flag, log streaming pauses), SMTP
// mailer (failure alerts go to the audit log + SSE bus instead), and the
// enrollment-token minting UI (tokens are seeded by operators; see Enroll).

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/events"
	"serverhub/internal/vgbackup"
)

// vgAgentOrgID namespaces agent artifact keys (orgs/<id>/jobs/...). The
// agent only uses it as a path segment; ServerHub has no organizations, so
// every claim carries this stable constant instead of a per-org UUID.
const vgAgentOrgID = "serverhub"

// VgAgentHandler serves the agent protocol. EncKey is the 32-byte credential
// envelope key (empty disables decrypt-dependent payloads fail-closed);
// Broker receives run/restore/agent lifecycle events for SSE consumers.
type VgAgentHandler struct {
	DB     *database.DB
	EncKey []byte
	Broker *events.Broker
}

// --- credential cryptography (ported, combined nonce‖ciphertext format) ---

func vgEncrypt(key []byte, plaintext string) (string, error) {
	if len(key) == 0 {
		return "", errors.New("encryption key not configured")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ct := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ct), nil
}

func vgDecrypt(key []byte, ciphertext string) (string, error) {
	if len(key) == 0 {
		return "", errors.New("encryption key not configured")
	}
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(data) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce, ct := data[:gcm.NonceSize()], data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// vgSha256Hex digests one-time tokens for storage. Plaintext tokens are
// shown once at creation, never stored, never logged.
func vgSha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func vgRandomHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// vgMintAgentToken creates a permanent agent credential: the plaintext
// token (returned once to the agent) and its bcrypt hash (the only
// server-side representation).
func vgMintAgentToken() (token, hash string, err error) {
	token = vgRandomHex(32)
	h, err := bcrypt.GenerateFromPassword([]byte(token), bcrypt.DefaultCost)
	if err != nil {
		return "", "", err
	}
	return token, string(h), nil
}

// --- agent authentication (separate from browser-user JWT) ---

// VgAgentAuth validates the X-Agent-ID header + Bearer token pair for
// agent-facing routes and sets "vgagent". Agent credentials are never
// treated as ServerHub user JWTs: no session, no role, no user identity.
// Failures are generic (no hash/token detail, no existence oracle beyond
// what the protocol requires).
func VgAgentAuth(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw := ""
		if h := c.GetHeader("Authorization"); len(h) > 7 && h[:7] == "Bearer " {
			raw = h[7:]
		}
		if raw == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing agent token"})
			return
		}
		agentID, err := uuid.Parse(c.GetHeader("X-Agent-ID"))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "missing or invalid X-Agent-ID header"})
			return
		}
		var agent vgbackup.VgAgent
		if err := db.GDB.Where("id = ?", agentID).First(&agent).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
			return
		}
		// Unregistered (pending) agents have no token hash yet.
		if agent.TokenHash == nil || bcrypt.CompareHashAndPassword([]byte(*agent.TokenHash), []byte(raw)) != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
			return
		}
		// Revoked agents are rejected everywhere: heartbeat, claims, status.
		if agent.RevokedAt != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "agent revoked"})
			return
		}
		c.Set("vgagent", &agent)
		c.Next()
	}
}

// VgLimitBody caps agent JSON payloads (1 MiB). Agent calls carry small
// metadata — bulk bytes travel to storage directly, never through the API —
// so anything larger is abuse or corruption. Oversized bodies fail binding
// and surface as 400, never as partial processing.
func VgLimitBody(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}

func vgAgentFromContext(c *gin.Context) *vgbackup.VgAgent {
	if v, ok := c.Get("vgagent"); ok {
		if a, ok := v.(*vgbackup.VgAgent); ok {
			return a
		}
	}
	return nil
}

// --- claim DTOs (wire shapes match the agent's expectations exactly) ---

type vgAgentRunDTO struct {
	ID          uuid.UUID `json:"id"`
	BackupJobID uuid.UUID `json:"backup_job_id"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

type vgAgentConnectionDTO struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type vgAgentStorageConfigDTO struct {
	Type         string `json:"type"`
	Path         string `json:"path,omitempty"`
	Bucket       string `json:"bucket,omitempty"`
	Region       string `json:"region,omitempty"`
	Endpoint     string `json:"endpoint,omitempty"`
	UsePathStyle bool   `json:"use_path_style,omitempty"`
	AccessKey    string `json:"access_key,omitempty"`
	SecretKey    string `json:"secret_key,omitempty"`
}

type vgAgentJobConfigDTO struct {
	JobID           string `json:"job_id"`
	OrganizationID  string `json:"organization_id"`
	Name            string `json:"name"`
	SourceType      string `json:"source_type"`
	ConnectionID    string `json:"connection_id,omitempty"`
	SourcePath      string `json:"source_path"`
	SourceDatabase  string `json:"source_database"`
	IncludePatterns string `json:"include_patterns"`
	ExcludePatterns string `json:"exclude_patterns"`
	Mode            string `json:"mode"`
	Encrypted       bool   `json:"encrypted"`
	RetentionDays   int    `json:"retention_days"`
	ExportFormat    string `json:"export_format"`
	// Connection carries decrypted DB credentials for database jobs.
	// Delivered per-claim over HTTPS; never persisted on the agent.
	Connection      *vgAgentConnectionDTO `json:"connection,omitempty"`
	StorageType     string                `json:"storage_type"`
	StorageBucket   string                `json:"storage_bucket"`
	StorageRegion   string                `json:"storage_region"`
	StorageEndpoint string                `json:"storage_endpoint"`
	StoragePath     string                `json:"storage_path"`
	// Storage is the self-contained provider config (S3 credentials
	// included, decrypted server-side per claim). Flat Storage* fields
	// stay for older agents.
	Storage vgAgentStorageConfigDTO `json:"storage"`
}

type vgAgentRestoreDTO struct {
	ID              uuid.UUID               `json:"id"`
	BackupRunID     uuid.UUID               `json:"backup_run_id"`
	DestinationPath string                  `json:"destination_path"`
	TargetDatabase  string                  `json:"target_database"`
	Status          string                  `json:"status"`
	StoragePath     string                  `json:"storage_path"`
	StorageType     string                  `json:"storage_type"`
	Storage         vgAgentStorageConfigDTO `json:"storage"`
	// DataKey is the raw base64 data key for encrypted backups.
	DataKey        string                `json:"data_key,omitempty"`
	Connection     *vgAgentConnectionDTO `json:"connection,omitempty"`
	SourceType     string                `json:"source_type"`
	SourceDatabase string                `json:"source_database"`
	ExportFormat   string                `json:"export_format,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
}

// connectionFor resolves a job's saved connection into a claim-ready config,
// decrypting the password in memory only. Returns nil when the job has no
// connection (filesystem / legacy jobs) — or when decryption fails, which
// claim handlers treat as fatal (fail loud, never connectionless).
func vgConnectionFor(job vgbackup.VgBackupJob, encKey []byte) *vgAgentConnectionDTO {
	if job.ConnectionID == nil {
		return nil
	}
	var conn vgbackup.VgDatabaseConnection
	if job.Connection != nil && job.Connection.ID == *job.ConnectionID {
		conn = *job.Connection
	} else {
		return nil
	}
	pw, err := vgDecrypt(encKey, conn.EncryptedPassword)
	if err != nil {
		return nil
	}
	return &vgAgentConnectionDTO{
		ID:       conn.ID.String(),
		Type:     string(conn.Type),
		Host:     conn.Host,
		Port:     conn.Port,
		Username: conn.Username,
		Password: pw,
	}
}

// vgStorageConfigFor resolves a target into a claim-ready config, unwrapping
// credentials only for providers that need the agent to present them (S3).
func vgStorageConfigFor(target vgbackup.VgStorageTarget, encKey []byte) vgAgentStorageConfigDTO {
	cfg := vgAgentStorageConfigDTO{
		Type:         string(target.Type),
		Path:         target.Path,
		Bucket:       target.Bucket,
		Region:       target.Region,
		Endpoint:     target.Endpoint,
		UsePathStyle: target.UsePathStyle,
	}
	if target.Type == vgbackup.VgStorageS3 {
		if ak, err := vgDecrypt(encKey, target.EncryptedAccessKey); err == nil {
			cfg.AccessKey = ak
		}
		if sk, err := vgDecrypt(encKey, target.EncryptedSecretKey); err == nil {
			cfg.SecretKey = sk
		}
	}
	return cfg
}

// --- registration & enrollment (public; token IS the auth) ---

// VgRegister is called by the agent binary with a registration key (legacy
// path; the enrollment flow below is preferred for setup UIs). Keys are
// single-use — a successful registration clears the stored digest, so a
// replayed or leaked key authenticates nothing — and minted keys expire
// (see GenerateRegistrationToken); both rejections answer 401 without
// distinction to avoid oracle behavior.
func (h *VgAgentHandler) VgRegister(c *gin.Context) {
	var req struct {
		RegistrationKey string `json:"registration_key" binding:"required"`
		Hostname        string `json:"hostname" binding:"required"`
		OS              string `json:"os"`
		Version         string `json:"version"`
		IPAddress       string `json:"ip_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var agent vgbackup.VgAgent
	if err := h.DB.GDB.Where("registration_key = ?", vgSha256Hex(req.RegistrationKey)).First(&agent).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid registration key"})
		return
	}
	if agent.RevokedAt != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid registration key"})
		return
	}
	// Minted keys carry a 24h expiry (GenerateRegistrationToken); NULL
	// means a legacy key without expiry, which stays valid until used.
	if agent.RegistrationExpiresAt != nil && !time.Now().Before(*agent.RegistrationExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid registration key"})
		return
	}

	agentToken, tokenHash, err := vgMintAgentToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}
	h.DB.GDB.Model(&agent).Updates(map[string]interface{}{
		"name":             req.Hostname,
		"token_hash":       tokenHash,
		"version":          req.Version,
		"status":           vgbackup.VgAgentOnline,
		"registration_key": nil,
	})

	machine := vgbackup.VgMachine{
		AgentID:   agent.ID,
		Hostname:  req.Hostname,
		OS:        req.OS,
		IPAddress: req.IPAddress,
	}
	h.DB.GDB.Where("agent_id = ?", agent.ID).Assign(machine).FirstOrCreate(&machine)

	audit.Write(h.DB, "vg-agent", "register", "vgagent", agent.ID.String(), "ok", req.Hostname)
	h.publishVg("vgbackup.agent", gin.H{"agent_id": agent.ID.String(), "event": "registered"})
	c.JSON(http.StatusOK, gin.H{
		"agent_id":    agent.ID,
		"agent_token": agentToken,
	})
}

// VgEnroll exchanges a one-time enrollment token for a permanent agent
// credential. Public endpoint (token IS the auth). Single-use enforced
// atomically; expired/used/unknown tokens are rejected without distinction
// (all 401) to avoid oracle behavior.
func (h *VgAgentHandler) VgEnroll(c *gin.Context) {
	var req struct {
		EnrollmentToken string `json:"enrollment_token" binding:"required"`
		MachineName     string `json:"machine_name" binding:"required"`
		Platform        string `json:"platform"`
		Architecture    string `json:"architecture"`
		AgentVersion    string `json:"agent_version"`
		Hostname        string `json:"hostname"`
		OS              string `json:"os"`
		IPAddress       string `json:"ip_address"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var tok vgbackup.VgEnrollmentToken
	if err := h.DB.GDB.Where("token_hash = ? AND revoked_at IS NULL", vgSha256Hex(strings.TrimSpace(req.EnrollmentToken))).First(&tok).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid enrollment token"})
		return
	}
	if tok.UsedAt != nil || vgEnrollmentExpired(tok.ExpiresAt, time.Now()) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid enrollment token"})
		return
	}
	if tok.AgentID == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid enrollment token"})
		return
	}

	agentToken, tokenHash, err := vgMintAgentToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token generation failed"})
		return
	}
	name := strings.TrimSpace(req.MachineName)
	if name == "" {
		name = strings.TrimSpace(req.Hostname)
	}
	now := time.Now()
	err = h.DB.GDB.Transaction(func(tx *gorm.DB) error {
		// Claim the token: exactly one concurrent enrollment can flip
		// used_at from NULL. RowsAffected==0 means we lost the race, hit a
		// replay, or the token was revoked mid-flight — abort without
		// issuing a second credential.
		claimed := tx.Model(&vgbackup.VgEnrollmentToken{}).Where("id = ? AND used_at IS NULL AND revoked_at IS NULL", tok.ID).
			Update("used_at", time.Now())
		if claimed.Error != nil {
			return claimed.Error
		}
		if claimed.RowsAffected == 0 {
			return errVgTokenConsumed
		}
		var agent vgbackup.VgAgent
		if err := tx.Where("id = ?", *tok.AgentID).First(&agent).Error; err != nil {
			return err
		}
		if agent.RevokedAt != nil {
			return errVgAgentRevoked
		}
		if err := tx.Model(&agent).Updates(map[string]interface{}{
			"name":             name,
			"machine_name":     strings.TrimSpace(req.MachineName),
			"platform":         strings.ToLower(strings.TrimSpace(req.Platform)),
			"architecture":     strings.ToLower(strings.TrimSpace(req.Architecture)),
			"version":          strings.TrimSpace(req.AgentVersion),
			"token_hash":       tokenHash,
			"status":           vgbackup.VgAgentOnline,
			"last_seen_at":     &now,
			"installed_at":     &now,
			"registration_key": nil,
		}).Error; err != nil {
			return err
		}
		machine := vgbackup.VgMachine{
			AgentID:   agent.ID,
			Hostname:  name,
			OS:        req.OS,
			IPAddress: req.IPAddress,
		}
		return tx.Where("agent_id = ?", agent.ID).Assign(machine).FirstOrCreate(&machine).Error
	})
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid enrollment token"})
		return
	}

	audit.Write(h.DB, "vg-agent", "enroll", "vgagent", tok.AgentID.String(), "ok", "")
	h.publishVg("vgbackup.agent", gin.H{"agent_id": tok.AgentID.String(), "event": "enrolled"})
	c.JSON(http.StatusOK, gin.H{
		"agent_id":                   tok.AgentID,
		"agent_token":                agentToken,
		"poll_interval_seconds":      30,
		"heartbeat_interval_seconds": 60,
	})
}

var (
	errVgAgentRevoked  = errors.New("agent revoked")
	errVgTokenConsumed = errors.New("token consumed")
)

// vgEnrollmentExpired is the pure expiry predicate (unit-tested).
func vgEnrollmentExpired(expiresAt, now time.Time) bool {
	return !now.Before(expiresAt)
}

// VgHeartbeat is the explicit liveness signal (agent-token auth). It
// updates last_seen_at and the reported agent version; ONLINE/OFFLINE is
// derived from recency, never set blindly. Empty/missing body is accepted
// (legacy agents post no payload).
func (h *VgAgentHandler) VgHeartbeat(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}
	var req struct {
		AgentVersion string `json:"agent_version"`
	}
	_ = c.ShouldBindJSON(&req) // optional payload; ignore bind errors
	now := time.Now()
	updates := map[string]interface{}{
		"status":       vgbackup.VgAgentOnline,
		"last_seen_at": &now,
	}
	if v := strings.TrimSpace(req.AgentVersion); v != "" && len(v) <= 32 {
		updates["version"] = v
	}
	h.DB.GDB.Model(agent).Updates(updates)
	c.JSON(http.StatusOK, gin.H{"status": "ok", "server_time": now})
}

// --- backup-run polling & claiming ---

// VgPendingRuns lists PENDING runs assigned to the calling agent, oldest first.
func (h *VgAgentHandler) VgPendingRuns(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}
	h.vgTouch(agent)

	var runs []vgbackup.VgBackupRun
	h.DB.GDB.Where("agent_id = ? AND status = ?", agent.ID, vgbackup.VgRunPending).
		Order("created_at ASC").Find(&runs)

	out := make([]vgAgentRunDTO, 0, len(runs))
	for _, r := range runs {
		out = append(out, vgAgentRunDTO{
			ID:          r.ID,
			BackupJobID: r.BackupJobID,
			Status:      string(r.Status),
			CreatedAt:   r.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, out)
}

// VgClaimRun atomically moves a PENDING run to RUNNING and returns it with
// the full job config the agent needs to execute. Returns 409 if the run is
// already claimed (or otherwise not PENDING), 404 if it is not ours.
func (h *VgAgentHandler) VgClaimRun(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}
	h.vgTouch(agent)

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	now := time.Now()
	res := h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Where("id = ? AND agent_id = ? AND status = ?", runID, agent.ID, vgbackup.VgRunPending).
		Updates(map[string]interface{}{"status": vgbackup.VgRunRunning, "started_at": &now})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "claim failed"})
		return
	}
	if res.RowsAffected == 0 {
		var run vgbackup.VgBackupRun
		if err := h.DB.GDB.Where("id = ? AND agent_id = ?", runID, agent.ID).First(&run).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "run already claimed"})
		return
	}

	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Preload("BackupJob.StorageTarget").Preload("BackupJob.Connection").Preload("BackupJob.Policy").
		Where("id = ?", runID).First(&run).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load run"})
		return
	}

	// Execution settings resolve policy-first so attached jobs run with
	// policy mode/encryption/retention; legacy jobs use inline settings.
	eff := vgbackup.EffectivePolicy(&run.BackupJob)

	// Fail loud on undecryptable connection credentials: handing the
	// agent a connectionless config would run the backup against the
	// agent's environment fallback (or fail opaquely there) instead of
	// surfacing the real problem — usually a rotated or lost
	// SERVERHUB_ENCRYPTION_KEY. The run is marked FAILED with an
	// actionable message so the dashboard explains itself; the agent
	// sees a 500, not a config that looks valid but isn't.
	conn := vgConnectionFor(run.BackupJob, h.EncKey)
	if run.BackupJob.ConnectionID != nil && conn == nil {
		errMsg := "server cannot decrypt the job's connection credentials; check SERVERHUB_ENCRYPTION_KEY"
		now2 := time.Now()
		updates := map[string]interface{}{
			"status": vgbackup.VgRunFailed, "completed_at": &now2,
			"error_message": errMsg, "failure_category": "CREDENTIAL_DECRYPT",
		}
		if run.StartedAt != nil {
			updates["duration_seconds"] = int64(now2.Sub(*run.StartedAt).Seconds())
		}
		h.DB.GDB.Model(&vgbackup.VgBackupRun{}).Where("id = ?", run.ID).Updates(updates)
		audit.Write(h.DB, "vg-agent:"+agent.ID.String(), "backup-failed", "vgbackuprun", run.ID.String(), "failed", errMsg)
		h.publishVg("vgbackup.run", gin.H{"run_id": run.ID.String(), "status": string(vgbackup.VgRunFailed)})
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	connID := ""
	if run.BackupJob.ConnectionID != nil {
		connID = run.BackupJob.ConnectionID.String()
	}
	c.JSON(http.StatusOK, gin.H{
		"run": vgAgentRunDTO{
			ID:          run.ID,
			BackupJobID: run.BackupJobID,
			Status:      string(run.Status),
			CreatedAt:   run.CreatedAt,
		},
		"cancel_requested": run.CancelRequested,
		"config": vgAgentJobConfigDTO{
			JobID:           run.BackupJob.ID.String(),
			OrganizationID:  vgAgentOrgID,
			Name:            run.BackupJob.Name,
			SourceType:      string(run.BackupJob.SourceType),
			ConnectionID:    connID,
			SourcePath:      run.BackupJob.SourcePath,
			SourceDatabase:  run.BackupJob.SourceDatabase,
			IncludePatterns: run.BackupJob.IncludePatterns,
			ExcludePatterns: run.BackupJob.ExcludePatterns,
			Mode:            string(eff.Mode),
			Encrypted:       eff.Encrypted,
			RetentionDays:   eff.RetentionDays,
			ExportFormat:    run.BackupJob.ExportFormat,
			Connection:      conn,
			StorageType:     string(run.BackupJob.StorageTarget.Type),
			StorageBucket:   run.BackupJob.StorageTarget.Bucket,
			StorageRegion:   run.BackupJob.StorageTarget.Region,
			StorageEndpoint: run.BackupJob.StorageTarget.Endpoint,
			StoragePath:     run.BackupJob.StorageTarget.Path,
			Storage:         vgStorageConfigFor(run.BackupJob.StorageTarget, h.EncKey),
		},
	})
}

// vgTouch records a heartbeat so agents polling for work don't flap
// OFFLINE between explicit heartbeats.
func (h *VgAgentHandler) vgTouch(agent *vgbackup.VgAgent) {
	now := time.Now()
	h.DB.GDB.Model(agent).Updates(map[string]interface{}{
		"status":       vgbackup.VgAgentOnline,
		"last_seen_at": &now,
	})
}

// VgUpdateRunStatus is called by the agent to update run progress/status.
func (h *VgAgentHandler) VgUpdateRunStatus(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Status          vgbackup.VgBackupRunStatus `json:"status"`
		BytesRead       int64                      `json:"bytes_read"`
		BytesCompressed int64                      `json:"bytes_compressed"`
		BytesUploaded   int64                      `json:"bytes_uploaded"`
		ErrorMessage    string                     `json:"error_message"`
		StoragePath     string                     `json:"storage_path"`
		Checksum        string                     `json:"checksum"`
		// DataKey is the base64 per-backup AES-256 data key, sent once by the
		// agent on completion of an encrypted backup. It is envelope-encrypted
		// with the server key before storage.
		DataKey string `json:"data_key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Counters are agent-reported telemetry: negatives are never valid and
	// would corrupt duration/size baselines and the dashboard.
	if req.BytesRead < 0 || req.BytesCompressed < 0 || req.BytesUploaded < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "byte counters must be >= 0"})
		return
	}

	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Where("id = ? AND agent_id = ?", runID, agent.ID).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	// Validate reported storage_path stays within the configured storage
	// target directory. These are agent-reported strings, never resolved
	// server-side here — but a lying or buggy agent must not rewrite history
	// to point at arbitrary paths.
	if req.StoragePath != "" {
		var target vgbackup.VgStorageTarget
		if err := h.DB.GDB.Where("id = ?", run.StorageTargetID).First(&target).Error; err == nil && target.Path != "" {
			if !vgbackup.VgIsWithinBase(target.Path, req.StoragePath) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path is outside the configured storage target directory"})
				return
			}
		}
	}

	if !vgbackup.CanTransition(run.Status, req.Status) {
		// Same-status progress reports (e.g. RUNNING → RUNNING heartbeats)
		// are idempotent, but only while the run is still active: a
		// terminal run must never accept counter writes.
		if req.Status == run.Status {
			switch run.Status {
			case vgbackup.VgRunRunning, vgbackup.VgRunUploading, vgbackup.VgRunVerifying:
				h.DB.GDB.Model(&run).Updates(map[string]interface{}{
					"bytes_read":       req.BytesRead,
					"bytes_compressed": req.BytesCompressed,
					"bytes_uploaded":   req.BytesUploaded,
				})
				c.JSON(http.StatusOK, gin.H{"updated": true})
				return
			default:
				// Terminal same-status reports (e.g. retried COMPLETED report)
				// are idempotent echos — return 200 without mutating counters.
				c.JSON(http.StatusOK, gin.H{"updated": true})
				return
			}
		}
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status transition: " + string(run.Status) + " → " + string(req.Status)})
		return
	}

	updates := map[string]interface{}{
		"status":           req.Status,
		"bytes_read":       req.BytesRead,
		"bytes_compressed": req.BytesCompressed,
		"bytes_uploaded":   req.BytesUploaded,
		"error_message":    req.ErrorMessage,
		"storage_path":     req.StoragePath,
		"checksum":         req.Checksum,
	}
	if req.Status == vgbackup.VgRunCompleted || req.Status == vgbackup.VgRunFailed {
		now := time.Now()
		updates["completed_at"] = &now
		if run.StartedAt != nil {
			updates["duration_seconds"] = int64(now.Sub(*run.StartedAt).Seconds())
		}
	}
	if req.DataKey != "" {
		wrapped, err := vgEncrypt(h.EncKey, req.DataKey)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store data key"})
			return
		}
		updates["data_key_encrypted"] = wrapped
	}

	// Atomic state transition: only update if the current DB status still
	// matches what we read above, preventing a race between two concurrent
	// status reports from the same agent.
	res := h.DB.GDB.Model(&vgbackup.VgBackupRun{}).
		Where("id = ? AND status = ?", run.ID, run.Status).
		Updates(updates)
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "update failed"})
		return
	}
	if res.RowsAffected == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "run status changed concurrently, please retry"})
		return
	}

	// Re-read: a Cancel may have landed concurrently with this report.
	var fresh vgbackup.VgBackupRun
	h.DB.GDB.Select("cancel_requested").Where("id = ?", run.ID).First(&fresh)

	// COMPLETED is only reported by the agent after its own HEAD/size/sha256
	// verification of the stored object. That makes the run's artifacts
	// verified-by-construction: record it so retention, restore eligibility,
	// and the UI can rely on persisted state instead of re-deriving it.
	if req.Status == vgbackup.VgRunCompleted {
		now := time.Now()
		h.DB.GDB.Model(&vgbackup.VgBackupArtifact{}).
			Where("backup_run_id = ? AND verification_status = ?", run.ID, vgbackup.VgArtifactUnverified).
			Updates(map[string]interface{}{"verification_status": vgbackup.VgArtifactVerified, "verified_at": &now})
	}

	// First FAILED report for a run is audit-logged. Terminal states never
	// transition out, so exactly one FAILED report exists per run. (Upstream
	// also wrote an alerts row + email; ServerHub has no backup-alerts table
	// yet — failure visibility arrives with the Phase 3 notification UI.
	// The audit trail plus SSE event below preserve the record.)
	if req.Status == vgbackup.VgRunFailed {
		audit.Write(h.DB, "vg-agent:"+agent.ID.String(), "backup-failed", "vgbackuprun", run.ID.String(), "failed", req.ErrorMessage)
	}
	if req.Status == vgbackup.VgRunCompleted || req.Status == vgbackup.VgRunFailed || req.Status == vgbackup.VgRunCancelled {
		h.publishVg("vgbackup.run", gin.H{"run_id": run.ID.String(), "status": string(req.Status)})
	}

	c.JSON(http.StatusOK, gin.H{"updated": true, "cancel_requested": fresh.CancelRequested})
}

// VgRegisterArtifact lets the agent record a backup artifact for a run.
// Idempotent on (run, name): the caller's retry after a lost response must
// not fork the artifact set. Auth via agent middleware; run must belong to
// the calling agent (404 otherwise — no cross-agent oracle).
func (h *VgAgentHandler) VgRegisterArtifact(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}

	runID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid run id"})
		return
	}

	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Where("id = ? AND agent_id = ?", runID, agent.ID).First(&run).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "run not found"})
		return
	}

	var req struct {
		Name        string `json:"name" binding:"required"`
		Size        int64  `json:"size"`
		Checksum    string `json:"checksum"`
		StoragePath string `json:"storage_path"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Size < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "size must be >= 0"})
		return
	}

	if req.StoragePath != "" {
		var target vgbackup.VgStorageTarget
		if err := h.DB.GDB.Where("id = ?", run.StorageTargetID).First(&target).Error; err == nil && target.Path != "" {
			if !vgbackup.VgIsWithinBase(target.Path, req.StoragePath) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "storage_path is outside the configured storage target directory"})
				return
			}
		}
	}

	artifact := vgbackup.VgBackupArtifact{
		BackupRunID:     runID,
		Name:            req.Name,
		Size:            req.Size,
		Checksum:        req.Checksum,
		StoragePath:     req.StoragePath,
		StorageTargetID: run.StorageTargetID,
	}
	var existing vgbackup.VgBackupArtifact
	if err := h.DB.GDB.Where("backup_run_id = ? AND name = ?", runID, req.Name).First(&existing).Error; err == nil {
		c.JSON(http.StatusOK, existing)
		return
	}
	if err := h.DB.GDB.Create(&artifact).Error; err != nil {
		// Lost race with a concurrent duplicate: return the winner instead
		// of surfacing a constraint error (unique idx on run+name).
		var winner vgbackup.VgBackupArtifact
		if herr := h.DB.GDB.Where("backup_run_id = ? AND name = ?", runID, req.Name).First(&winner).Error; herr == nil {
			c.JSON(http.StatusOK, winner)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register artifact"})
		return
	}
	c.JSON(http.StatusCreated, artifact)
}

// VgRegisterChunk records one uploaded chunk of an artifact. Same
// (artifact, index) with the same checksum is idempotent (200 existing);
// with a different checksum it is a data-integrity conflict (409). The
// artifact must belong to a run owned by the calling agent (403 on
// mismatch — the artifact exists, the caller just may not touch it).
func (h *VgAgentHandler) VgRegisterChunk(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}

	artifactID, err := uuid.Parse(c.Param("artifact_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid artifact_id"})
		return
	}

	var artifact vgbackup.VgBackupArtifact
	if err := h.DB.GDB.First(&artifact, artifactID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "artifact not found"})
		return
	}
	var run vgbackup.VgBackupRun
	if err := h.DB.GDB.Where("id = ? AND agent_id = ?", artifact.BackupRunID, agent.ID).First(&run).Error; err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}

	var req struct {
		Index       int    `json:"index"`
		Size        int64  `json:"size"`
		Checksum    string `json:"checksum"`
		StoragePath string `json:"storage_path"`
		Uploaded    bool   `json:"uploaded"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Index < 0 || req.Size < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "index and size must be >= 0"})
		return
	}

	var existing vgbackup.VgBackupChunk
	if err := h.DB.GDB.Where("artifact_id = ? AND index = ?", artifactID, req.Index).First(&existing).Error; err == nil {
		if existing.Checksum != req.Checksum {
			c.JSON(http.StatusConflict, gin.H{"error": "chunk already exists with a different checksum"})
			return
		}
		c.JSON(http.StatusOK, existing)
		return
	}

	chunk := vgbackup.VgBackupChunk{
		ArtifactID:  artifactID,
		Index:       req.Index,
		Size:        req.Size,
		Checksum:    req.Checksum,
		StoragePath: req.StoragePath,
		Uploaded:    req.Uploaded,
	}
	if err := h.DB.GDB.Create(&chunk).Error; err != nil {
		// Lost race with a concurrent duplicate: resolve like a normal
		// duplicate (checksum decides 200 vs 409) instead of 500ing.
		var winner vgbackup.VgBackupChunk
		if herr := h.DB.GDB.Where("artifact_id = ? AND index = ?", artifactID, req.Index).First(&winner).Error; herr == nil {
			if winner.Checksum != req.Checksum {
				c.JSON(http.StatusConflict, gin.H{"error": "chunk already exists with a different checksum"})
				return
			}
			c.JSON(http.StatusOK, winner)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register chunk"})
		return
	}
	c.JSON(http.StatusCreated, chunk)
}

// --- restores ---

// connectionForRestore resolves restore credentials: the restore's own
// pinned connection first, falling back to the source backup job's
// connection (covers restores created before connection pinning).
func vgConnectionForRestore(job vgbackup.VgRestoreJob, encKey []byte) *vgAgentConnectionDTO {
	if job.ConnectionID != nil && job.Connection != nil && job.Connection.ID == *job.ConnectionID {
		if pw, err := vgDecrypt(encKey, job.Connection.EncryptedPassword); err == nil && pw != "" {
			return &vgAgentConnectionDTO{
				ID:       job.Connection.ID.String(),
				Type:     string(job.Connection.Type),
				Host:     job.Connection.Host,
				Port:     job.Connection.Port,
				Username: job.Connection.Username,
				Password: pw,
			}
		}
	}
	// Fallback: source job's connection via the backup run.
	if job.BackupRun.BackupJob.ConnectionID != nil && job.BackupRun.BackupJob.Connection != nil &&
		job.BackupRun.BackupJob.Connection.ID == *job.BackupRun.BackupJob.ConnectionID {
		return vgConnectionFor(job.BackupRun.BackupJob, encKey)
	}
	return nil
}

func (h *VgAgentHandler) vgRestoreDTO(job vgbackup.VgRestoreJob) vgAgentRestoreDTO {
	dto := vgAgentRestoreDTO{
		ID:              job.ID,
		BackupRunID:     job.BackupRunID,
		DestinationPath: job.DestinationPath,
		TargetDatabase:  job.TargetDatabase,
		Status:          string(job.Status),
		StoragePath:     job.BackupRun.StoragePath,
		StorageType:     string(job.BackupRun.BackupJob.StorageTarget.Type),
		Storage:         vgStorageConfigFor(job.BackupRun.BackupJob.StorageTarget, h.EncKey),
		Connection:      vgConnectionForRestore(job, h.EncKey),
		SourceType:      string(job.BackupRun.BackupJob.SourceType),
		SourceDatabase:  job.BackupRun.BackupJob.SourceDatabase,
		ExportFormat:    job.BackupRun.BackupJob.ExportFormat,
		CreatedAt:       job.CreatedAt,
	}
	if job.BackupRun.DataKeyEncrypted != "" {
		if raw, err := vgDecrypt(h.EncKey, job.BackupRun.DataKeyEncrypted); err == nil {
			dto.DataKey = raw
		}
	}
	return dto
}

// VgPendingRestores lists PENDING restores assigned to the calling agent.
func (h *VgAgentHandler) VgPendingRestores(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}
	h.vgTouch(agent)

	var jobs []vgbackup.VgRestoreJob
	h.DB.GDB.Preload("Connection").Preload("BackupRun.BackupJob.StorageTarget").Preload("BackupRun.BackupJob.Connection").
		Where("agent_id = ? AND status = ?", agent.ID, vgbackup.VgRestorePending).
		Order("created_at ASC").Find(&jobs)

	out := make([]vgAgentRestoreDTO, 0, len(jobs))
	for _, j := range jobs {
		out = append(out, h.vgRestoreDTO(j))
	}
	c.JSON(http.StatusOK, out)
}

// VgClaimRestore atomically moves a PENDING restore to RUNNING. Returns 409
// if already claimed, 404 if it is not ours.
func (h *VgAgentHandler) VgClaimRestore(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}
	h.vgTouch(agent)

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	now := time.Now()
	res := h.DB.GDB.Model(&vgbackup.VgRestoreJob{}).
		Where("id = ? AND agent_id = ? AND status = ?", id, agent.ID, vgbackup.VgRestorePending).
		Updates(map[string]interface{}{"status": vgbackup.VgRestoreRunning, "started_at": &now})
	if res.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "claim failed"})
		return
	}
	if res.RowsAffected == 0 {
		var job vgbackup.VgRestoreJob
		if err := h.DB.GDB.Where("id = ? AND agent_id = ?", id, agent.ID).First(&job).Error; err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "restore not found"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{"error": "restore already claimed"})
		return
	}

	var job vgbackup.VgRestoreJob
	if err := h.DB.GDB.Preload("Connection").Preload("BackupRun.BackupJob.StorageTarget").Preload("BackupRun.BackupJob.Connection").
		Where("id = ?", id).First(&job).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load restore"})
		return
	}

	// Same fail-loud rule as backup claims: a restore pinned to a
	// connection (or whose source job has one) must receive usable
	// credentials, and an encrypted backup's data key must unwrap —
	// otherwise the agent would fail opaquely mid-restore, potentially
	// after partially overwriting the destination.
	if (job.ConnectionID != nil || job.BackupRun.BackupJob.ConnectionID != nil) &&
		vgConnectionForRestore(job, h.EncKey) == nil {
		h.failRestoreClaim(c, agent, &job, "server cannot decrypt restore credentials; check SERVERHUB_ENCRYPTION_KEY")
		return
	}
	if job.BackupRun.DataKeyEncrypted != "" {
		if _, err := vgDecrypt(h.EncKey, job.BackupRun.DataKeyEncrypted); err != nil {
			h.failRestoreClaim(c, agent, &job, "server cannot unwrap the backup data key; check SERVERHUB_ENCRYPTION_KEY")
			return
		}
	}
	c.JSON(http.StatusOK, h.vgRestoreDTO(job))
}

// failRestoreClaim marks a just-claimed restore FAILED with an actionable
// message instead of handing the agent a config that cannot succeed.
func (h *VgAgentHandler) failRestoreClaim(c *gin.Context, agent *vgbackup.VgAgent, job *vgbackup.VgRestoreJob, errMsg string) {
	now := time.Now()
	updates := map[string]interface{}{
		"status": vgbackup.VgRestoreFailed, "completed_at": &now, "error_message": errMsg,
	}
	if job.StartedAt != nil {
		updates["duration_seconds"] = int64(now.Sub(*job.StartedAt).Seconds())
	}
	h.DB.GDB.Model(&vgbackup.VgRestoreJob{}).Where("id = ?", job.ID).Updates(updates)
	audit.Write(h.DB, "vg-agent:"+agent.ID.String(), "restore-failed", "vgrestore", job.ID.String(), "failed", errMsg)
	h.publishVg("vgbackup.restore", gin.H{"restore_id": job.ID.String(), "status": string(vgbackup.VgRestoreFailed)})
	c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
}

// VgUpdateRestoreStatus records agent-reported restore progress. Terminal
// reports are idempotent: a retried COMPLETED/FAILED report (lost response)
// echoes the recorded outcome instead of erroring.
func (h *VgAgentHandler) VgUpdateRestoreStatus(c *gin.Context) {
	agent := vgAgentFromContext(c)
	if agent == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
		return
	}

	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req struct {
		Status       vgbackup.VgRestoreStatus `json:"status"`
		ErrorMessage string                   `json:"error_message"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var job vgbackup.VgRestoreJob
	if err := h.DB.GDB.Where("id = ? AND agent_id = ?", id, agent.ID).First(&job).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}

	if req.Status == job.Status && (job.Status == vgbackup.VgRestoreCompleted || job.Status == vgbackup.VgRestoreFailed) {
		c.JSON(http.StatusOK, gin.H{"updated": true})
		return
	}
	if !vgbackup.CanRestoreTransition(job.Status, req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid restore transition: " + string(job.Status) + " → " + string(req.Status)})
		return
	}

	updates := map[string]interface{}{"status": req.Status}
	if req.ErrorMessage != "" {
		updates["error_message"] = req.ErrorMessage
	}
	if req.Status == vgbackup.VgRestoreRunning && job.StartedAt == nil {
		now := time.Now()
		updates["started_at"] = &now
	}
	if req.Status == vgbackup.VgRestoreCompleted || req.Status == vgbackup.VgRestoreFailed {
		now := time.Now()
		updates["completed_at"] = &now
		if job.StartedAt != nil {
			updates["duration_seconds"] = int64(now.Sub(*job.StartedAt).Seconds())
		}
	}

	h.DB.GDB.Model(&vgbackup.VgRestoreJob{}).
		Where("id = ? AND agent_id = ? AND status = ?", id, agent.ID, job.Status).
		Updates(updates)

	if req.Status == vgbackup.VgRestoreCompleted || req.Status == vgbackup.VgRestoreFailed {
		h.publishVg("vgbackup.restore", gin.H{"restore_id": id.String(), "status": string(req.Status)})
	}
	c.JSON(http.StatusOK, gin.H{"updated": true})
}

// publishVg emits lifecycle events for SSE consumers. Broker fan-out is
// non-blocking; a missing broker (tests) simply skips.
func (h *VgAgentHandler) publishVg(eventType string, data interface{}) {
	if h.Broker == nil {
		return
	}
	h.Broker.Publish(eventType, data)
}

// classifyFailure maps a free-text error message to a structured category
// so future dashboards can display actionable error types instead of raw
// strings.
func classifyFailure(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "authentication") || strings.Contains(m, "password") ||
		strings.Contains(m, "permission denied") || strings.Contains(m, "access denied") ||
		strings.Contains(m, "invalid credentials"):
		return "AUTH_ERROR"
	case strings.Contains(m, "no space left") || strings.Contains(m, "disk full") ||
		strings.Contains(m, "not enough space"):
		return "DISK_FULL"
	case strings.Contains(m, "connection refused") || strings.Contains(m, "no such host") ||
		strings.Contains(m, "network") || strings.Contains(m, "dial tcp") ||
		strings.Contains(m, "i/o timeout"):
		return "NETWORK_ERROR"
	case strings.Contains(m, "s3") || strings.Contains(m, "bucket") ||
		strings.Contains(m, "storage") || strings.Contains(m, "upload"):
		return "STORAGE_ERROR"
	case strings.Contains(m, "timeout") || strings.Contains(m, "deadline exceeded") ||
		strings.Contains(m, "context deadline"):
		return "TIMEOUT"
	case strings.Contains(m, "encrypt") || strings.Contains(m, "decrypt") ||
		strings.Contains(m, "data key"):
		return "ENCRYPTION_ERROR"
	case strings.Contains(m, "cancelled") || strings.Contains(m, "canceled"):
		return "CANCELLED"
	case strings.Contains(m, "source") || strings.Contains(m, "pg_dump") ||
		strings.Contains(m, "mongodump") || strings.Contains(m, "backup database"):
		return "SOURCE_ERROR"
	case strings.Contains(m, "verification") || strings.Contains(m, "checksum") ||
		strings.Contains(m, "hash mismatch"):
		return "VERIFICATION_ERROR"
	default:
		return "UNKNOWN"
	}
}
