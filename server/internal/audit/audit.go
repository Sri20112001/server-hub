package audit

import "database/sql"

// Write records a security-sensitive action. Never pass secret values in metadata.
func Write(db *sql.DB, actor, action, resource, resourceID, result, metadata string) {
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
}
