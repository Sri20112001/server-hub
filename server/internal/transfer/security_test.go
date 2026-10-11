package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- chunked-upload session validation -----------------------------------

func TestValidateUploadID(t *testing.T) {
	valid := []string{"abc123", "sess-1_ABC", "x", strings.Repeat("a", 64)}
	for _, id := range valid {
		if err := ValidateUploadID(id); err != nil {
			t.Errorf("ValidateUploadID(%q) unexpected error: %v", id, err)
		}
	}
	invalid := []string{
		"",
		"../escape",
		"../../etc/x",
		"/absolute",
		"a/b",
		`a\b`,
		"..",
		"has space",
		"semi;colon",
		"quote\"x",
		strings.Repeat("a", 65),
	}
	for _, id := range invalid {
		if err := ValidateUploadID(id); err == nil {
			t.Errorf("ValidateUploadID(%q) should have returned an error", id)
		}
	}
}

func TestSaveChunk_RejectsTraversalUploadID(t *testing.T) {
	svc, _ := newSvcForSecurity(t)
	for _, id := range []string{"../../pwn", "/abs", "a/b", ""} {
		if err := svc.SaveChunk(id, 0, strings.NewReader("x")); err == nil {
			t.Errorf("SaveChunk(%q) should have returned an error", id)
		}
	}
	// No chunk directory may have been created for rejected IDs.
	base := chunkBaseDir()
	entries, _ := os.ReadDir(base)
	for _, e := range entries {
		if strings.Contains(e.Name(), "pwn") || e.Name() == "abs" {
			t.Errorf("rejected upload_id created state on disk: %q", e.Name())
		}
	}
}

func TestSaveChunk_RejectsBadIndex(t *testing.T) {
	svc, _ := newSvcForSecurity(t)
	for _, idx := range []int{-1, -100, 100000} {
		if err := svc.SaveChunk("valid-id-1", idx, strings.NewReader("x")); err == nil {
			t.Errorf("SaveChunk(index=%d) should have returned an error", idx)
		}
	}
}

func TestSaveChunk_EnforcesChunkCap(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root, 10) // 10-byte budget
	if err := svc.SaveChunk("cap-test-1", 0, strings.NewReader("0123456789ABCDEF")); err == nil {
		t.Fatal("oversize chunk should have been rejected")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrFileTooLarge {
			t.Fatalf("want FILE_TOO_LARGE, got %v", err)
		}
	}
	// Rejected chunk must not linger on disk.
	entries, _ := os.ReadDir(filepath.Join(chunkBaseDir(), "cap-test-1"))
	if len(entries) != 0 {
		t.Errorf("rejected chunk left %d files behind", len(entries))
	}
}

func TestFinalizeChunk_EnforcesTotalCap(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root, 10) // 10-byte budget
	// Each chunk fits the per-chunk cap; the assembled total (16) does not.
	if err := svc.SaveChunk("total-cap-1", 0, strings.NewReader("12345678")); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveChunk("total-cap-1", 1, strings.NewReader("12345678")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FinalizeChunk(context.Background(), "total-cap-1", "/", "big.bin"); err == nil {
		t.Fatal("oversize assembly should have been rejected")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrFileTooLarge {
			t.Fatalf("want FILE_TOO_LARGE, got %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "big.bin")); !os.IsNotExist(err) {
		t.Error("rejected assembly must not leave the destination file behind")
	}
}

func TestFinalizeChunk_RoundTripUnderCap(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root, 1024)
	if err := svc.SaveChunk("roundtrip-1", 0, strings.NewReader("hello-")); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveChunk("roundtrip-1", 1, strings.NewReader("chunked")); err != nil {
		t.Fatal(err)
	}
	p, err := svc.FinalizeChunk(context.Background(), "roundtrip-1", "/", "out.txt")
	if err != nil {
		t.Fatal(err)
	}
	if p != "/out.txt" {
		t.Errorf("client path = %q, want /out.txt", p)
	}
	data, err := os.ReadFile(filepath.Join(root, "out.txt"))
	if err != nil || string(data) != "hello-chunked" {
		t.Errorf("assembled content mismatch: %v %q", err, data)
	}
	if _, err := os.Stat(filepath.Join(chunkBaseDir(), "roundtrip-1")); !os.IsNotExist(err) {
		t.Error("chunk dir should be removed after finalize")
	}
}

func TestFinalizeChunk_RejectsTraversalAndTrash(t *testing.T) {
	svc, _ := newSvcForSecurity(t)
	if err := svc.SaveChunk("fin-guard-1", 0, strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FinalizeChunk(context.Background(), "../../evil", "/", "x.txt"); err == nil {
		t.Error("traversal upload_id should be rejected")
	}
	if _, err := svc.FinalizeChunk(context.Background(), "fin-guard-1", "/.trash", "x.txt"); err == nil {
		t.Error("finalize into trash should be rejected")
	}
	if _, err := svc.FinalizeChunk(context.Background(), "fin-guard-1", "/", ".trash"); err == nil {
		t.Error("reserved filename should be rejected")
	}
}

// --- trash-area isolation --------------------------------------------------

func TestTrash_HiddenFromBrowser(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root, 10<<20)
	if err := os.WriteFile(filepath.Join(root, "gone.txt"), []byte("secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "stays.txt"), []byte("visible"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.TrashMove("/gone.txt"); err != nil {
		t.Fatal(err)
	}

	listed, err := svc.ListDirectory("/")
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range listed.Items {
		if it.Name == TrashDirName || it.Path == "/gone.txt" {
			t.Errorf("trashed content leaked into listing: %+v", it)
		}
	}

	results, err := svc.Search("gone", "/")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range results {
		if strings.Contains(r.Path, TrashDirName) {
			t.Errorf("trashed content leaked into search: %+v", r)
		}
	}

	items, err := svc.TrashList()
	if err != nil || len(items) != 1 {
		t.Fatalf("trash list = %+v, %v", items, err)
	}
	trashPath := "/" + TrashDirName + "/" + items[0].ID
	if _, _, err := svc.OpenForDownload(trashPath); err == nil {
		t.Error("trash payload must not be downloadable via file endpoints")
	}
	if _, err := svc.GetMetadata(trashPath); err == nil {
		t.Error("trash payload must not be visible via metadata")
	}
}

func TestTrash_OperationGuards(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	_ = os.WriteFile(filepath.Join(root, "f.txt"), []byte("x"), 0644)

	if _, err := svc.SaveUpload(context.Background(), "/.trash", "x.txt", strings.NewReader("x"), 1); err == nil {
		t.Error("upload into trash should be rejected")
	}
	if _, err := svc.SaveUpload(context.Background(), "/", ".trash", strings.NewReader("x"), 1); err == nil {
		t.Error("reserved filename should be rejected")
	}
	if err := svc.CreateDirectory("/.trash/evil"); err == nil {
		t.Error("mkdir inside trash should be rejected")
	}
	if err := svc.Rename("/f.txt", ".trash"); err == nil {
		t.Error("rename to reserved name should be rejected")
	}
	if err := svc.Move("/f.txt", "/.trash/f.txt"); err == nil {
		t.Error("move into trash area should be rejected")
	}
	if err := svc.Copy(context.Background(), "/f.txt", "/.trash/f.txt"); err == nil {
		t.Error("copy into trash area should be rejected")
	}
	if err := svc.Delete("/.trash/anything", false); err == nil {
		t.Error("direct delete inside trash should be rejected")
	}
	if err := svc.TrashMove("/.trash"); err == nil {
		t.Error("trashing the trash area should be rejected")
	}
}

// --- symlink hardening -----------------------------------------------------

func TestResolveSafePath_SymlinkedParent(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("s"), 0644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "sub")
	if err := os.Symlink(outside, link); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	// The link itself resolves outside.
	if _, err := ResolveSafePath(root, "/sub"); err == nil {
		t.Error("symlinked dir resolving outside should be rejected")
	}
	// And so does any not-yet-existing path beneath it (upload dest,
	// mkdir target, …) — the previous implementation let these through.
	for _, p := range []string{"/sub/newfile.txt", "/sub/nested/deep.txt"} {
		if _, err := ResolveSafePath(root, p); err == nil {
			t.Errorf("ResolveSafePath(%q) under symlinked parent should be rejected", p)
		}
	}
	// A symlinked parent pointing INSIDE root stays usable.
	inner := filepath.Join(root, "inner")
	if err := os.Mkdir(inner, 0755); err != nil {
		t.Fatal(err)
	}
	okLink := filepath.Join(root, "alias")
	if err := os.Symlink(inner, okLink); err != nil {
		t.Skip("cannot create symlink:", err)
	}
	if _, err := ResolveSafePath(root, "/alias/newfile.txt"); err != nil {
		t.Errorf("in-root symlink should stay usable: %v", err)
	}
}

func TestCopy_SkipsSymlinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("top-secret"), 0644); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "srcdir")
	if err := os.Mkdir(src, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "ok.txt"), []byte("fine"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(src, "evil-link")); err != nil {
		t.Skip("cannot create symlink:", err)
	}

	svc := NewFileService(root, 10<<20)
	if err := svc.Copy(context.Background(), "/srcdir", "/dst"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "dst", "evil-link")); !os.IsNotExist(err) {
		t.Error("symlink must not be reproduced in the copy destination")
	}
	data, err := os.ReadFile(filepath.Join(root, "dst", "ok.txt"))
	if err != nil || string(data) != "fine" {
		t.Errorf("regular file copy broke: %v %q", err, data)
	}
}

// --- upload temp-file hygiene ------------------------------------------------

func TestSaveUpload_LeavesNoTempBehind(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	if _, err := svc.SaveUpload(context.Background(), "/", "clean.txt", strings.NewReader("data"), 4); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %q", e.Name())
		}
	}
}

func newSvcForSecurity(t *testing.T) (*FileService, string) {
	t.Helper()
	root := t.TempDir()
	return NewFileService(root, 10<<20), root
}
