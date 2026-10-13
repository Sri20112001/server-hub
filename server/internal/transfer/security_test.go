package transfer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
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
	sess := sessID("cap")
	if err := svc.SaveChunk(sess, 0, strings.NewReader("0123456789ABCDEF")); err == nil {
		t.Fatal("oversize chunk should have been rejected")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrFileTooLarge {
			t.Fatalf("want FILE_TOO_LARGE, got %v", err)
		}
	}
	// Rejected chunk must not linger on disk.
	entries, _ := os.ReadDir(filepath.Join(chunkBaseDir(), sess))
	if len(entries) != 0 {
		t.Errorf("rejected chunk left %d files behind", len(entries))
	}
}

func TestFinalizeChunk_EnforcesTotalCap(t *testing.T) {
	root := t.TempDir()
	svc := NewFileService(root, 10) // 10-byte budget
	sess := sessID("totalcap")
	// Each chunk fits the per-chunk cap; the assembled total (16) does not.
	if err := svc.SaveChunk(sess, 0, strings.NewReader("12345678")); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveChunk(sess, 1, strings.NewReader("12345678")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FinalizeChunk(context.Background(), sess, "/", "big.bin"); err == nil {
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
	sess := sessID("roundtrip")
	if err := svc.SaveChunk(sess, 0, strings.NewReader("hello-")); err != nil {
		t.Fatal(err)
	}
	if err := svc.SaveChunk(sess, 1, strings.NewReader("chunked")); err != nil {
		t.Fatal(err)
	}
	p, err := svc.FinalizeChunk(context.Background(), sess, "/", "out.txt")
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
	if _, err := os.Stat(filepath.Join(chunkBaseDir(), sess)); !os.IsNotExist(err) {
		t.Error("chunk dir should be removed after finalize")
	}
}

func TestFinalizeChunk_RejectsTraversalAndTrash(t *testing.T) {
	svc, _ := newSvcForSecurity(t)
	sess := sessID("finguard")
	if err := svc.SaveChunk(sess, 0, strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.FinalizeChunk(context.Background(), "../../evil", "/", "x.txt"); err == nil {
		t.Error("traversal upload_id should be rejected")
	}
	if _, err := svc.FinalizeChunk(context.Background(), sess, "/.trash", "x.txt"); err == nil {
		t.Error("finalize into trash should be rejected")
	}
	if _, err := svc.FinalizeChunk(context.Background(), sess, "/", ".trash"); err == nil {
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

// --- atomic non-overwriting finalization -----------------------------------
// These tests pin the check-then-act fix: on Unix, os.Rename silently
// replaces an existing destination, so a naive stat-then-rename lets a
// racing operation's file be destroyed. Exactly one contender must win
// with a complete file; every loser must see FILE_EXISTS; pre-existing
// destinations must survive byte-for-byte.

func TestSaveUpload_ConcurrentSameDestination(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	const racers = 8
	payloads := make([]string, racers)
	for i := range payloads {
		payloads[i] = "payload-" + strings.Repeat(string(rune('A'+i)), 256)
	}

	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, racers)
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			_, errs[i] = svc.SaveUpload(context.Background(), "/", "race.txt",
				strings.NewReader(payloads[i]), int64(len(payloads[i])))
		}(i)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrFileExists {
			t.Fatalf("loser must see FILE_EXISTS, got %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one upload must win, got %d", wins)
	}
	got, err := os.ReadFile(filepath.Join(root, "race.txt"))
	if err != nil {
		t.Fatal(err)
	}
	complete := false
	for _, p := range payloads {
		if string(got) == p {
			complete = true
		}
	}
	if !complete {
		t.Error("winner file is truncated or interleaved, not one complete payload")
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %q", e.Name())
		}
	}
}

func TestSaveUpload_ExistingDestinationKept(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	if err := os.WriteFile(filepath.Join(root, "keep.txt"), []byte("original-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := svc.SaveUpload(context.Background(), "/", "keep.txt",
		strings.NewReader("attacker-bytes"), int64(len("attacker-bytes")))
	if err == nil {
		t.Fatal("upload over an existing file must fail")
	}
	var appErr *AppError
	if !errors.As(err, &appErr) || appErr.Code != ErrFileExists {
		t.Fatalf("want FILE_EXISTS, got %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "keep.txt"))
	if string(got) != "original-bytes" {
		t.Errorf("pre-existing file was clobbered: %q", got)
	}
}

func TestRename_ConcurrentSameTarget(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("content-A"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("content-B"), 0644); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, src := range []string{"/a.txt", "/b.txt"} {
		wg.Add(1)
		go func(i int, src string) {
			defer wg.Done()
			<-start
			errs[i] = svc.Rename(src, "c.txt")
		}(i, src)
	}
	close(start)
	wg.Wait()

	wins := 0
	for _, err := range errs {
		if err == nil {
			wins++
			continue
		}
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrFileExists {
			t.Fatalf("loser must see FILE_EXISTS, got %v", err)
		}
	}
	if wins != 1 {
		t.Fatalf("exactly one rename must win, got %d", wins)
	}
	got, err := os.ReadFile(filepath.Join(root, "c.txt"))
	if err != nil || (string(got) != "content-A" && string(got) != "content-B") {
		t.Errorf("winner file incomplete or missing: %v %q", err, got)
	}
}

func TestRename_Move_DirectoryStillWorks(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	if err := svc.CreateDirectory("/srcdir"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "srcdir", "inner.txt"), []byte("in"), 0644); err != nil {
		t.Fatal(err)
	}
	// Directory rename/move must keep working through the no-replace path.
	if err := svc.Rename("/srcdir", "destdir"); err != nil {
		t.Fatalf("dir rename: %v", err)
	}
	if err := svc.Move("/destdir", "/moved"); err != nil {
		t.Fatalf("dir move: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "moved", "inner.txt")); err != nil {
		t.Errorf("moved tree incomplete: %v", err)
	}
	// ...and still refuse to overwrite an existing directory.
	if err := svc.CreateDirectory("/other"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Rename("/moved", "other"); err == nil {
		t.Error("rename onto an existing directory must fail")
	}
}

func TestMove_ExistingDestinationKept(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	if err := os.WriteFile(filepath.Join(root, "victim.txt"), []byte("victim-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("src-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Move("/src.txt", "/victim.txt"); err == nil {
		t.Fatal("move onto an existing file must fail")
	}
	got, _ := os.ReadFile(filepath.Join(root, "victim.txt"))
	if string(got) != "victim-bytes" {
		t.Errorf("destination was clobbered: %q", got)
	}
	if _, err := os.Stat(filepath.Join(root, "src.txt")); err != nil {
		t.Error("source must survive a refused move")
	}
}

func TestCopy_ExistingDestinationKept(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	if err := os.WriteFile(filepath.Join(root, "dst.txt"), []byte("dst-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src.txt"), []byte("src-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.Copy(context.Background(), "/src.txt", "/dst.txt"); err == nil {
		t.Fatal("copy onto an existing file must fail")
	}
	got, _ := os.ReadFile(filepath.Join(root, "dst.txt"))
	if string(got) != "dst-bytes" {
		t.Errorf("destination was clobbered: %q", got)
	}
}

func TestTrashRestore_ExistingDestinationKept(t *testing.T) {
	svc, root := newSvcForSecurity(t)
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("v1"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.TrashMove("/f.txt"); err != nil {
		t.Fatal(err)
	}
	items, err := svc.TrashList()
	if err != nil || len(items) != 1 {
		t.Fatalf("trash list = %+v, %v", items, err)
	}
	// Someone recreates the path before the restore lands.
	if err := os.WriteFile(filepath.Join(root, "f.txt"), []byte("v2"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := svc.TrashRestore(items[0].ID); err == nil {
		t.Fatal("restore onto an existing file must fail")
	}
	got, _ := os.ReadFile(filepath.Join(root, "f.txt"))
	if string(got) != "v2" {
		t.Errorf("recreated file was clobbered: %q", got)
	}
	// The trash entry (and its metadata) must survive the refused restore
	// so the user can restore elsewhere after clearing the path.
	if items, err := svc.TrashList(); err != nil || len(items) != 1 {
		t.Errorf("trash entry must survive a refused restore: %+v, %v", items, err)
	}
}

func TestTrashRestore_RejectsCraftedID(t *testing.T) {
	svc, _ := newSvcForSecurity(t)
	for _, id := range []string{"../x", "..", "/abs", "12/34", "abc", "", "1.meta", ".", "1 "} {
		err := svc.TrashRestore(id)
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrInvalidRequest {
			t.Errorf("TrashRestore(%q) must be rejected as invalid, got %v", id, err)
		}
	}
	// A well-formed but unknown ID is a lookup miss, not a validation hit.
	if err := svc.TrashRestore("123456789"); err == nil {
		t.Fatal("unknown trash id must fail")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrFileNotFound {
			t.Errorf("unknown trash id must be FILE_NOT_FOUND, got %v", err)
		}
	}
}

func TestSaveChunk_DuplicateIndexRejected(t *testing.T) {
	svc, _ := newSvcForSecurity(t)
	sess := sessID("dupidx")
	if err := svc.SaveChunk(sess, 0, strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	// A second write to the same slot must fail loudly instead of
	// silently interleaving bytes into the winner's chunk file.
	if err := svc.SaveChunk(sess, 0, strings.NewReader("second")); err == nil {
		t.Fatal("duplicate chunk index must fail")
	} else {
		var appErr *AppError
		if !errors.As(err, &appErr) || appErr.Code != ErrFileExists {
			t.Fatalf("want FILE_EXISTS, got %v", err)
		}
	}
	p, err := svc.FinalizeChunk(context.Background(), sess, "/", "dup.txt")
	if err != nil || p != "/dup.txt" {
		t.Fatalf("finalize after refused duplicate: %v %q", err, p)
	}
}

func newSvcForSecurity(t *testing.T) (*FileService, string) {
	t.Helper()
	root := t.TempDir()
	return NewFileService(root, 10<<20), root
}

// sessID hands out globally-unique chunk session IDs: chunk state lives in
// a shared OS temp dir (not the per-test storage root), so fixed IDs would
// collide with leftovers from earlier runs and turn O_EXCL's protection
// into false failures.
func sessID(tag string) string {
	return fmt.Sprintf("t%d-%d-%s", os.Getpid(), time.Now().UnixNano(), tag)
}
