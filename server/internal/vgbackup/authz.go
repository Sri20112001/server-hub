// Backup authorization policy (Phase 1 foundation).
//
// Two layers protect every backup resource, matching how ServerHub guards
// everything else:
//
//  1. Route layer (Phase 2 handlers): the existing RequireRole middleware
//     gates reads to viewer+ and mutations to operator+, exactly like all
//     other ServerHub routes. Nothing here bypasses or replaces it.
//  2. Ownership layer (this file): within the role gate, a caller may touch
//     a resource they own (CreatedByUserID) — or anything, if admin.
//
// EXPLICIT POLICY — administrator bypass: ServerHub admins bypass ownership
// for every subsystem (containers, users, secrets, deployments); backup
// resources follow the same rule rather than inventing a stricter one.
// This is deliberately documented here instead of being implicit.
//
// Identity is always the immutable ServerHub users.id (uint). Usernames are
// never used as identity: renames cannot transfer or strand ownership, and
// uniqueness comes from the users primary key, not from string matching.
// userID 0 (unauthenticated/missing identity) is denied unconditionally,
// and unrecognized roles deny closed — including for owners.
package vgbackup

// NOTE on role strings: the literals below must equal middleware.RoleViewer,
// RoleOperator, RoleAdmin ("viewer"/"operator"/"admin"). They are literals
// rather than imports because middleware (transitively via applog) imports
// package database, which imports vgbackup — referencing the constants here
// would be an import cycle. authz_test.go asserts equality against the
// middleware constants so drift fails loudly instead of silently.
const (
	serverRoleViewer   = "viewer"
	serverRoleOperator = "operator"
	serverRoleAdmin    = "admin"
)

// BackupRole is the backup-domain role, mirroring the caller's ServerHub
// global role one-to-one. There are no backup-specific roles and no
// organization membership: the deployment is the tenant.
type BackupRole string

const (
	BackupViewer   BackupRole = "viewer"
	BackupOperator BackupRole = "operator"
	BackupAdmin    BackupRole = "admin"
)

// MapBackupRole validates a ServerHub role string (middleware.RoleViewer,
// RoleOperator, RoleAdmin) into a BackupRole. Anything else — empty,
// unknown, differently-cased legacy values — is an error so callers fail
// closed instead of guessing.
func MapBackupRole(serverRole string) (BackupRole, error) {
	switch serverRole {
	case serverRoleViewer:
		return BackupViewer, nil
	case serverRoleOperator:
		return BackupOperator, nil
	case serverRoleAdmin:
		return BackupAdmin, nil
	default:
		return "", NewVgError("unknown backup role")
	}
}

// CanAccessBackupResource reports whether (role, userID) may access the
// backup resource owned by ownerID. Admins bypass ownership per the policy
// above; owners always pass; everyone else — including viewers looking at
// someone else's resources via guessed UUIDs — is denied. userID 0 never
// passes, even against ownerID 0 (system-owned rows are admin-only).
func CanAccessBackupResource(role BackupRole, userID, ownerID uint) bool {
	if userID == 0 {
		return false
	}
	switch role {
	case BackupAdmin:
		return true
	case BackupViewer, BackupOperator:
		return userID == ownerID
	default:
		return false
	}
}

// vgError is a minimal typed error so unmapped roles are distinguishable
// from generic failures without importing the transfer package's AppError
// (vgbackup must not depend on transfer).
type vgError struct{ msg string }

func (e *vgError) Error() string { return e.msg }

func NewVgError(msg string) error { return &vgError{msg: msg} }
