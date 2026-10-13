// Package vgbackup is ServerHub's integrated database-backup domain,
// adapted from the VaultGuard backup backend (monolithic integration: no
// second API service, no second database).
//
// Scope (Phase 1: persistence + identity foundation only): backup-domain
// tables and pure policy helpers live here. Agent HTTP API, enrollment
// flows, scheduler, retention worker, storage/connection handlers,
// verification, restore execution, and UI arrive in later phases.
//
// Identity model (see authz.go): ServerHub users are authoritative.
// VaultGuard's users/organizations/members/refresh_tokens/alerts/audit_logs
// tables are deliberately NOT ported — ServerHub already owns users,
// roles, alerts, and the immutable audit log. Every ported table uses a
// vg_ prefix so a name can never collide with a ServerHub table, and
// per-user ownership is a CreatedByUserID column holding the immutable
// ServerHub users.id (never a username string, so renames are harmless).
//
// Tenancy: VaultGuard scoped every row by organization_id. ServerHub has
// no organizations — the deployment itself is the tenant boundary — so the
// column is dropped, not faked. If ServerHub ever grows organizations,
// that is a data-migration project, not a column rename.
package vgbackup

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// --- Base ---

type VgBase struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (b *VgBase) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

// --- Agent ---

type VgAgentStatus string

const (
	VgAgentOnline  VgAgentStatus = "ONLINE"
	VgAgentOffline VgAgentStatus = "OFFLINE"
)

// Agent lifecycle states exposed by the API (transient, never stored):
// PENDING = created but never enrolled; REVOKED = administratively
// revoked; otherwise the ONLINE/OFFLINE heartbeat state.
const (
	VgLifecyclePending = "PENDING"
	VgLifecycleOnline  = "ONLINE"
	VgLifecycleOffline = "OFFLINE"
	VgLifecycleRevoked = "REVOKED"
)

// VgOnlineThreshold is how recently last_seen_at must be for ONLINE.
const VgOnlineThreshold = 3 * time.Minute

type VgAgent struct {
	VgBase
	CreatedByUserID uint   `gorm:"not null;default:0;index" json:"created_by"`
	Name            string `gorm:"not null" json:"name"`
	// TokenHash is nil until the agent completes registration. NULLs never
	// collide in the unique index, so any number of pending (key-only) rows
	// can coexist.
	TokenHash      *string       `gorm:"uniqueIndex" json:"-"`
	TokenRotatedAt *time.Time    `json:"token_rotated_at"`
	Status         VgAgentStatus `gorm:"not null;default:'OFFLINE'" json:"status"`
	Version        string        `json:"version"`
	LastSeenAt     *time.Time    `json:"last_seen_at"`
	// RegistrationKey stores a SHA-256 hex digest of the one-time key, never
	// plaintext. Pending (never-enrolled) rows carry it; enrollment clears it.
	RegistrationKey *string `gorm:"uniqueIndex" json:"-"`
	// Lifecycle metadata (agent milestone): inventory + revocation.
	Platform              string     `gorm:"default:''" json:"platform"`
	Architecture          string     `gorm:"default:''" json:"architecture"`
	MachineName           string     `gorm:"default:''" json:"machine_name"`
	InstalledAt           *time.Time `json:"installed_at,omitempty"`
	RevokedAt             *time.Time `json:"revoked_at,omitempty"`
	RegistrationExpiresAt *time.Time `json:"-"`
}

func (VgAgent) TableName() string { return "vg_agents" }

// Lifecycle returns the effective enrollment-aware state. ONLINE is derived
// from heartbeat recency, never trusted blindly from the stored flag.
func (a *VgAgent) Lifecycle() string {
	if a.RevokedAt != nil {
		return VgLifecycleRevoked
	}
	if a.TokenHash == nil {
		return VgLifecyclePending
	}
	if a.LastSeenAt != nil && time.Since(*a.LastSeenAt) < VgOnlineThreshold {
		return VgLifecycleOnline
	}
	return VgLifecycleOffline
}

// --- EnrollmentToken ---
// EnrollmentToken is a short-lived single-use token for agent enrollment.
// Only the SHA-256 hex digest is stored; plaintext is shown once at
// creation. Optionally linked to a pre-created pending Agent row.
type VgEnrollmentToken struct {
	VgBase
	AgentID         *uuid.UUID `gorm:"type:uuid;index" json:"agent_id,omitempty"`
	TokenHash       string     `gorm:"uniqueIndex;not null" json:"-"`
	ExpiresAt       time.Time  `gorm:"not null;index" json:"expires_at"`
	UsedAt          *time.Time `json:"used_at,omitempty"`
	RevokedAt       *time.Time `json:"revoked_at,omitempty"`
	CreatedByUserID uint       `json:"-"`
	Agent           *VgAgent   `gorm:"foreignKey:AgentID;constraint:OnDelete:CASCADE" json:"agent,omitempty"`
}

func (VgEnrollmentToken) TableName() string { return "vg_enrollment_tokens" }

// --- Machine ---

type VgMachine struct {
	VgBase
	AgentID   uuid.UUID `gorm:"type:uuid;not null;index" json:"agent_id"`
	Hostname  string    `gorm:"not null" json:"hostname"`
	OS        string    `json:"os"`
	IPAddress string    `json:"ip_address"`
	Agent     VgAgent   `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
}

func (VgMachine) TableName() string { return "vg_machines" }

// --- StorageTarget ---

type VgStorageType string

const (
	VgStorageS3    VgStorageType = "S3"
	VgStorageLocal VgStorageType = "LOCAL"
	VgStorageSMB   VgStorageType = "SMB"
)

type VgStorageTarget struct {
	VgBase
	CreatedByUserID uint          `gorm:"not null;default:0;index" json:"created_by"`
	Name            string        `gorm:"not null" json:"name"`
	Type            VgStorageType `gorm:"not null" json:"type"`
	Bucket          string        `json:"bucket"`
	Region          string        `json:"region"`
	Endpoint        string        `json:"endpoint"`
	// UsePathStyle selects path-style addressing (http://host/bucket/key,
	// MinIO-friendly) over virtual-hosted style (AWS/R2/Wasabi).
	UsePathStyle       bool   `gorm:"default:false" json:"use_path_style"`
	EncryptedAccessKey string `json:"-"`
	EncryptedSecretKey string `json:"-"`
	Path               string `json:"path"`
	// HasCredentials is transient (never stored): tells the UI whether
	// credentials exist without ever returning them.
	HasCredentials bool `gorm:"-" json:"has_credentials"`
}

func (VgStorageTarget) TableName() string { return "vg_storage_targets" }

// --- S3Region ---

// VgS3Region is reference data for the storage-target region picker: AWS
// region codes plus S3-compatible presets. Seeding happens with the Phase 2
// storage UI (this phase creates the table only); admins can add private
// endpoints via the API without a redeploy. StorageTarget.Region stores the
// code as free text, so unknown future regions keep working regardless.
type VgS3Region struct {
	VgBase
	Code     string `gorm:"uniqueIndex;not null" json:"code"`
	Name     string `gorm:"not null" json:"name"`
	Provider string `gorm:"not null;default:'AWS'" json:"provider"`
	Endpoint string `json:"endpoint,omitempty"`
	IsSystem bool   `gorm:"default:true" json:"is_system"`
	Active   bool   `gorm:"default:true" json:"active"`
}

func (VgS3Region) TableName() string { return "vg_s3_regions" }

// --- DatabaseConnection ---
// DatabaseConnection is a reusable infrastructure resource: connection
// metadata configured once, referenced by many backup jobs. Secrets are
// NEVER stored here in plaintext — only AES-256-GCM ciphertext (same
// SERVERHUB_ENCRYPTION_KEY envelope as other ServerHub secrets), never
// returned by the API. The agent receives decrypted credentials per-claim
// over HTTPS; filesystem jobs leave ConnectionID NULL.
type VgConnectionType string

const (
	VgConnectionPostgres VgConnectionType = "POSTGRES"
	VgConnectionMongo    VgConnectionType = "MONGODB"
	VgConnectionMssql    VgConnectionType = "MSSQL"
)

type VgConnectionStatus string

const (
	VgConnectionUnknown      VgConnectionStatus = "UNKNOWN"
	VgConnectionConnected    VgConnectionStatus = "CONNECTED"
	VgConnectionDisconnected VgConnectionStatus = "DISCONNECTED"
	VgConnectionError        VgConnectionStatus = "ERROR"
)

type VgDatabaseConnection struct {
	VgBase
	CreatedByUserID uint             `gorm:"not null;default:0;index;uniqueIndex:idx_vg_conn_owner_name" json:"created_by"`
	AgentID         uuid.UUID        `gorm:"type:uuid;not null;index" json:"agent_id"`
	Name            string           `gorm:"not null;uniqueIndex:idx_vg_conn_owner_name" json:"name"`
	Type            VgConnectionType `gorm:"not null" json:"type"`
	Host            string           `gorm:"not null" json:"host"`
	Port            int              `gorm:"not null" json:"port"`
	Username        string           `gorm:"not null" json:"username"`
	// EncryptedPassword holds AES-256-GCM ciphertext (base64). json:"-" so
	// it never leaves the server except per-claim to the owning agent.
	EncryptedPassword string             `json:"-"`
	Status            VgConnectionStatus `gorm:"not null;default:'UNKNOWN'" json:"status"`
	LastCheckedAt     *time.Time         `json:"last_checked_at,omitempty"`
	// DatabaseNames caches the last discovered database list (metadata,
	// not secret) so the job wizard can populate its dropdown without a
	// live agent round-trip. Refreshed on test/save.
	DatabaseNames string `gorm:"default:''" json:"-"`
	// DatabaseMetadata caches richer per-database details (JSON array of
	// DatabaseInfo: sizes, table counts) collected at Test/Refresh time.
	// Best-effort: empty when the engine doesn't expose details.
	DatabaseMetadata string  `gorm:"default:''" json:"-"`
	Agent            VgAgent `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
}

func (VgDatabaseConnection) TableName() string { return "vg_database_connections" }

// UnknownSize marks an unavailable size/count in DatabaseInfo (-1, since 0
// is a legitimate empty-database reading).
const UnknownSize = int64(-1)

// DatabaseInfo is one cached database entry: identity plus best-effort
// details. TableCount -1 (and SizeBytes -1) mean "unknown for this engine".
type DatabaseInfo struct {
	Name       string `json:"name"`
	SizeBytes  int64  `json:"size_bytes"`
	TableCount int    `json:"table_count"`
}

// TableInfo is one table/collection inside a database (live drill-in, not
// cached). Rows -1 / SizeBytes -1 mean unknown.
type TableInfo struct {
	Schema    string `json:"schema,omitempty"`
	Name      string `json:"name"`
	Rows      int64  `json:"rows"`
	SizeBytes int64  `json:"size_bytes"`
}

// DatabaseList returns the cached database names as a slice.
func (c *VgDatabaseConnection) DatabaseList() []string {
	if c.DatabaseNames == "" {
		return []string{}
	}
	var out []string
	for _, n := range splitConnDBs(c.DatabaseNames) {
		if n != "" {
			out = append(out, n)
		}
	}
	return out
}

func splitConnDBs(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

// MetadataList returns cached DatabaseInfo, falling back to bare names with
// unknown details when no metadata was ever collected.
func (c *VgDatabaseConnection) MetadataList() []DatabaseInfo {
	if strings.TrimSpace(c.DatabaseMetadata) != "" {
		var out []DatabaseInfo
		if err := json.Unmarshal([]byte(c.DatabaseMetadata), &out); err == nil && len(out) > 0 {
			return out
		}
	}
	names := c.DatabaseList()
	out := make([]DatabaseInfo, 0, len(names))
	for _, n := range names {
		out = append(out, DatabaseInfo{Name: n, SizeBytes: UnknownSize, TableCount: -1})
	}
	return out
}

// MarshalDatabaseMetadata encodes details for storage ("" when empty).
func MarshalDatabaseMetadata(infos []DatabaseInfo) string {
	if len(infos) == 0 {
		return ""
	}
	data, err := json.Marshal(infos)
	if err != nil {
		return ""
	}
	return string(data)
}

// MergeMetadataNames re-attaches cached sizes/counts to a fresh names-only
// discovery (frontend loopback sends names; details survive by name match).
func MergeMetadataNames(cached []DatabaseInfo, names []string) []DatabaseInfo {
	byName := map[string]DatabaseInfo{}
	for _, d := range cached {
		byName[d.Name] = d
	}
	out := make([]DatabaseInfo, 0, len(names))
	for _, n := range names {
		if d, ok := byName[n]; ok {
			out = append(out, d)
		} else {
			out = append(out, DatabaseInfo{Name: n, SizeBytes: UnknownSize, TableCount: -1})
		}
	}
	return out
}

// --- BackupJob ---

type VgBackupSourceType string

const (
	VgSourceFilesystem VgBackupSourceType = "FILESYSTEM"
	VgSourceSQLServer  VgBackupSourceType = "SQL_SERVER"
	// VgSourceMssqlServer is what the dashboard sends for SQL Server jobs;
	// both spellings are accepted wherever source types are matched.
	VgSourceMssqlServer VgBackupSourceType = "MSSQL_SERVER"
	VgSourceDBF         VgBackupSourceType = "DBF"
	VgSourcePostgres    VgBackupSourceType = "POSTGRES"
	VgSourceMongo       VgBackupSourceType = "MONGODB"
)

type VgBackupMode string

const (
	VgModeNormal     VgBackupMode = "NORMAL"
	VgModeCompressed VgBackupMode = "COMPRESSED"
)

// Backup strategy. FULL is the only strategy the backup engines implement;
// the column exists so INCREMENTAL/DIFFERENTIAL can land later WITHOUT
// renaming full backups into something they are not (validation rejects
// anything but FULL until the engines implement the semantics).
const VgStrategyFull = "FULL"

type VgBackupJob struct {
	VgBase
	CreatedByUserID uint      `gorm:"not null;default:0;index" json:"created_by"`
	AgentID         uuid.UUID `gorm:"type:uuid;not null;index" json:"agent_id"`
	StorageTargetID uuid.UUID `gorm:"type:uuid;not null;index" json:"storage_target_id"`
	// PolicyID optionally attaches a VgBackupPolicy. NULL means "job-inline
	// settings" — every pre-policy job keeps working with zero migration.
	// Effective settings resolve policy-first, job-inline fallback (see
	// EffectivePolicy).
	PolicyID   *uuid.UUID         `gorm:"type:uuid;index" json:"policy_id,omitempty"`
	Name       string             `gorm:"not null" json:"name"`
	SourceType VgBackupSourceType `gorm:"not null" json:"source_type"`
	// ConnectionID references a saved VgDatabaseConnection for database jobs.
	// NULL for filesystem jobs and for legacy jobs created before the
	// connections feature (backward compatible).
	ConnectionID    *uuid.UUID `gorm:"type:uuid;index" json:"connection_id,omitempty"`
	SourcePath      string     `json:"source_path"`
	SourceDatabase  string     `json:"source_database"`
	IncludePatterns string     `json:"include_patterns"`
	ExcludePatterns string     `json:"exclude_patterns"`
	// ExportFormat selects the MongoDB payload shape: ARCHIVE (default,
	// mongodump-compatible binary), JSON (one EJSON doc per line), or CSV
	// (flattened, header row; values restore as strings). Ignored by other
	// source types.
	ExportFormat  string       `gorm:"default:'ARCHIVE'" json:"export_format"`
	Mode          VgBackupMode `gorm:"not null;default:'NORMAL'" json:"mode"`
	Encrypted     bool         `gorm:"default:false" json:"encrypted"`
	RetentionDays int          `gorm:"default:30" json:"retention_days"`
	Enabled       bool         `gorm:"default:true" json:"enabled"`
	// SLA / RPO / RTO targets (0 = not configured).
	SLATargetMinutes int                   `gorm:"default:0" json:"sla_target_minutes"`
	RPOTargetMinutes int                   `gorm:"default:0" json:"rpo_target_minutes"`
	RTOTargetMinutes int                   `gorm:"default:0" json:"rto_target_minutes"`
	Schedule         *VgBackupSchedule     `gorm:"foreignKey:BackupJobID" json:"schedule,omitempty"`
	Policy           *VgBackupPolicy       `gorm:"foreignKey:PolicyID" json:"policy,omitempty"`
	Agent            VgAgent               `gorm:"foreignKey:AgentID" json:"agent,omitempty"`
	StorageTarget    VgStorageTarget       `gorm:"foreignKey:StorageTargetID" json:"storage_target,omitempty"`
	Connection       *VgDatabaseConnection `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
}

func (VgBackupJob) TableName() string { return "vg_backup_jobs" }

// --- BackupPolicy ---
//
// BackupPolicy defines HOW a backup operates (schedule, processing,
// retention, verification, retry); BackupJob defines WHAT/WHERE (source,
// agent, storage). Jobs with PolicyID == NULL use their own inline settings,
// so every existing job keeps working with no data migration.

type VgBackupPolicy struct {
	VgBase
	CreatedByUserID uint   `gorm:"not null;default:0;index" json:"created_by"`
	Name            string `gorm:"not null" json:"name"`
	// Strategy is FULL-only until the engines implement real incrementals.
	Strategy string `gorm:"not null;default:'FULL'" json:"strategy"`
	// Schedule. Empty CronExpr = manual-only policy.
	CronExpr string `gorm:"default:''" json:"cron_expr"`
	Timezone string `gorm:"default:'UTC'" json:"timezone"`
	Enabled  bool   `gorm:"default:true" json:"enabled"`
	// Processing (mirrors the job-inline equivalents they override).
	Mode          VgBackupMode `gorm:"not null;default:'NORMAL'" json:"mode"`
	Encrypted     bool         `gorm:"default:false" json:"encrypted"`
	RetentionDays int          `gorm:"default:30" json:"retention_days"`
	// Verification: when true, LOCAL artifacts are server-verified after
	// COMPLETED and only verified points are restore-eligible.
	VerificationEnabled bool `gorm:"default:true" json:"verification_enabled"`
	// Retry: failed runs are refired as NEW linked runs (terminal states
	// never transition out), capped at MaxRetries with DelaySeconds between
	// attempts. 0 disables.
	MaxRetries        int `gorm:"default:0" json:"max_retries"`
	RetryDelaySeconds int `gorm:"default:300" json:"retry_delay_seconds"`
	// SLA / RPO / RTO targets (0 = not configured).
	SLATargetMinutes int `gorm:"default:0" json:"sla_target_minutes"`
	RPOTargetMinutes int `gorm:"default:0" json:"rpo_target_minutes"`
	RTOTargetMinutes int `gorm:"default:0" json:"rto_target_minutes"`
}

func (VgBackupPolicy) TableName() string { return "vg_backup_policies" }

// ResolvedPolicy is the effective how-to-back-up for one job: policy values
// when attached, otherwise the job's own inline settings. Every execution
// path (scheduler, RunNow, claim config, preflight) must resolve through
// here so attached and legacy jobs behave identically downstream.
type ResolvedPolicy struct {
	CronExpr            string
	Timezone            string
	Enabled             bool
	Mode                VgBackupMode
	Encrypted           bool
	RetentionDays       int
	VerificationEnabled bool
	MaxRetries          int
	RetryDelaySeconds   int
	SLATargetMinutes    int
	RPOTargetMinutes    int
	RTOTargetMinutes    int
	FromPolicy          bool
	PolicyID            *uuid.UUID
}

// EffectivePolicy resolves a job's operating settings. Schedules: a policy
// with a non-empty CronExpr wins; otherwise the job's BackupSchedule row.
// Processing/retention/SLA: policy wins wholesale when attached.
func EffectivePolicy(job *VgBackupJob) ResolvedPolicy {
	if job.Policy != nil && job.PolicyID != nil {
		p := job.Policy
		tz := p.Timezone
		if tz == "" {
			tz = "UTC"
		}
		r := ResolvedPolicy{
			Mode: p.Mode, Encrypted: p.Encrypted,
			RetentionDays:       p.RetentionDays,
			VerificationEnabled: p.VerificationEnabled,
			MaxRetries:          p.MaxRetries, RetryDelaySeconds: p.RetryDelaySeconds,
			SLATargetMinutes: p.SLATargetMinutes,
			RPOTargetMinutes: p.RPOTargetMinutes,
			RTOTargetMinutes: p.RTOTargetMinutes,
			Enabled:          p.Enabled, FromPolicy: true, PolicyID: job.PolicyID,
		}
		if p.CronExpr != "" {
			r.CronExpr, r.Timezone = p.CronExpr, tz
		} else if job.Schedule != nil {
			// Attached policy without its own schedule: keep the job's
			// schedule row, but the policy's Enabled still governs.
			r.CronExpr, r.Timezone = job.Schedule.CronExpr, job.Schedule.Timezone
		} else {
			r.CronExpr = ""
		}
		if r.Timezone == "" {
			r.Timezone = "UTC"
		}
		return r
	}
	tz, cron := "UTC", ""
	enabled := job.Enabled
	if job.Schedule != nil {
		cron, tz = job.Schedule.CronExpr, job.Schedule.Timezone
		if tz == "" {
			tz = "UTC"
		}
	}
	return ResolvedPolicy{
		CronExpr: cron, Timezone: tz, Enabled: enabled,
		Mode: job.Mode, Encrypted: job.Encrypted,
		RetentionDays:       job.RetentionDays,
		VerificationEnabled: true,
		MaxRetries:          0, RetryDelaySeconds: 300,
		SLATargetMinutes: job.SLATargetMinutes,
		RPOTargetMinutes: job.RPOTargetMinutes,
		RTOTargetMinutes: job.RTOTargetMinutes,
		FromPolicy:       false,
	}
}

// --- BackupSchedule ---

type VgBackupSchedule struct {
	VgBase
	BackupJobID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"backup_job_id"`
	CronExpr    string    `gorm:"not null" json:"cron_expr"`
	Timezone    string    `gorm:"default:'UTC'" json:"timezone"`
}

func (VgBackupSchedule) TableName() string { return "vg_backup_schedules" }

// --- BackupRun ---

type VgBackupRunStatus string

const (
	VgRunPending   VgBackupRunStatus = "PENDING"
	VgRunRunning   VgBackupRunStatus = "RUNNING"
	VgRunUploading VgBackupRunStatus = "UPLOADING"
	VgRunVerifying VgBackupRunStatus = "VERIFYING"
	VgRunCompleted VgBackupRunStatus = "COMPLETED"
	VgRunFailed    VgBackupRunStatus = "FAILED"
	VgRunCancelled VgBackupRunStatus = "CANCELLED"
)

type VgBackupRun struct {
	VgBase
	BackupJobID     uuid.UUID         `gorm:"type:uuid;not null;index" json:"backup_job_id"`
	AgentID         uuid.UUID         `gorm:"type:uuid;not null;index" json:"agent_id"`
	StorageTargetID uuid.UUID         `gorm:"type:uuid;not null" json:"storage_target_id"`
	Status          VgBackupRunStatus `gorm:"not null;default:'PENDING'" json:"status"`
	// ScheduledFor is the cron boundary this run was fired for. Uniqueness
	// is enforced by the scheduler (one PENDING run per job+boundary);
	// ad-hoc runs leave it NULL.
	ScheduledFor *time.Time `json:"scheduled_for,omitempty"`
	// CancelRequested is set by Cancel; the agent honors it at stage
	// boundaries and reports CANCELLED itself. PENDING runs flip to
	// CANCELLED immediately since no agent owns them yet.
	CancelRequested bool `gorm:"default:false" json:"cancel_requested"`
	// Retry bookkeeping: retries are NEW runs (terminal states never
	// transition out), linked here for observability. RetryOfRunID is the
	// failed run this run retries; RetryAttempt counts refires (0 = original).
	RetryOfRunID    *uuid.UUID `gorm:"type:uuid;index" json:"retry_of_run_id,omitempty"`
	RetryAttempt    int        `gorm:"default:0" json:"retry_attempt"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	BytesRead       int64      `json:"bytes_read"`
	BytesCompressed int64      `json:"bytes_compressed"`
	BytesUploaded   int64      `json:"bytes_uploaded"`
	DurationSeconds int64      `json:"duration_seconds"`
	SourceType      string     `json:"source_type"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	// FailureCategory classifies the error for structured alerting and UI display.
	FailureCategory string `gorm:"default:''" json:"failure_category,omitempty"`
	StoragePath     string `json:"storage_path"`
	Checksum        string `json:"checksum"`
	// DataKeyEncrypted holds the per-backup AES-256 data key, envelope-
	// encrypted with the server key. The server never sees plaintext
	// backup data, only the wrapped key.
	DataKeyEncrypted string      `json:"-"`
	BackupJob        VgBackupJob `gorm:"foreignKey:BackupJobID" json:"backup_job,omitempty"`
}

func (VgBackupRun) TableName() string { return "vg_backup_runs" }

// CanTransition returns true if moving from → to is a valid state transition.
func CanTransition(from, to VgBackupRunStatus) bool {
	switch from {
	case VgRunPending:
		return to == VgRunRunning || to == VgRunCancelled
	case VgRunRunning:
		return to == VgRunUploading || to == VgRunFailed || to == VgRunCancelled
	case VgRunUploading:
		return to == VgRunVerifying || to == VgRunFailed || to == VgRunCancelled
	case VgRunVerifying:
		return to == VgRunCompleted || to == VgRunFailed || to == VgRunCancelled
	}
	return false
}

type VgBackupArtifact struct {
	VgBase
	// Repeat registration (same run + name) must return the existing row,
	// so (backup_run_id, name) is unique: concurrent duplicates collapse
	// instead of forking the artifact set.
	BackupRunID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_vg_artifact_run_name" json:"backup_run_id"`
	Name        string    `gorm:"not null;uniqueIndex:idx_vg_artifact_run_name" json:"name"`
	Size        int64     `json:"size"`
	Checksum    string    `json:"checksum"`
	StoragePath string    `json:"storage_path"`
	// StorageTargetID pins the exact target this artifact was written to,
	// even if the job's target is changed or deleted later.
	StorageTargetID uuid.UUID `gorm:"type:uuid;index" json:"storage_target_id"`
	// Verification tracks HEAD/checksum confirmation: PENDING → VERIFIED | FAILED.
	VerifiedAt         *time.Time `json:"verified_at,omitempty"`
	VerificationStatus string     `gorm:"default:'PENDING'" json:"verification_status"`
}

func (VgBackupArtifact) TableName() string { return "vg_backup_artifacts" }

const (
	VgArtifactUnverified = "PENDING"
	VgArtifactVerified   = "VERIFIED"
	VgArtifactVerifyFail = "FAILED"
)

// --- BackupChunk ---

type VgBackupChunk struct {
	VgBase
	// Idempotency key is (artifact_id, index): chunk N of one artifact.
	// Both columns share one composite unique index — a single-column
	// unique on index alone would collide chunk N across different
	// artifacts of the same run.
	ArtifactID  uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:idx_vg_chunk_artifact_index" json:"artifact_id"`
	Index       int       `gorm:"not null;uniqueIndex:idx_vg_chunk_artifact_index" json:"index"`
	Size        int64     `json:"size"`
	Checksum    string    `json:"checksum"`
	StoragePath string    `json:"storage_path"`
	Uploaded    bool      `gorm:"default:false" json:"uploaded"`
}

func (VgBackupChunk) TableName() string { return "vg_backup_chunks" }

// --- RestoreJob ---

type VgRestoreStatus string

const (
	VgRestorePending   VgRestoreStatus = "PENDING"
	VgRestoreRunning   VgRestoreStatus = "RUNNING"
	VgRestoreCompleted VgRestoreStatus = "COMPLETED"
	VgRestoreFailed    VgRestoreStatus = "FAILED"
)

// CanRestoreTransition returns true if moving from → to is a valid restore
// state transition. Terminal states never transition out; a repeated report
// of the current state is idempotent at the handler layer, not a transition.
func CanRestoreTransition(from, to VgRestoreStatus) bool {
	switch from {
	case VgRestorePending:
		return to == VgRestoreRunning || to == VgRestoreFailed
	case VgRestoreRunning:
		return to == VgRestoreCompleted || to == VgRestoreFailed
	}
	return false
}

type VgRestoreJob struct {
	VgBase
	BackupRunID uuid.UUID `gorm:"type:uuid;not null;index" json:"backup_run_id"`
	AgentID     uuid.UUID `gorm:"type:uuid;not null;index" json:"agent_id"`
	// ConnectionID pins the saved connection for database restores, copied
	// from the source backup job at creation. NULL for filesystem restores
	// and legacy rows (agent env fallback applies).
	ConnectionID    *uuid.UUID            `gorm:"type:uuid;index" json:"connection_id,omitempty"`
	Status          VgRestoreStatus       `gorm:"not null;default:'PENDING'" json:"status"`
	DestinationPath string                `json:"destination_path"`
	TargetDatabase  string                `json:"target_database"`
	StartedAt       *time.Time            `json:"started_at"`
	CompletedAt     *time.Time            `json:"completed_at"`
	DurationSeconds int64                 `json:"duration_seconds"`
	ErrorMessage    string                `json:"error_message,omitempty"`
	BackupRun       VgBackupRun           `gorm:"foreignKey:BackupRunID" json:"backup_run,omitempty"`
	Connection      *VgDatabaseConnection `gorm:"foreignKey:ConnectionID" json:"connection,omitempty"`
}

func (VgRestoreJob) TableName() string { return "vg_restore_jobs" }

// --- BackupSizeBaseline ---
// Tracks rolling average backup size per job for anomaly detection.

type VgBackupSizeBaseline struct {
	VgBase
	BackupJobID   uuid.UUID `gorm:"type:uuid;not null;uniqueIndex" json:"backup_job_id"`
	AvgBytes      int64     `gorm:"not null;default:0" json:"avg_bytes"`
	SampleCount   int       `gorm:"not null;default:0" json:"sample_count"`
	LastUpdatedAt time.Time `json:"last_updated_at"`
}

func (VgBackupSizeBaseline) TableName() string { return "vg_backup_size_baselines" }
