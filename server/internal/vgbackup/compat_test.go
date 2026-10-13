package vgbackup_test

import (
	"testing"

	"serverhub/internal/middleware"
	"serverhub/internal/vgbackup"
)

// The role literals in authz.go must equal the middleware constants.
// Importing middleware from vgbackup would be an import cycle
// (middleware → applog → database → vgbackup), so equality is pinned here
// in the external test package instead. If middleware ever renames a role,
// this fails loudly rather than silently opening or closing access.
func TestRoleLiteralsMatchMiddleware(t *testing.T) {
	if got, err := vgbackup.MapBackupRole(middleware.RoleViewer); err != nil || got != vgbackup.BackupViewer {
		t.Errorf("RoleViewer %q not accepted as viewer: %v %v", middleware.RoleViewer, got, err)
	}
	if got, err := vgbackup.MapBackupRole(middleware.RoleOperator); err != nil || got != vgbackup.BackupOperator {
		t.Errorf("RoleOperator %q not accepted as operator: %v %v", middleware.RoleOperator, got, err)
	}
	if got, err := vgbackup.MapBackupRole(middleware.RoleAdmin); err != nil || got != vgbackup.BackupAdmin {
		t.Errorf("RoleAdmin %q not accepted as admin: %v %v", middleware.RoleAdmin, got, err)
	}
}
