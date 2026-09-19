package models

// Service types: frontend | backend | worker | database | cache | proxy | other
// Health states: HEALTHY | DEGRADED | DOWN | UNKNOWN
// Deployment statuses: PENDING | RUNNING | SUCCESS | FAILED | CANCELLED

type User struct {
	ID           int64  `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	CreatedAt    string `json:"createdAt"`
}

type Project struct {
	ID            int64  `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description,omitempty"`
	Repository    string `json:"repository,omitempty"`
	Branch        string `json:"branch,omitempty"`
	Environment   string `json:"environment,omitempty"`
	DeploymentPath string `json:"deploymentPath,omitempty"`
	ComposeFile   string `json:"composeFile,omitempty"`
	GatewayPrefix string `json:"gatewayPrefix,omitempty"`
	HealthURL     string `json:"healthUrl,omitempty"`
	Status        string `json:"status"`
	AutoDeploy    bool   `json:"autoDeploy"`
	CreatedAt     string `json:"createdAt"`
	UpdatedAt     string `json:"updatedAt"`
}

type Service struct {
	ID              int64  `json:"id"`
	ProjectID       int64  `json:"projectId"`
	Name            string `json:"name"`
	Type            string `json:"type"`
	ContainerName   string `json:"containerName,omitempty"`
	InternalPort    *int   `json:"internalPort,omitempty"`
	HostPort        *int   `json:"hostPort,omitempty"`
	HealthURL       string `json:"healthUrl,omitempty"`
	DockerServiceName string `json:"dockerServiceName,omitempty"`
	Status          string `json:"status"`
	LastHealth      string `json:"lastHealth,omitempty"`
	LastHealthAt    string `json:"lastHealthAt,omitempty"`
	ResponseTimeMs  *int64 `json:"responseTimeMs,omitempty"`
}

type Deployment struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"projectId"`
	CommitSHA   string `json:"commitSha,omitempty"`
	Branch      string `json:"branch,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	Status      string `json:"status"`
	StartedAt   string `json:"startedAt"`
	CompletedAt string `json:"completedAt,omitempty"`
	DurationSec *int64 `json:"durationSec,omitempty"`
	Logs        string `json:"logs,omitempty"`
}

type SecretMeta struct {
	ID          int64  `json:"id"`
	ProjectID   int64  `json:"projectId"`
	Name        string `json:"name"`
	Environment string `json:"environment"`
	ServiceID   *int64 `json:"serviceId,omitempty"`
	Configured  bool   `json:"configured"`
	UpdatedAt   string `json:"updatedAt"`
}

type GatewayRoute struct {
	ID        int64  `json:"id"`
	ProjectID *int64 `json:"projectId,omitempty"`
	Host      string `json:"host,omitempty"`
	PathPrefix string `json:"pathPrefix"`
	Target    string `json:"target"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
}

type AuditLog struct {
	ID         int64  `json:"id"`
	Actor      string `json:"actor"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resourceId,omitempty"`
	Timestamp  string `json:"timestamp"`
	Result     string `json:"result"`
	Metadata   string `json:"metadata,omitempty"`
}
