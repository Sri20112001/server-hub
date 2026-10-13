package vgbackup

import (
	"os"
	"path/filepath"
	"testing"
)

// Adversarial cases for VgIsAbsPath: only absolute spellings pass.
func TestVgIsAbsPathAdversarial(t *testing.T) {
	cases := []struct {
		p    string
		want bool
	}{
		{"", false},
		{"..", false},
		{"../etc", false},
		{"relative/path", false},
		{`relative\path`, false},
		{"/", true},
		{"/var/backups", true},
		{`C:\backups`, true},
		{"C:/backups", true},
		{`\\srv\share`, true},
		{"//srv/share", true},
		{"C:relative", false},
		{"~/backups", false},
	}
	for _, tc := range cases {
		if got := VgIsAbsPath(tc.p); got != tc.want {
			t.Errorf("VgIsAbsPath(%q) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

// Adversarial cases for VgIsWithinBase: traversal, prefix collisions,
// mixed separators, and dot segments must not escape base.
func TestVgIsWithinBaseAdversarial(t *testing.T) {
	cases := []struct {
		base, child string
		want        bool
	}{
		{"/var/backups", "/var/backups", true},
		{"/var/backups", "/var/backups/", true},
		{"/var/backups", "/var/backups/run-1/file.bak", true},
		{"/var/backups", "/var/backups2/evil", false},         // prefix collision
		{"/var/backups", "/var/backups/../etc/passwd", false}, // traversal
		{"/var/backups", "/var/backups/sub/../../etc", false}, // traversal via subdir
		{"/var/backups", "/etc/passwd", false},
		{"/var/backups", "relative", false},
		{"/var/backups", "/var/backups/./sub", true}, // dot segment stays inside
		{"C:/backups", `C:\backups\run-1`, true},     // mixed separators
		{`C:\backups`, "C:/backups/run-1", true},     // mixed separators, reversed
		{"C:/backups", `C:\backups2\evil`, false},
		{"C:/backups", "C:/backups/../Windows", false},
		{"/", "/anything", true}, // root contains all absolute paths
	}
	for _, tc := range cases {
		if got := VgIsWithinBase(tc.base, tc.child); got != tc.want {
			t.Errorf("VgIsWithinBase(%q, %q) = %v, want %v", tc.base, tc.child, got, tc.want)
		}
	}
}

// validateSafeStoragePath against the real filesystem: a symlink inside
// the root that points outside must be rejected; the actual os.Remove
// target must never be the escape destination.
func TestValidateSafeStoragePathSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Plain file inside root passes.
	inside := filepath.Join(root, "run-1.bak")
	if err := os.WriteFile(inside, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateSafeStoragePath(root, inside); err != nil {
		t.Fatalf("inside-root file rejected: %v", err)
	}

	// Traversal string is rejected before touching the filesystem.
	if err := validateSafeStoragePath(root, filepath.Join(root, "..", "evil")); err == nil {
		t.Fatal("traversal path accepted")
	}
	// Root-prefix collision is rejected.
	if err := validateSafeStoragePath(root, root+"-evil"+string(filepath.Separator)+"f"); err == nil {
		t.Fatal("prefix-collision path accepted")
	}
	// Relative path is rejected.
	if err := validateSafeStoragePath(root, "run-1.bak"); err == nil {
		t.Fatal("relative path accepted")
	}

	// Symlink inside root -> outside must be rejected, and the escape
	// target must survive (proving no removal happens through the link).
	victim := filepath.Join(outside, "victim.bak")
	if err := os.WriteFile(victim, []byte("precious"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link.bak")
	if err := os.Symlink(victim, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := validateSafeStoragePath(root, link); err == nil {
		t.Fatal("symlink escape accepted")
	}
	if b, err := os.ReadFile(victim); err != nil || string(b) != "precious" {
		t.Fatal("escape target disturbed")
	}

	// Symlink inside root -> inside root is accepted.
	innerLink := filepath.Join(root, "inner-link.bak")
	if err := os.Symlink(inside, innerLink); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := validateSafeStoragePath(root, innerLink); err != nil {
		t.Fatalf("inner symlink rejected: %v", err)
	}
}
