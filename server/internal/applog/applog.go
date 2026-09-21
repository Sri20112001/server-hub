// Package applog is the central activity/log store for ServerHub.
// Every significant event lands in app_logs so the log-aggregator UI
// (or external collector) has one table to tail. app_logs is append-only:
// DB triggers reject UPDATE and DELETE, and this package intentionally
// exposes no mutating operation besides INSERT (see Query for reads).
//
// Levels: DEBUG | INFO | WARN | ERROR
// Sources: api | deploy | health | auth | backup | discovery | system |
// webhook | container | exec | project | service | secret | monitor | server
package applog

import (
	"time"

	"gorm.io/gorm"

	"serverhub/internal/database"
)

type Entry struct {
	Level      string
	Source     string
	Actor      string
	Action     string
	Resource   string
	ResourceID string
	ProjectID  *uint
	Message    string
	Metadata   string
	RequestID  string
	Method     string
	Path       string
	StatusCode int
	LatencyMs  int64
}

// Write inserts one log row. Never fails the caller — errors are swallowed so
// logging can never break the request path. Writes are synchronous (single
// INSERT, <1ms on Postgres) to keep ordering deterministic for the aggregator.
func Write(db *gorm.DB, e Entry) {
	if db == nil {
		return
	}
	if e.Level == "" {
		e.Level = "INFO"
	}
	if e.Source == "" {
		e.Source = "system"
	}
	_ = db.Create(&database.AppLog{
		Timestamp:  time.Now().UTC(),
		Level:      e.Level,
		Source:     e.Source,
		Actor:      e.Actor,
		Action:     e.Action,
		Resource:   e.Resource,
		ResourceID: e.ResourceID,
		ProjectID:  e.ProjectID,
		Message:    e.Message,
		Metadata:   e.Metadata,
		RequestID:  e.RequestID,
		Method:     e.Method,
		Path:       e.Path,
		StatusCode: e.StatusCode,
		LatencyMs:  e.LatencyMs,
	}).Error
}

func Info(db *gorm.DB, source, message string) {
	Write(db, Entry{Level: "INFO", Source: source, Message: message})
}

func Warn(db *gorm.DB, source, message string) {
	Write(db, Entry{Level: "WARN", Source: source, Message: message})
}

func Error(db *gorm.DB, source, message string) {
	Write(db, Entry{Level: "ERROR", Source: source, Message: message})
}

// Filter is the query surface for the future aggregator UI / API.
type Filter struct {
	Level     string
	Source    string
	Search    string
	ProjectID *uint
	Limit     int
	Offset    int
}

// Query returns newest-first rows matching the filter.
func Query(db *gorm.DB, f Filter) ([]database.AppLog, error) {
	if f.Limit <= 0 || f.Limit > 500 {
		f.Limit = 100
	}
	q := db.Model(&database.AppLog{}).Order("timestamp DESC")
	if f.Level != "" {
		q = q.Where("level = ?", f.Level)
	}
	if f.Source != "" {
		q = q.Where("source = ?", f.Source)
	}
	if f.ProjectID != nil {
		q = q.Where("project_id = ?", *f.ProjectID)
	}
	if f.Search != "" {
		like := "%" + f.Search + "%"
		q = q.Where("message LIKE ? OR action LIKE ? OR resource LIKE ?", like, like, like)
	}
	var out []database.AppLog
	if err := q.Limit(f.Limit).Offset(f.Offset).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// Prune is retained for backward compatibility but is intentionally a
// no-op: app_logs is append-only and must never be deleted or altered
// (DB triggers reject DELETE/UPDATE). Retention is always "keep forever".
// It returns 0 rows affected regardless of retainDays.
func Prune(_ *gorm.DB, _ int) (int64, error) {
	return 0, nil
}

// Truncate caps oversized log payloads before persisting so a single huge
// output (e.g. container tail, compose run) cannot bloat the DB row.
func Truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
