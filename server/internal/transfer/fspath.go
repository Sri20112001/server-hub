package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// ResolveSafePath resolves requestedPath relative to root and ensures the
// result stays inside root. It rejects path traversal and symlinks that
// escape root — including symlinks in parent directories of paths that do
// not exist yet (e.g. an upload destination under a symlinked folder).
func ResolveSafePath(root, requestedPath string) (string, error) {
	rootClean := filepath.Clean(root)

	// Reject any path that contains a raw ".." segment before cleaning,
	// covering both slash variants and URL-encoded forms that the HTTP layer
	// may have already decoded.
	normalized := filepath.ToSlash(requestedPath)
	for _, seg := range strings.Split(normalized, "/") {
		if seg == ".." {
			return "", NewError(ErrPathTraversal, "path escapes storage root")
		}
	}

	// Clean the requested path to remove any redundant . components.
	cleaned := filepath.Clean("/" + requestedPath)
	abs := filepath.Join(rootClean, cleaned)

	// Belt-and-suspenders: ensure the joined path is still inside root.
	if !withinRoot(rootClean, abs) {
		return "", NewError(ErrPathTraversal, "path escapes storage root")
	}

	// Resolve symlinks and re-check. Non-existent trailing components
	// cannot be symlinks themselves, but every existing ancestor is
	// resolved: otherwise a symlinked parent directory would let a
	// not-yet-existing path escape root undetected.
	if err := assertResolvedWithinRoot(rootClean, abs); err != nil {
		return "", err
	}
	return abs, nil
}

// withinRoot reports whether abs is root itself or nested inside it.
func withinRoot(rootClean, abs string) bool {
	return abs == rootClean || strings.HasPrefix(abs, rootClean+string(filepath.Separator))
}

// assertResolvedWithinRoot resolves the nearest existing ancestor of abs
// (plus root itself) and verifies the resolved location stays inside root.
// The unresolved abs is still returned to callers: filesystem operations
// through a permitted symlink land inside the real root, while
// ToClientPath keeps working on the client-visible spelling.
func assertResolvedWithinRoot(rootClean, abs string) error {
	cur := abs
	var suffix []string
	for {
		if fi, err := os.Lstat(cur); err == nil {
			_ = fi
			break
		} else if !errors.Is(err, fs.ErrNotExist) {
			return NewError(ErrInvalidPath, "cannot resolve path")
		} else {
			parent := filepath.Dir(cur)
			if parent == cur {
				return NewError(ErrInvalidPath, "cannot resolve path")
			}
			suffix = append([]string{filepath.Base(cur)}, suffix...)
			cur = parent
		}
	}
	resolved, err := filepath.EvalSymlinks(cur)
	if err != nil {
		return NewError(ErrInvalidPath, "cannot resolve path")
	}
	full := resolved
	for _, s := range suffix {
		full = filepath.Join(full, s)
	}
	// Compare fully-resolved forms: resolve root the same way so bind
	// mounts or /tmp symlinks (e.g. macOS /var → /private/var) cannot
	// cause false rejections or false acceptances.
	rootReal, err := filepath.EvalSymlinks(rootClean)
	if err != nil {
		return NewError(ErrInvalidPath, "cannot resolve path")
	}
	if !withinRoot(rootReal, full) {
		if len(suffix) > 0 {
			return NewError(ErrPathTraversal, "path escapes storage root")
		}
		return NewError(ErrPathTraversal, "symlink escapes storage root")
	}
	return nil
}

// ToClientPath converts an absolute filesystem path back to a client-visible
// relative path (always starting with /).
func ToClientPath(root, absPath string) string {
	rel, err := filepath.Rel(root, absPath)
	if err != nil {
		return "/"
	}
	slashed := filepath.ToSlash(rel)
	if slashed == "." {
		return "/"
	}
	return "/" + slashed
}

// renameNoReplace moves src to dst without ever overwriting an existing
// destination. For files it links dst to src first: the link syscall fails
// atomically with EEXIST when dst exists (POSIX and Windows/NTFS alike),
// so dst appears complete in one step and a racing creator can never have
// its file silently replaced. src is removed only after a successful link.
// Directories cannot be hard-linked: they fall back to os.Rename, which
// callers invoke with the destination pre-checked under the service op
// mutex (same-process writers serialized; Windows rename additionally
// refuses existing destinations natively). A cross-device or no-hardlink
// filesystem (bind mounts across devices, FAT/exFAT) takes the same rename
// fallback instead of failing outright.
func renameNoReplace(src, dst string) error {
	if fi, err := os.Stat(src); err == nil && fi.IsDir() {
		return renameFallback(src, dst)
	}
	if err := os.Link(src, dst); err == nil {
		return os.Remove(src)
	} else if os.IsExist(err) {
		return NewError(ErrFileExists, "destination already exists")
	}
	// Any other link failure (cross-device bind mounts, FAT/exFAT, other
	// filesystems without hard links): fall back to rename, which callers
	// invoke pre-checked under the service op mutex. A genuinely broken
	// filesystem surfaces its error from the rename itself.
	return renameFallback(src, dst)
}

func renameFallback(src, dst string) error {
	if err := os.Rename(src, dst); err != nil {
		if os.IsExist(err) {
			return NewError(ErrFileExists, "destination already exists")
		}
		return err
	}
	return nil
}

// CopyFile streams src to dst without loading the whole file into memory.
// It respects ctx cancellation. O_EXCL guarantees a concurrent create of
// the same destination fails instead of interleaving two copies.
func CopyFile(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		if os.IsExist(err) {
			return NewError(ErrFileExists, "destination already exists")
		}
		return err
	}

	_, err = io.Copy(out, readerWithContext(ctx, in))
	if closeErr := out.Close(); closeErr != nil && err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(dst)
	}
	return err
}

// CopyDir recursively copies src directory to dst.
func CopyDir(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		// Never follow or reproduce symlinks: a planted link could
		// otherwise pull outside files into the copy (read-outside) or
		// recreate an escape in the destination tree.
		if d.Type()&fs.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return CopyFile(ctx, path, target)
	})
}

// DiskUsage returns total, used, and available bytes for the filesystem
// containing path. Implemented per-OS in disk_*.go files.
func DiskUsage(path string) (total, used, available uint64, err error) {
	return diskUsage(path)
}

// TrashDirName is the reserved trash directory inside the storage root.
// It is hidden from the file browser (list/search/metadata/download) and
// managed only through the dedicated trash endpoints: without this,
// "deleted" files would stay readable and downloadable by any viewer
// until the trash is purged.
const TrashDirName = ".trash"

// isTrashPath reports whether a client-visible path addresses the trash
// area itself. Client paths use slash spelling on every OS, so the
// slash-only path package (not filepath) normalizes here: doubling the
// leading slash must collapse, otherwise Windows keeps a UNC-style `\\`
// prefix and the comparison below misses.
func isTrashPath(clientPath string) bool {
	p := filepath.ToSlash(clientPath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	c := path.Clean(p)
	return c == "/"+TrashDirName || strings.HasPrefix(c, "/"+TrashDirName+"/")
}

// rejectTrash refuses file operations that target the trash area outside
// the dedicated trash endpoints. Trash payloads are addressed by opaque
// IDs through those endpoints instead.
func rejectTrash(clientPath string) error {
	if isTrashPath(clientPath) {
		return NewError(ErrInvalidPath, "path is inside the trash area")
	}
	return nil
}

// sanitizeHeaderName strips characters that would malform a
// Content-Disposition header value (quotes, backslashes, control codes).
// Go's http stack rejects such values with a failed response; sanitizing
// keeps downloads working for odd-but-legal uploaded filenames.
func sanitizeHeaderName(name string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || r == '"' || r == '\\' {
			return -1
		}
		return r
	}, name)
}

// ValidateFileName ensures a name component contains no path separators.
func ValidateFileName(name string) error {
	if name == "" || strings.ContainsAny(name, `/\`) || name == "." || name == ".." {
		return NewError(ErrInvalidFileName, fmt.Sprintf("invalid file name: %q", name))
	}
	return nil
}

// contextReader wraps an io.Reader and aborts on context cancellation.
type contextReader struct {
	ctx context.Context
	r   io.Reader
}

func readerWithContext(ctx context.Context, r io.Reader) io.Reader {
	return &contextReader{ctx: ctx, r: r}
}

func (cr *contextReader) Read(p []byte) (int, error) {
	if err := cr.ctx.Err(); err != nil {
		return 0, err
	}
	return cr.r.Read(p)
}
