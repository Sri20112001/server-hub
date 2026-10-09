package deploypath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateWrite(t *testing.T) {
	// Temp-dir roots keep the test OS-agnostic (Windows absolute paths
	// need a volume name; Linux prod uses /srv/apps style roots).
	base := t.TempDir()
	rootA := filepath.Join(base, "apps")
	rootB := filepath.Join(base, "stacks")
	roots := []string{rootA, rootB}
	ok := []string{"", rootA, filepath.Join(rootA, "myapp"), filepath.Join(rootB, "x")}
	for _, p := range ok {
		if _, err := ValidateWrite(p, roots); err != nil {
			t.Fatalf("ValidateWrite(%q) = %v, want nil", p, err)
		}
	}
	bad := []string{
		"relative/path",                    // not absolute
		rootA + "-evil" + string(filepath.Separator) + "x", // string-prefix trap
		filepath.Join(rootA, "..", "etc"),  // Clean escapes the root
		filepath.Join(base, "elsewhere"),   // outside all roots
	}
	for _, p := range bad {
		if _, err := ValidateWrite(p, roots); err == nil {
			t.Fatalf("ValidateWrite(%q) = nil, want error", p)
		}
	}
}

func TestResolveExecSymlinkEscape(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	// Symlink inside the root pointing outside must be rejected.
	link := filepath.Join(root, "evil")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := ResolveExec(link, []string{root}); err == nil {
		t.Fatal("ResolveExec(symlink escape) = nil, want error")
	} else if !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("wrong error: %v", err)
	}
	// A real directory inside the root resolves fine.
	real := filepath.Join(root, "app")
	if err := os.MkdirAll(real, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := ResolveExec(real, []string{root}); err != nil || got != real {
		t.Fatalf("ResolveExec(%q) = %q, %v", real, got, err)
	}
}

func TestRootsEdgeCases(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "apps")
	inside := filepath.Join(root, "app")

	// Empty roots: legacy mode, absolute paths allowed.
	if _, err := ValidateWrite(inside, nil); err != nil {
		t.Fatalf("empty roots must allow absolute paths: %v", err)
	}
	if _, err := ValidateWrite("relative", nil); err == nil {
		t.Fatal("empty roots must still reject relative paths")
	}
	// All-malformed roots: fail closed, not open.
	if _, err := ValidateWrite(inside, []string{"", "relative", "   "}); err == nil {
		t.Fatal("all-invalid roots must reject everything (fail closed)")
	}
	// Mixed: valid root still works alongside garbage entries.
	if _, err := ValidateWrite(inside, []string{"", root}); err != nil {
		t.Fatalf("valid root among garbage must work: %v", err)
	}
}

func TestValidateComposeFile(t *testing.T) {
	for _, ok := range []string{"docker-compose.yml", "compose.yaml", "prod.yml"} {
		if err := ValidateComposeFile(ok); err != nil {
			t.Fatalf("ValidateComposeFile(%q) = %v", ok, err)
		}
	}
	for _, bad := range []string{"", "../evil.yml", "/abs.yml", "-f", "sub/dir.yml"} {
		if err := ValidateComposeFile(bad); err == nil {
			t.Fatalf("ValidateComposeFile(%q) = nil, want error", bad)
		}
	}
}
