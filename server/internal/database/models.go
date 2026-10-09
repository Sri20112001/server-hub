package database

import "time"

// GORM entities define the Postgres schema (created via CreateTable for
// missing tables; existing tables are never rebuilt). The API DTOs in
// internal/models stay untouched; these are persistence-only.

// TableName overrides keep GORM from pluralizing differently.
type User struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username     string    `gorm:"uniqueIndex;not null" json:"username"`
	PasswordHash string    `gorm:"not null" json:"-"`
	Role         string    `gorm:"not null;default:admin" json:"role"`
	CreatedAt    time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (User) TableName() string { return "users" }

type Project struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string    `gorm:"not null;index" json:"name"`
	Description    string    `gorm:"not null;default:''" json:"description"`
	Repository     string    `gorm:"not null;default:''" json:"repository"`
	Branch         string    `gorm:"not null;default:main" json:"branch"`
	Environment    string    `gorm:"not null;default:production" json:"environment"`
	DeploymentPath string    `gorm:"not null;default:''" json:"deploymentPath"`
	ComposeFile    string    `gorm:"not null;default:docker-compose.yml" json:"composeFile"`
	GatewayPrefix  string    `gorm:"not null;default:''" json:"gatewayPrefix"`
	HealthURL      string    `gorm:"not null;default:''" json:"healthUrl"`
	Status         string    `gorm:"not null;default:unknown;index" json:"status"`
	AutoDeploy     bool      `gorm:"not null;default:false" json:"autoDeploy"`
	CreatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (Project) TableName() string { return "projects" }

type Service struct {
	ID                uint   `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID         uint   `gorm:"not null;uniqueIndex:idx_services_proj_name" json:"projectId"`
	Name              string `gorm:"not null;uniqueIndex:idx_services_proj_name" json:"name"`
	Type              string `gorm:"not null;default:other" json:"type"`
	ContainerName     string `gorm:"not null;default:''" json:"containerName"`
	InternalPort      *int   `json:"internalPort"`
	HostPort          *int   `json:"hostPort"`
	HealthURL         string `gorm:"not null;default:''" json:"healthUrl"`
	DockerServiceName string `gorm:"not null;default:''" json:"dockerServiceName"`
	Status            string `gorm:"not null;default:unknown;index" json:"status"`
	LastHealth        string `gorm:"not null;default:UNKNOWN" json:"lastHealth"`
	LastHealthAt      string `gorm:"not null;default:''" json:"lastHealthAt"`
	ResponseTimeMs    *int64 `json:"responseTimeMs"`
}

func (Service) TableName() string { return "services" }

type Deployment struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID   uint      `gorm:"not null;index" json:"projectId"`
	CommitSHA   string    `gorm:"not null;default:''" json:"commitSha"`
	Branch      string    `gorm:"not null;default:''" json:"branch"`
	Trigger     string    `gorm:"not null;default:manual" json:"trigger"`
	Status      string    `gorm:"not null;default:PENDING;index" json:"status"`
	StartedAt   time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
	DurationSec *int64    `json:"durationSec"`
	Logs        string    `gorm:"not null;default:''" json:"logs"`
}

func (Deployment) TableName() string { return "deployments" }

type Secret struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID      uint      `gorm:"not null;uniqueIndex:idx_secrets_proj_env_name" json:"projectId"`
	Name           string    `gorm:"not null;uniqueIndex:idx_secrets_proj_env_name" json:"name"`
	Environment    string    `gorm:"not null;default:production;uniqueIndex:idx_secrets_proj_env_name" json:"environment"`
	ServiceID      *uint     `json:"serviceId"`
	EncryptedValue string    `gorm:"not null" json:"-"`
	Nonce          string    `gorm:"not null" json:"-"`
	UpdatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (Secret) TableName() string { return "secrets" }

type GatewayRoute struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID *uint     `json:"projectId"`
	Host      string    `gorm:"not null;default:''" json:"host"`
	PathPrefix string   `gorm:"not null" json:"pathPrefix"`
	Target    string    `gorm:"not null" json:"target"`
	Enabled   bool      `gorm:"not null;default:true" json:"enabled"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (GatewayRoute) TableName() string { return "gateway_routes" }

type AuditLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Actor      string    `gorm:"not null;index" json:"actor"`
	Action     string    `gorm:"not null;index" json:"action"`
	Resource   string    `gorm:"not null;index" json:"resource"`
	ResourceID string    `gorm:"not null;default:''" json:"resourceId"`
	Timestamp  time.Time `gorm:"index;default:CURRENT_TIMESTAMP" json:"timestamp"`
	Result     string    `gorm:"not null;default:ok" json:"result"`
	Metadata   string    `gorm:"not null;default:''" json:"metadata"`
}

func (AuditLog) TableName() string { return "audit_logs" }

type Operation struct {
	ID          string    `gorm:"primaryKey" json:"id"`
	Type        string    `gorm:"not null;index" json:"type"`
	TargetType  string    `gorm:"not null;default:''" json:"targetType"`
	TargetID    string    `gorm:"not null;default:''" json:"targetId"`
	Status      string    `gorm:"not null;default:QUEUED;index" json:"status"`
	Stage       string    `gorm:"not null;default:''" json:"stage"`
	Stages      string    `gorm:"not null;default:'[]'" json:"stages"`
	InitiatedBy string    `gorm:"not null;default:''" json:"initiatedBy"`
	Error       string    `gorm:"not null;default:''" json:"error"`
	CreatedAt   time.Time `gorm:"index;default:CURRENT_TIMESTAMP" json:"createdAt"`
	StartedAt   time.Time `json:"startedAt"`
	CompletedAt time.Time `json:"completedAt"`
}

func (Operation) TableName() string { return "operations" }

type Backup struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ProjectID uint      `gorm:"not null;index" json:"projectId"`
	Kind      string    `gorm:"not null;default:snapshot" json:"kind"`
	Path      string    `gorm:"not null" json:"-"`
	SizeBytes int64     `gorm:"not null;default:0" json:"sizeBytes"`
	Status    string    `gorm:"not null;default:SUCCESS" json:"status"`
	CreatedAt time.Time `gorm:"index;default:CURRENT_TIMESTAMP" json:"createdAt"`
	Logs      string    `gorm:"not null;default:''" json:"-"`
}

func (Backup) TableName() string { return "backups" }

type ServerSnapshot struct {
	Ts       int64   `gorm:"primaryKey" json:"ts"`
	CPU      float64 `gorm:"not null;default:0" json:"cpu"`
	MemPct   float64 `gorm:"not null;default:0" json:"memPct"`
	MemUsedMB float64 `gorm:"not null;default:0" json:"memMB"`
	DiskPct  float64 `gorm:"not null;default:0" json:"diskPct"`
	NetRx    int64   `gorm:"not null;default:0" json:"netRx"`
	NetTx    int64   `gorm:"not null;default:0" json:"netTx"`
	DiskRead int64   `gorm:"not null;default:0" json:"diskRead"`
	DiskWrite int64  `gorm:"not null;default:0" json:"diskWrite"`
}

func (ServerSnapshot) TableName() string { return "server_snapshots" }

// DbServer stores a known database server plus its (encrypted) credentials.
// Detection itself is live (Docker labels, host port probes, registered
// services); this table only persists what the user saved via Connect so
// browsing and health checks work without retyping passwords.
type DbServer struct {
	ID                uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Key               string    `gorm:"uniqueIndex;not null" json:"key"` // source|engine|host|port|container
	Engine            string    `gorm:"not null" json:"engine"`          // postgres | mysql | redis | mongo
	Source            string    `gorm:"not null;default:''" json:"source"`
	Host              string    `gorm:"not null;default:''" json:"host"`
	Port              int       `gorm:"not null;default:0" json:"port"`
	Container         string    `gorm:"not null;default:''" json:"container"`
	Username          string    `gorm:"not null;default:''" json:"username"`
	EncryptedPassword string    `gorm:"not null;default:''" json:"-"`
	Nonce             string    `gorm:"not null;default:''" json:"-"`
	DefaultDB         string    `gorm:"not null;default:''" json:"defaultDb"`
	UpdatedAt         time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (DbServer) TableName() string { return "db_servers" }

// DbRegistration links one chosen database to the service row created for it,
// so re-scans can show "registered" instead of offering it again.
type DbRegistration struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerKey string    `gorm:"not null;uniqueIndex:idx_dbreg_server_db" json:"serverKey"`
	Database  string    `gorm:"not null;uniqueIndex:idx_dbreg_server_db" json:"database"`
	ProjectID uint      `gorm:"not null;index" json:"projectId"`
	ServiceID uint      `gorm:"not null" json:"serviceId"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (DbRegistration) TableName() string { return "db_registrations" }

// AppSetting is a simple key/value store for server configuration edited via
// the API (notification channels, etc.). Secrets are stored encrypted with
// an "enc:" prefix; everything else is plaintext.
type AppSetting struct {
	Key   string `gorm:"primaryKey" json:"key"`
	Value string `gorm:"not null;default:''" json:"value"`
}

func (AppSetting) TableName() string { return "app_settings" }

// RefreshToken stores one side of the session refresh flow. Only the
// SHA-256 hash of the token is persisted (a DB leak must not yield live
// sessions). Tokens rotate on every use: presenting an already-revoked
// token is treated as theft and revokes every token for that user.
type RefreshToken struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	TokenHash string    `gorm:"uniqueIndex;not null" json:"-"`
	Username  string    `gorm:"index;not null" json:"username"`
	ExpiresAt time.Time `gorm:"index;not null" json:"expiresAt"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	Revoked   bool      `gorm:"not null;default:false" json:"-"`
}

func (RefreshToken) TableName() string { return "refresh_tokens" }

// ─── Managed Servers (Phase 1-9) ────────────────────────────────────────────

// ServerGroup organises managed servers (e.g. Production, Development).
type ServerGroup struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null" json:"name"`
	Description string    `gorm:"not null;default:''" json:"description"`
	CreatedAt   time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (ServerGroup) TableName() string { return "server_groups" }

// ManagedServer is a remote Linux server registered in ServerHub.
type ManagedServer struct {
	ID            uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name          string     `gorm:"not null;index" json:"name"`
	Hostname      string     `gorm:"not null;default:''" json:"hostname"`
	IPAddress     string     `gorm:"not null;default:''" json:"ipAddress"`
	OS            string     `gorm:"not null;default:''" json:"os"`
	OSVersion     string     `gorm:"not null;default:''" json:"osVersion"`
	Arch          string     `gorm:"not null;default:''" json:"arch"`
	CPUInfo       string     `gorm:"not null;default:''" json:"cpuInfo"`
	CPUCores      int        `gorm:"not null;default:0" json:"cpuCores"`
	RAMTotal      int64      `gorm:"not null;default:0" json:"ramTotal"`
	DiskTotal     int64      `gorm:"not null;default:0" json:"diskTotal"`
	Status        string     `gorm:"not null;default:UNKNOWN;index" json:"status"`
	AgentStatus   string     `gorm:"not null;default:UNKNOWN" json:"agentStatus"`
	LastHeartbeat *time.Time `gorm:"index" json:"lastHeartbeat"`
	GroupID       *uint      `gorm:"index" json:"groupId"`
	CreatedAt     time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt     time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (ManagedServer) TableName() string { return "managed_servers" }

// AgentToken authenticates a ServerHub Agent. Only the SHA-256 hash is stored.
type AgentToken struct {
	ID         uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID   uint       `gorm:"not null;index" json:"serverId"`
	TokenHash  string     `gorm:"uniqueIndex;not null" json:"-"`
	Label      string     `gorm:"not null;default:''" json:"label"`
	Revoked    bool       `gorm:"not null;default:false" json:"revoked"`
	CreatedAt  time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	LastUsedAt *time.Time `json:"lastUsedAt"`
}

func (AgentToken) TableName() string { return "agent_tokens" }

// ServerMetric stores periodic resource snapshots from a managed server.
type ServerMetric struct {
	ID           uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID     uint      `gorm:"not null;index:idx_sm_server_ts" json:"serverId"`
	Timestamp    time.Time `gorm:"not null;index:idx_sm_server_ts" json:"timestamp"`
	CPUUsage     float64   `gorm:"not null;default:0" json:"cpuUsage"`
	MemoryUsage  float64   `gorm:"not null;default:0" json:"memoryUsage"`
	MemoryUsedMB float64   `gorm:"not null;default:0" json:"memoryUsedMB"`
	DiskUsage    float64   `gorm:"not null;default:0" json:"diskUsage"`
	DiskUsedGB   float64   `gorm:"not null;default:0" json:"diskUsedGB"`
	NetRx        int64     `gorm:"not null;default:0" json:"netRx"`
	NetTx        int64     `gorm:"not null;default:0" json:"netTx"`
	LoadAvg1     float64   `gorm:"not null;default:0" json:"loadAvg1"`
	UptimeSec    int64     `gorm:"not null;default:0" json:"uptimeSec"`
}

func (ServerMetric) TableName() string { return "server_metrics" }

// Alert is a threshold-triggered alert with TRIGGERED/RESOLVED state machine.
// Fingerprint carries the Alertmanager fingerprint for external alerts so
// repeated webhook deliveries update one row instead of duplicating it.
type Alert struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	ServerID    *uint      `gorm:"index" json:"serverId"`
	Condition   string     `gorm:"not null" json:"condition"`
	Threshold   float64    `gorm:"not null;default:0" json:"threshold"`
	Severity    string     `gorm:"not null;default:WARNING" json:"severity"`
	Status      string     `gorm:"not null;default:TRIGGERED;index" json:"status"`
	Message     string     `gorm:"not null;default:''" json:"message"`
	TriggeredAt time.Time  `gorm:"index;default:CURRENT_TIMESTAMP" json:"triggeredAt"`
	ResolvedAt  *time.Time `json:"resolvedAt"`
	Fingerprint string     `gorm:"index;not null;default:''" json:"fingerprint"`
	Source      string     `gorm:"not null;default:''" json:"source"`
}

func (Alert) TableName() string { return "alerts" }

// InAppNotification is the in-app notification center entry.
type InAppNotification struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Username  string    `gorm:"index;not null" json:"username"`
	Title     string    `gorm:"not null" json:"title"`
	Body      string    `gorm:"not null;default:''" json:"body"`
	Category  string    `gorm:"not null;default:system" json:"category"`
	Read      bool      `gorm:"not null;default:false" json:"read"`
	ServerID  *uint     `gorm:"index" json:"serverId"`
	AlertID   *uint     `gorm:"index" json:"alertId"`
	CreatedAt time.Time `gorm:"index;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (InAppNotification) TableName() string { return "in_app_notifications" }

// AppLog is the central activity/log store for the future log-aggregator UI.
// Every significant event (HTTP requests, deploys, health transitions,
// auth, backups) lands here with a level + source + optional project link.
type AppLog struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Timestamp  time.Time `gorm:"index;not null" json:"timestamp"`
	Level      string    `gorm:"index;not null;default:INFO" json:"level"` // DEBUG|INFO|WARN|ERROR
	Source     string    `gorm:"index;not null" json:"source"`             // api|deploy|health|auth|backup|discovery|system
	Actor      string    `gorm:"index;default:''" json:"actor"`
	Action     string    `gorm:"index;default:''" json:"action"`
	Resource   string    `gorm:"default:''" json:"resource"`
	ResourceID string    `gorm:"default:''" json:"resourceId"`
	ProjectID  *uint     `gorm:"index" json:"projectId"`
	Message    string    `gorm:"not null" json:"message"`
	Metadata   string    `gorm:"default:''" json:"metadata"` // JSON string (text column, kept for existing installs)
	RequestID  string    `gorm:"index;default:''" json:"requestId"`
	Method     string    `gorm:"default:''" json:"method"`
	Path       string    `gorm:"index;default:''" json:"path"`
	StatusCode int       `gorm:"default:0" json:"statusCode"`
	LatencyMs  int64     `gorm:"default:0" json:"latencyMs"`
}

func (AppLog) TableName() string { return "app_logs" }

// ─── Notification Groups (Phase 1: multi-recipient email) ───────────────────

// NotificationGroup names a set of email recipients (e.g. "Default
// Notifications", "Infrastructure Team"). Members live in
// notification_group_members; deleting a group cascades to its members.
type NotificationGroup struct {
	ID          uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string    `gorm:"uniqueIndex;not null" json:"name"`
	Description string    `gorm:"not null;default:''" json:"description"`
	// Quiet hours ("HH:MM" local to QuietTZ, overnight spans allowed).
	// Empty start/end disables quiet hours for the group.
	QuietStart         string    `gorm:"not null;default:''" json:"quietStart"`
	QuietEnd           string    `gorm:"not null;default:''" json:"quietEnd"`
	QuietTZ            string    `gorm:"not null;default:'UTC'" json:"quietTZ"`
	QuietAllowCritical bool      `gorm:"not null;default:true" json:"quietAllowCritical"`
	CreatedAt   time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt   time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (NotificationGroup) TableName() string { return "notification_groups" }

// NotificationGroupMember is one email recipient in a group. Email is stored
// lowercased+trimmed with a per-group unique constraint (duplicates rejected).
type NotificationGroupMember struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	GroupID   uint      `gorm:"not null;uniqueIndex:idx_ngm_group_email,priority:1" json:"groupId"`
	Email     string    `gorm:"not null;uniqueIndex:idx_ngm_group_email,priority:2" json:"email"`
	CreatedAt time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	// Association exists for the FK cascade only; never serialized.
	Group NotificationGroup `gorm:"foreignKey:GroupID;constraint:OnDelete:CASCADE" json:"-"`
}

func (NotificationGroupMember) TableName() string { return "notification_group_members" }

// ─── Notification Rules (Phase 2 engine) ────────────────────────────────────

// NotificationRule routes matching events to a notification group.
// Channels is a comma-separated subset of {EMAIL, IN_APP}.
// ConditionJSON holds an optional structured Condition (rules package);
// empty means "match on event type (+ optional severity) only".
// CooldownSeconds suppresses repeat notifications for the same logical
// alert within the window (0 = notify every transition, max 30 days).
type NotificationRule struct {
	ID                  uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	Name                string    `gorm:"uniqueIndex;not null" json:"name"`
	Description         string    `gorm:"not null;default:''" json:"description"`
	Enabled             bool      `gorm:"not null;default:true;index" json:"enabled"`
	EventType           string    `gorm:"not null;index" json:"eventType"`
	Severity            string    `gorm:"not null;default:''" json:"severity"`
	ConditionJSON       string    `gorm:"not null;default:''" json:"conditionJson"`
	NotificationGroupID uint      `gorm:"not null;index" json:"notificationGroupId"`
	Channels            string    `gorm:"not null;default:'EMAIL'" json:"channels"`
	CooldownSeconds     int       `gorm:"not null;default:0" json:"cooldownSeconds"`
	NotifyOnRecovery    bool      `gorm:"not null;default:false" json:"notifyOnRecovery"`
	// Repeat reminders while an incident stays firing (0 = use policy default).
	RepeatIntervalSec int `gorm:"not null;default:0" json:"repeatIntervalSec"`
	// MaxRepeats caps repeat reminders per incident cycle (0 = policy default).
	MaxRepeats int `gorm:"not null;default:0" json:"maxRepeats"`
	// DigestMode immediate|digest (digest collapses a window into one summary).
	DigestMode string `gorm:"not null;default:'immediate'" json:"digestMode"`
	// DigestIntervalMin window in minutes when digest mode is on.
	DigestIntervalMin int `gorm:"not null;default:60" json:"digestIntervalMin"`
	// GroupBy comma-separated label keys splitting digest batches (max 3).
	GroupBy string `gorm:"not null;default:''" json:"groupBy"`
	CreatedBy           string    `gorm:"not null;default:''" json:"createdBy"`
	UpdatedBy           string    `gorm:"not null;default:''" json:"updatedBy"`
	CreatedAt           time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt           time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (NotificationRule) TableName() string { return "notification_rules" }

// NotificationRuleState tracks per-rule, per-alert notification progress so
// cooldowns and recovery semantics survive restarts. Identity is
// (rule_id, fingerprint) with a unique constraint — the same constraint
// that makes concurrent evaluation safe.
type NotificationRuleState struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	RuleID         uint      `gorm:"not null;uniqueIndex:idx_rrs_rule_fp,priority:1" json:"ruleId"`
	Fingerprint    string    `gorm:"not null;uniqueIndex:idx_rrs_rule_fp,priority:2" json:"fingerprint"`
	LastNotifiedAt time.Time `gorm:"not null;index" json:"lastNotifiedAt"`
	LastState      string    `gorm:"not null;default:''" json:"lastState"`
	LastEventAt    time.Time `gorm:"not null" json:"lastEventAt"`
	CreatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (NotificationRuleState) TableName() string { return "notification_rule_state" }

// ─── Centralized delivery policy (spam-proof notifications) ────────────────

// DeliveryPolicy is the single global notification policy row (id=1,
// seeded with safe defaults). All channels share it: dedupe, cooldowns,
// repeats, rate budgets, digest defaults and the emergency pause.
type DeliveryPolicy struct {
	ID                    uint       `gorm:"primaryKey" json:"id"`
	EmergencyPause        bool       `gorm:"not null;default:false" json:"emergencyPause"`
	PauseUntil            *time.Time `json:"pauseUntil"`
	PauseReason           string     `gorm:"not null;default:''" json:"pauseReason"`
	CooldownCriticalSec   int        `gorm:"not null;default:900" json:"cooldownCriticalSec"`
	CooldownWarningSec    int        `gorm:"not null;default:3600" json:"cooldownWarningSec"`
	CooldownInfoSec       int        `gorm:"not null;default:21600" json:"cooldownInfoSec"`
	RepeatIntervalSec     int        `gorm:"not null;default:3600" json:"repeatIntervalSec"`
	MaxRepeats            int        `gorm:"not null;default:3" json:"maxRepeats"`
	NotifyOnRecovery      bool       `gorm:"not null;default:true" json:"notifyOnRecovery"`
	EmailPerHour          int        `gorm:"not null;default:60" json:"emailPerHour"`
	TgPerHour             int        `gorm:"not null;default:60" json:"tgPerHour"`
	DigestIntervalMin     int        `gorm:"not null;default:60" json:"digestIntervalMin"`
	UpdatedBy             string     `gorm:"not null;default:''" json:"updatedBy"`
	UpdatedAt             time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (DeliveryPolicy) TableName() string { return "notification_policy" }

// DeliveryState is the concurrency-safe per-incident policy state keyed by
// the stable dedupe key. Claimed inside a row-locked transaction, so
// concurrent workers cannot double-notify one incident.
type DeliveryState struct {
	ID               uint      `gorm:"primaryKey;autoIncrement" json:"-"`
	DedupeKey        string    `gorm:"uniqueIndex;not null" json:"dedupeKey"`
	Severity         string    `gorm:"not null;default:''" json:"severity"`
	FirstNotifiedAt  time.Time `gorm:"not null" json:"firstNotifiedAt"`
	LastNotifiedAt   time.Time `gorm:"not null;index" json:"lastNotifiedAt"`
	RepeatCount      int       `gorm:"not null;default:0" json:"repeatCount"`
	LastState        string    `gorm:"not null;default:''" json:"lastState"`
	SuppressedCount  int       `gorm:"not null;default:0" json:"suppressedCount"`
	LastSuppressReason string  `gorm:"not null;default:''" json:"lastSuppressReason"`
	UpdatedAt        time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (DeliveryState) TableName() string { return "delivery_state" }

// DeliveryOutbox is one pending channel delivery. Enqueued in the same
// transaction as the state claim (atomic); the worker sends after commit.
type DeliveryOutbox struct {
	ID          uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	DedupeKey   string     `gorm:"index;not null" json:"dedupeKey"`
	Severity    string     `gorm:"not null;default:''" json:"severity"`
	Channel     string     `gorm:"not null;index" json:"channel"`
	GroupID     uint       `gorm:"not null;default:0" json:"groupId"`
	Title       string     `gorm:"not null" json:"title"`
	Body        string     `gorm:"not null;default:''" json:"body"`
	Status      string     `gorm:"not null;index;default:'pending'" json:"status"`
	Attempts    int        `gorm:"not null;default:0" json:"attempts"`
	NextRetryAt time.Time  `gorm:"not null;index" json:"nextRetryAt"`
	LastError   string     `gorm:"not null;default:''" json:"lastError"`
	CreatedAt   time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	SentAt      *time.Time `json:"sentAt"`
}

func (DeliveryOutbox) TableName() string { return "delivery_outbox" }

// DeliveryAttempt records one provider send attempt for observability.
type DeliveryAttempt struct {
	ID        uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	OutboxID  uint      `gorm:"index;not null" json:"outboxId"`
	Ok        bool      `gorm:"not null" json:"ok"`
	Error     string    `gorm:"not null;default:''" json:"error"`
	LatencyMs int64     `gorm:"not null;default:0" json:"latencyMs"`
	CreatedAt time.Time `gorm:"index;default:CURRENT_TIMESTAMP" json:"createdAt"`
}

func (DeliveryAttempt) TableName() string { return "delivery_attempts" }

// ProviderState holds the circuit breaker per channel.
type ProviderState struct {
	Channel              string     `gorm:"primaryKey" json:"channel"`
	ConsecutiveFailures  int        `gorm:"not null;default:0" json:"consecutiveFailures"`
	OpenedUntil          *time.Time `json:"openedUntil"`
	LastSuccessAt        *time.Time `json:"lastSuccessAt"`
	LastFailureAt        *time.Time `json:"lastFailureAt"`
}

func (ProviderState) TableName() string { return "delivery_provider_state" }

// DigestBatch accumulates one summary window for digest-mode rules.
type DigestBatch struct {
	ID         uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	DigestKey  string    `gorm:"uniqueIndex:idx_digest_key_window,priority:1;not null" json:"digestKey"`
	WindowEnd  time.Time `gorm:"uniqueIndex:idx_digest_key_window,priority:2;not null" json:"windowEnd"`
	Channel    string    `gorm:"not null" json:"channel"`
	GroupID    uint      `gorm:"not null;default:0" json:"groupId"`
	Severity   string    `gorm:"not null;default:''" json:"severity"`
	Count      int       `gorm:"not null;default:0" json:"count"`
	Titles     string    `gorm:"not null;default:''" json:"titles"`
	Resources  string    `gorm:"not null;default:''" json:"resources"`
	Status     string    `gorm:"not null;default:'open';index" json:"status"`
	CreatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt  time.Time `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (DigestBatch) TableName() string { return "digest_batches" }

// MaintenanceWindow suppresses non-critical delivery for a scope while active.
type MaintenanceWindow struct {
	ID        uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name      string     `gorm:"not null" json:"name"`
	Scope     string     `gorm:"not null;default:'all';index" json:"scope"`
	StartsAt  time.Time  `gorm:"not null;index" json:"startsAt"`
	EndsAt    time.Time  `gorm:"not null;index" json:"endsAt"`
	Reason    string     `gorm:"not null;default:''" json:"reason"`
	Enabled   bool       `gorm:"not null;default:true" json:"enabled"`
	CreatedBy string     `gorm:"not null;default:''" json:"createdBy"`
	CreatedAt time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (MaintenanceWindow) TableName() string { return "maintenance_windows" }
