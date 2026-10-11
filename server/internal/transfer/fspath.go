package transfer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ResolveSafePath resolves requestedPath relative to root and ensures the
// result stays inside root. It rejects path traversal and symlinks that
// escape root.
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
	if !strings.HasPrefix(abs, rootClean+string(filepath.Separator)) && abs != rootClean {
		return "", NewError(ErrPathTraversal, "path escapes storage root")
	}

	// Resolve symlinks and re-check.
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Path doesn't exist yet (e.g. upload destination) – return the
			// unresolved path after the prefix check already passed.
			return abs, nil
		}
		return "", NewError(ErrInvalidPath, "cannot resolve path")
	}
	if !strings.HasPrefix(resolved, rootClean+string(filepath.Separator)) && resolved != rootClean {
		return "", NewError(ErrPathTraversal, "symlink escapes storage root")
	}
	return resolved, nil
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

// CopyFile streams src to dst without loading the whole file into memory.
// It respects ctx cancellation.
func CopyFile(ctx context.Context, src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
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
