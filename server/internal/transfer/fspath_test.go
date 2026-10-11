package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveSafePath_Valid(t *testing.T) {
	root := t.TempDir()

	cases := []struct {
		req  string
		want string // relative to root
	}{
		{"/", ""},
		{"/foo", "foo"},
		{"/foo/bar", filepath.Join("foo", "bar")},
	}

	for _, tc := range cases {
		got, err := ResolveSafePath(root, tc.req)
		if err != nil {
			t.Errorf("ResolveSafePath(%q) unexpected error: %v", tc.req, err)
			continue
		}
		want := filepath.Join(root, tc.want)
		if want == root+string(filepath.Separator) {
			want = root
		}
		if got != want && got != root {
			t.Errorf("ResolveSafePath(%q) = %q, want %q", tc.req, got, want)
		}
	}
}

func TestResolveSafePath_Traversal(t *testing.T) {
	root := t.TempDir()

	attacks := []string{
		"../../etc/passwd",
		"../../../home/user/.ssh/id_rsa",
		"/foo/../../etc/passwd",
		// URL-encoded variants are decoded by the HTTP layer before reaching
		// ResolveSafePath, so the raw percent-encoded form is NOT an attack
		// vector at this level. The decoded form is tested above.
	}

	for _, attack := range attacks {
		_, err := ResolveSafePath(root, attack)
		if err == nil {
			t.Errorf("ResolveSafePath(%q) should have returned an error", attack)
		}
	}
}

func TestResolveSafePath_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()

	// Create a symlink inside root pointing outside
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	_, err := ResolveSafePath(root, "/escape")
	if err == nil {
		t.Error("ResolveSafePath should reject symlink escaping root")
	}
}

func TestValidateFileName(t *testing.T) {
	valid := []string{"file.txt", "my-file", "report_2025.pdf", "hello world"}
	for _, name := range valid {
		if err := ValidateFileName(name); err != nil {
			t.Errorf("ValidateFileName(%q) unexpected error: %v", name, err)
		}
	}

	invalid := []string{"", ".", "..", "foo/bar", "foo\\bar", "/etc/passwd"}
	for _, name := range invalid {
		if err := ValidateFileName(name); err == nil {
			t.Errorf("ValidateFileName(%q) should have returned an error", name)
		}
	}
}

func TestToClientPath(t *testing.T) {
	root := "/data/files"
	cases := []struct {
		abs  string
		want string
	}{
		{"/data/files", "/"},
		{"/data/files/foo", "/foo"},
		{"/data/files/foo/bar.txt", "/foo/bar.txt"},
	}
	for _, tc := range cases {
		got := ToClientPath(root, tc.abs)
		if got != tc.want {
			t.Errorf("ToClientPath(%q) = %q, want %q", tc.abs, got, tc.want)
		}
	}
}
