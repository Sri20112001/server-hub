package audit

import (
	"strings"
	"time"

	"serverhub/internal/database"
)

// Write records a security-sensitive action in audit_logs AND mirrors it
// into app_logs (the central DB log store) so no event exists only in one
// place. Both tables are append-only (DB triggers reject UPDATE/DELETE).
// Never pass secret values in metadata.
func Write(db *database.DB, actor, action, resource, resourceID, result, metadata string) {
	if db == nil {
		return
	}
	if result == "" {
		result = "ok"
	}
	_, _ = db.Exec(
		`INSERT INTO audit_logs (actor, action, resource, resource_id, result, metadata) VALUES (?,?,?,?,?,?)`,
		actor, action, resource, resourceID, result, metadata,
	)
	mirrorToAppLogs(db, actor, action, resource, resourceID, result, metadata)
}

// mirrorToAppLogs inserts the best-effort central-store copy. Failures are
// swallowed so logging can never break the request path.
func mirrorToAppLogs(db *database.DB, actor, action, resource, resourceID, result, metadata string) {
	defer func() { _ = recover() }()
	if db == nil || db.SQL == nil {
		return
	}
	source := strings.ToLower(strings.TrimSpace(resource))
	if source == "" {
		source = "system"
	}
	level := "INFO"
	lr := strings.ToLower(result)
	switch {
	case strings.Contains(lr, "fail"):
		level = "ERROR"
	case lr == "firing" || strings.Contains(lr, "warn"):
		level = "WARN"
	}
	msg := action + " " + resource
	if resourceID != "" {
		msg += " " + resourceID
	}
	msg += " -> " + result
	meta := truncate(metadata, 8000)
	_, _ = db.Exec(
		`INSERT INTO app_logs (timestamp, level, source, actor, action, resource, resource_id, message, metadata) VALUES (?,?,?,?,?,?,?,?,?)`,
		time.Now().UTC().Format("2006-01-02 15:04:05.000000"), level, source, actor, action, resource, resourceID, msg, meta,
	)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
