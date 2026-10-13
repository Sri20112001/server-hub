package vgbackup

import (
	"testing"
)

// MapBackupRole must accept exactly the three ServerHub global roles and
// deny everything else closed — including VaultGuard's legacy uppercase
// spellings, which must never authenticate here.
func TestMapBackupRole(t *testing.T) {
	ok := map[string]BackupRole{
		"viewer":   BackupViewer,
		"operator": BackupOperator,
		"admin":    BackupAdmin,
	}
	for in, want := range ok {
		got, err := MapBackupRole(in)
		if err != nil || got != want {
			t.Errorf("MapBackupRole(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	denied := []string{"", "root", "OWNER", "ADMIN", "OPERATOR", "VIEWER", " viewer", "viewer ", "Admin", "superuser"}
	for _, in := range denied {
		if got, err := MapBackupRole(in); err == nil {
			t.Errorf("MapBackupRole(%q) must fail, got %q", in, got)
		}
	}
}

func TestCanAccessBackupResource(t *testing.T) {
	cases := []struct {
		name   string
		role   BackupRole
		user   uint
		owner  uint
		access bool
	}{
		{"owner reads own", BackupViewer, 7, 7, true},
		{"owner mutates own", BackupOperator, 7, 7, true},
		{"admin reads foreign", BackupAdmin, 1, 9, true},
		{"admin mutates foreign", BackupAdmin, 1, 9, true},
		{"viewer denied foreign", BackupViewer, 7, 9, false},
		{"operator denied foreign", BackupOperator, 7, 9, false},
		{"guessed-ID read denied", BackupViewer, 7, 424242, false},
		{"zero user denied even as owner", BackupViewer, 0, 0, false},
		{"zero user denied as admin", BackupAdmin, 0, 9, false},
		{"unknown role denied as owner", BackupRole("root"), 7, 7, false},
		{"unknown role denied as admin-claim", BackupRole("OWNER"), 1, 9, false},
		{"empty role denied", BackupRole(""), 7, 7, false},
	}
	for _, tc := range cases {
		if got := CanAccessBackupResource(tc.role, tc.user, tc.owner); got != tc.access {
			t.Errorf("%s: role=%q user=%d owner=%d → %v, want %v",
				tc.name, tc.role, tc.user, tc.owner, got, tc.access)
		}
	}
}

// Ownership is keyed by immutable user ID, so renames can never transfer or
// strand it: the same ID keeps access under any username, and two IDs never
// share access. This pins the ID-not-username contract, not just behavior.
func TestOwnershipSurvivesRename(t *testing.T) {
	const ownerID = uint(7)
	// Same numeric ID, "renamed" user — access unchanged.
	if !CanAccessBackupResource(BackupViewer, ownerID, ownerID) {
		t.Error("owner must keep access regardless of username")
	}
	// A different ID must not inherit it, even with an admin-sounding role
	// string that MapBackupRole would reject upstream.
	if CanAccessBackupResource(BackupViewer, 8, ownerID) {
		t.Error("different user ID must not access another's resource")
	}
}
