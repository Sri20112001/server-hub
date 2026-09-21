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
