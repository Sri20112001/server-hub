// Health check models — added to existing models.go via db.go registration.
// Kept in a separate file to avoid touching the large models.go.
package database

import "time"

// HealthCheck defines a periodic probe (HTTP, TCP, or Ping).
type HealthCheck struct {
	ID             uint       `gorm:"primaryKey;autoIncrement" json:"id"`
	Name           string     `gorm:"not null" json:"name"`
	Type           string     `gorm:"not null;default:http" json:"type"` // http|tcp|ping
	Target         string     `gorm:"not null" json:"target"`
	Interval       int        `gorm:"not null;default:60" json:"interval"` // seconds
	Timeout        int        `gorm:"not null;default:10" json:"timeout"`  // seconds
	ExpectedStatus int        `gorm:"not null;default:200" json:"expectedStatus"`
	Enabled        bool       `gorm:"not null;default:true" json:"enabled"`
	Status         string     `gorm:"not null;default:UNKNOWN;index" json:"status"` // UP|DOWN|UNKNOWN
	ResponseTimeMs *int64     `json:"responseTimeMs"`
	LastCheckedAt  *time.Time `json:"lastCheckedAt"`
	ServerID       *uint      `gorm:"index" json:"serverId"`
	CreatedAt      time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"createdAt"`
	UpdatedAt      time.Time  `gorm:"default:CURRENT_TIMESTAMP" json:"updatedAt"`
}

func (HealthCheck) TableName() string { return "health_checks" }

// HealthCheckResult stores one probe result.
type HealthCheckResult struct {
	ID             uint      `gorm:"primaryKey;autoIncrement" json:"id"`
	HealthCheckID  uint      `gorm:"not null;index:idx_hcr_check_ts" json:"healthCheckId"`
	Timestamp      time.Time `gorm:"not null;index:idx_hcr_check_ts" json:"timestamp"`
	Status         string    `gorm:"not null" json:"status"` // UP|DOWN
	ResponseTimeMs int64     `gorm:"not null;default:0" json:"responseTimeMs"`
	Error          string    `gorm:"not null;default:''" json:"error"`
}

func (HealthCheckResult) TableName() string { return "health_check_results" }
