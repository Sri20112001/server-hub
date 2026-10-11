package transfer

import (
	"time"

	"serverhub/internal/database"
)

// TransferAction constants mirror the homelabs-upload activity vocabulary
// so migrated history and new records share one language.
const (
	ActionUpload       = "upload"
	ActionDownload     = "download"
	ActionMkdir        = "mkdir"
	ActionRename       = "rename"
	ActionMove         = "move"
	ActionCopy         = "copy"
	ActionDelete       = "delete"
	ActionTrashMove    = "trash_move"
	ActionTrashRestore = "trash_restore"
	ActionTrashPurge   = "trash_purge"
	ActionZipDownload  = "zip_download"
	ActionBulkRename   = "bulk_rename"
)

// LogTransfer appends one transfer-history row to nv_activity.
// Best-effort: logging must never fail the file operation itself.
// Timestamps use RFC3339 UTC.
func LogTransfer(db *database.DB, user, role, action, path, detail, ip string, status int) {
	if db == nil {
		return
	}
	_, _ = db.Exec(`INSERT INTO nv_activity
		("time","user","role","action","path","detail","ip","status")
		VALUES (?,?,?,?,?,?,?,?)`,
		time.Now().UTC().Format(time.RFC3339), user, role, action, path, detail, ip, status)
}
