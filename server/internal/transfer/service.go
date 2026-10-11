package transfer

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// FileService contains all business logic for file operations.
// It is storage-root confined: every client path resolves through
// ResolveSafePath, and traversal/symlink escapes are rejected.
type FileService struct {
	root          string
	maxUploadSize int64
	chunkMu       sync.Mutex
	chunks        map[string][]string // uploadID → ordered tmp chunk paths
}

func NewFileService(root string, maxUploadSize int64) *FileService {
	return &FileService{
		root:          root,
		maxUploadSize: maxUploadSize,
		chunks:        make(map[string][]string),
	}
}

// ListDirectory returns the contents of the directory at clientPath.
func (s *FileService) ListDirectory(clientPath string) (*ListResponse, error) {
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, NewError(ErrDirNotFound, "directory not found")
		}
		return nil, NewError(ErrInternal, "cannot read directory")
	}

	items := make([]FileItem, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		item := entryToItem(s.root, abs, info, e.IsDir())
		items = append(items, item)
	}

	// Directories first, then alphabetical within each group.
	sort.Slice(items, func(i, j int) bool {
		if items[i].Type != items[j].Type {
			return items[i].Type == ItemTypeDirectory
		}
		return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
	})

	return &ListResponse{
		Path:  ToClientPath(s.root, abs),
		Items: items,
	}, nil
}

// GetMetadata returns metadata for a single file or directory.
func (s *FileService) GetMetadata(clientPath string) (*FileItem, error) {
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, NewError(ErrFileNotFound, "file not found")
		}
		return nil, NewError(ErrInternal, "cannot stat path")
	}
	item := entryToItem(s.root, filepath.Dir(abs), info, info.IsDir())
	if !info.IsDir() {
		item.MimeType = detectMIME(abs)
	}
	return &item, nil
}

// OpenForDownload returns an *os.File ready for streaming to the client.
func (s *FileService) OpenForDownload(clientPath string) (*os.File, os.FileInfo, error) {
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return nil, nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, NewError(ErrFileNotFound, "file not found")
		}
		return nil, nil, NewError(ErrInternal, "cannot stat file")
	}
	if info.IsDir() {
		return nil, nil, NewError(ErrInvalidPath, "path is a directory")
	}
	f, err := os.Open(abs)
	if err != nil {
		return nil, nil, NewError(ErrInternal, "cannot open file")
	}
	return f, info, nil
}

// SaveUpload streams the multipart file to disk.
func (s *FileService) SaveUpload(ctx context.Context, destDir, filename string, src io.Reader, size int64) (string, error) {
	if size > s.maxUploadSize {
		return "", NewError(ErrFileTooLarge, "file exceeds maximum upload size")
	}
	if err := ValidateFileName(filename); err != nil {
		return "", err
	}

	absDir, err := ResolveSafePath(s.root, destDir)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(absDir); err != nil || !info.IsDir() {
		return "", NewError(ErrDirNotFound, "destination directory not found")
	}

	destAbs := filepath.Join(absDir, filename)
	if _, err := os.Stat(destAbs); err == nil {
		return "", NewError(ErrFileExists, "file already exists")
	}

	tmp := destAbs + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", NewError(ErrInternal, "cannot create file")
	}

	limited := io.LimitReader(src, s.maxUploadSize+1)
	written, err := io.Copy(f, readerWithContext(ctx, limited))
	f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return "", NewError(ErrInternal, "upload failed")
	}
	if written > s.maxUploadSize {
		_ = os.Remove(tmp)
		return "", NewError(ErrFileTooLarge, "file exceeds maximum upload size")
	}

	if err := os.Rename(tmp, destAbs); err != nil {
		_ = os.Remove(tmp)
		return "", NewError(ErrInternal, "cannot finalise upload")
	}
	return ToClientPath(s.root, destAbs), nil
}

// CreateDirectory creates a new directory.
func (s *FileService) CreateDirectory(clientPath string) error {
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err == nil {
		return NewError(ErrDirExists, "directory already exists")
	}
	if err := os.Mkdir(abs, 0755); err != nil {
		return NewError(ErrInternal, "cannot create directory")
	}
	return nil
}

// Rename renames a file or directory within the same parent.
func (s *FileService) Rename(clientPath, newName string) error {
	if err := ValidateFileName(newName); err != nil {
		return err
	}
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(abs); err != nil {
		return NewError(ErrFileNotFound, "source not found")
	}
	newAbs := filepath.Join(filepath.Dir(abs), newName)
	if _, err := os.Stat(newAbs); err == nil {
		return NewError(ErrFileExists, "destination already exists")
	}
	if err := os.Rename(abs, newAbs); err != nil {
		return NewError(ErrInternal, "rename failed")
	}
	return nil
}

// Move moves src to dst.
func (s *FileService) Move(srcPath, dstPath string) error {
	srcAbs, err := ResolveSafePath(s.root, srcPath)
	if err != nil {
		return err
	}
	dstAbs, err := ResolveSafePath(s.root, dstPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(srcAbs); err != nil {
		return NewError(ErrFileNotFound, "source not found")
	}
	if _, err := os.Stat(dstAbs); err == nil {
		return NewError(ErrFileExists, "destination already exists")
	}
	// Prevent moving a directory into itself.
	if strings.HasPrefix(dstAbs+string(filepath.Separator), srcAbs+string(filepath.Separator)) {
		return NewError(ErrInvalidPath, "cannot move directory into itself")
	}
	if err := os.Rename(srcAbs, dstAbs); err != nil {
		return NewError(ErrInternal, "move failed")
	}
	return nil
}

// Copy copies src to dst (files or directories).
func (s *FileService) Copy(ctx context.Context, srcPath, dstPath string) error {
	srcAbs, err := ResolveSafePath(s.root, srcPath)
	if err != nil {
		return err
	}
	dstAbs, err := ResolveSafePath(s.root, dstPath)
	if err != nil {
		return err
	}
	info, err := os.Stat(srcAbs)
	if err != nil {
		return NewError(ErrFileNotFound, "source not found")
	}
	if _, err := os.Stat(dstAbs); err == nil {
		return NewError(ErrFileExists, "destination already exists")
	}
	if info.IsDir() {
		return CopyDir(ctx, srcAbs, dstAbs)
	}
	return CopyFile(ctx, srcAbs, dstAbs)
}

// TrashDir returns the path to the .trash directory inside the storage root.
func (s *FileService) trashDir() string { return filepath.Join(s.root, ".trash") }

// TrashMove moves a file/dir to .trash instead of deleting it.
func (s *FileService) TrashMove(clientPath string) error {
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return err
	}
	if abs == filepath.Clean(s.root) {
		return NewError(ErrInvalidPath, "cannot trash storage root")
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NewError(ErrFileNotFound, "path not found")
		}
		return NewError(ErrInternal, "cannot stat path")
	}

	trash := s.trashDir()
	if err := os.MkdirAll(trash, 0755); err != nil {
		return NewError(ErrInternal, "cannot create trash directory")
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	meta := TrashItem{
		ID:           id,
		OriginalPath: ToClientPath(s.root, abs),
		Name:         info.Name(),
		IsDir:        info.IsDir(),
		Size:         info.Size(),
		DeletedAt:    time.Now().UTC(),
	}

	// Write metadata sidecar
	metaBytes, _ := json.Marshal(meta)
	if err := os.WriteFile(filepath.Join(trash, id+".meta"), metaBytes, 0644); err != nil {
		return NewError(ErrInternal, "cannot write trash metadata")
	}

	if err := os.Rename(abs, filepath.Join(trash, id)); err != nil {
		_ = os.Remove(filepath.Join(trash, id+".meta"))
		return NewError(ErrInternal, "trash move failed")
	}
	return nil
}

// TrashList returns all items currently in the trash.
func (s *FileService) TrashList() ([]TrashItem, error) {
	trash := s.trashDir()
	entries, err := os.ReadDir(trash)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []TrashItem{}, nil
		}
		return nil, NewError(ErrInternal, "cannot read trash")
	}
	var items []TrashItem
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".meta") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(trash, e.Name()))
		if err != nil {
			continue
		}
		var item TrashItem
		if json.Unmarshal(data, &item) == nil {
			items = append(items, item)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].DeletedAt.After(items[j].DeletedAt)
	})
	return items, nil
}

// TrashRestore moves an item from trash back to its original location.
func (s *FileService) TrashRestore(id string) error {
	trash := s.trashDir()
	metaPath := filepath.Join(trash, id+".meta")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return NewError(ErrFileNotFound, "trash item not found")
	}
	var item TrashItem
	if err := json.Unmarshal(data, &item); err != nil {
		return NewError(ErrInternal, "corrupt trash metadata")
	}
	destAbs, err := ResolveSafePath(s.root, item.OriginalPath)
	if err != nil {
		return err
	}
	if _, err := os.Stat(destAbs); err == nil {
		return NewError(ErrFileExists, "destination already exists")
	}
	if err := os.MkdirAll(filepath.Dir(destAbs), 0755); err != nil {
		return NewError(ErrInternal, "cannot create parent directory")
	}
	if err := os.Rename(filepath.Join(trash, id), destAbs); err != nil {
		return NewError(ErrInternal, "restore failed")
	}
	_ = os.Remove(metaPath)
	return nil
}

// TrashPurge permanently deletes all items in the trash.
func (s *FileService) TrashPurge() error {
	trash := s.trashDir()
	if err := os.RemoveAll(trash); err != nil {
		return NewError(ErrInternal, "purge failed")
	}
	return nil
}

// FolderSize calculates the total size of a directory tree.
func (s *FileService) FolderSize(clientPath string) (*FolderSizeResult, error) {
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return nil, err
	}
	total, count := walkSize(abs)
	return &FolderSizeResult{
		Path:      ToClientPath(s.root, abs),
		SizeBytes: total,
		FileCount: count,
	}, nil
}

// StorageBreakdown returns per-top-level-directory sizes in one call.
func (s *FileService) StorageBreakdown() (*StorageBreakdownResult, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, NewError(ErrInternal, "cannot read directory")
	}
	folders := make([]FolderStat, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		abs := filepath.Join(s.root, e.Name())
		total, count := walkSize(abs)
		folders = append(folders, FolderStat{
			Name:      e.Name(),
			Path:      ToClientPath(s.root, abs),
			Bytes:     total,
			FileCount: count,
		})
	}
	// Biggest first — matches the dashboard's display order.
	sort.Slice(folders, func(i, j int) bool { return folders[i].Bytes > folders[j].Bytes })
	return &StorageBreakdownResult{Folders: folders}, nil
}

// walkSize sums file bytes and counts files under abs (no symlink following).
func walkSize(abs string) (int64, int) {
	var total int64
	var count int
	_ = filepath.WalkDir(abs, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if info, e := d.Info(); e == nil {
			total += info.Size()
			count++
		}
		return nil
	})
	return total, count
}

// ZipStream writes a zip archive of the given client paths to w.
func (s *FileService) ZipStream(ctx context.Context, clientPaths []string, w io.Writer) error {
	zw := zip.NewWriter(w)
	defer zw.Close()

	for _, cp := range clientPaths {
		abs, err := ResolveSafePath(s.root, cp)
		if err != nil {
			continue
		}
		info, err := os.Stat(abs)
		if err != nil {
			continue
		}
		if info.IsDir() {
			if err := zipDir(ctx, zw, abs, info.Name()); err != nil {
				return err
			}
		} else {
			if err := zipFile(ctx, zw, abs, info.Name()); err != nil {
				return err
			}
		}
	}
	return nil
}

func zipDir(ctx context.Context, zw *zip.Writer, dir, base string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || ctx.Err() != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		zipPath := filepath.ToSlash(filepath.Join(base, rel))
		if d.IsDir() {
			_, err = zw.Create(zipPath + "/")
			return err
		}
		return zipFile(ctx, zw, path, zipPath)
	})
}

func zipFile(ctx context.Context, zw *zip.Writer, src, name string) error {
	f, err := os.Open(src)
	if err != nil {
		return nil // skip unreadable
	}
	defer f.Close()
	w, err := zw.Create(name)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, readerWithContext(ctx, f))
	return err
}

// SaveChunk stores one chunk of a multi-part upload.
func (s *FileService) SaveChunk(uploadID string, chunkIndex int, src io.Reader) error {
	tmpDir := filepath.Join(os.TempDir(), "sh-chunks", uploadID)
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		return NewError(ErrInternal, "cannot create chunk dir")
	}
	chunkPath := filepath.Join(tmpDir, fmt.Sprintf("%05d", chunkIndex))
	f, err := os.Create(chunkPath)
	if err != nil {
		return NewError(ErrInternal, "cannot create chunk file")
	}
	defer f.Close()
	if _, err := io.Copy(f, src); err != nil {
		return NewError(ErrInternal, "chunk write failed")
	}
	s.chunkMu.Lock()
	s.chunks[uploadID] = append(s.chunks[uploadID], chunkPath)
	s.chunkMu.Unlock()
	return nil
}

// FinalizeChunk assembles all chunks into the final file.
func (s *FileService) FinalizeChunk(ctx context.Context, uploadID, destDir, filename string) (string, error) {
	if err := ValidateFileName(filename); err != nil {
		return "", err
	}
	absDir, err := ResolveSafePath(s.root, destDir)
	if err != nil {
		return "", err
	}
	destAbs := filepath.Join(absDir, filename)
	if _, err := os.Stat(destAbs); err == nil {
		return "", NewError(ErrFileExists, "file already exists")
	}

	tmpDir := filepath.Join(os.TempDir(), "sh-chunks", uploadID)
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		return "", NewError(ErrInternal, "chunk dir not found")
	}

	out, err := os.Create(destAbs)
	if err != nil {
		return "", NewError(ErrInternal, "cannot create output file")
	}
	for _, e := range entries {
		if ctx.Err() != nil {
			out.Close()
			_ = os.Remove(destAbs)
			return "", ctx.Err()
		}
		f, err := os.Open(filepath.Join(tmpDir, e.Name()))
		if err != nil {
			out.Close()
			_ = os.Remove(destAbs)
			return "", NewError(ErrInternal, "cannot read chunk")
		}
		_, copyErr := io.Copy(out, f)
		f.Close()
		if copyErr != nil {
			out.Close()
			_ = os.Remove(destAbs)
			return "", NewError(ErrInternal, "chunk assembly failed")
		}
	}
	out.Close()
	_ = os.RemoveAll(tmpDir)
	s.chunkMu.Lock()
	delete(s.chunks, uploadID)
	s.chunkMu.Unlock()
	return ToClientPath(s.root, destAbs), nil
}

// Delete removes a file or directory.
func (s *FileService) Delete(clientPath string, recursive bool) error {
	abs, err := ResolveSafePath(s.root, clientPath)
	if err != nil {
		return err
	}
	// Prevent deleting the storage root itself.
	if abs == filepath.Clean(s.root) {
		return NewError(ErrInvalidPath, "cannot delete storage root")
	}
	info, err := os.Stat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return NewError(ErrFileNotFound, "path not found")
		}
		return NewError(ErrInternal, "cannot stat path")
	}
	if info.IsDir() && !recursive {
		return NewError(ErrInvalidRequest, "set recursive=true to delete a directory")
	}
	if err := os.RemoveAll(abs); err != nil {
		return NewError(ErrInternal, "delete failed")
	}
	return nil
}

// Search walks the directory tree looking for entries whose names contain query.
// Results are capped at 200 to avoid unbounded responses on large trees.
func (s *FileService) Search(query, basePath string) ([]FileItem, error) {
	abs, err := ResolveSafePath(s.root, basePath)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(query)
	var results []FileItem
	const maxResults = 200

	err = filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if len(results) >= maxResults {
			return filepath.SkipAll
		}
		// Skip symlinks that escape root.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}
		if path == abs {
			return nil
		}
		if strings.Contains(strings.ToLower(d.Name()), lower) {
			info, err := d.Info()
			if err != nil {
				return nil
			}
			item := entryToItem(s.root, filepath.Dir(path), info, d.IsDir())
			results = append(results, item)
		}
		return nil
	})
	if err != nil {
		return nil, NewError(ErrInternal, "search failed")
	}
	return results, nil
}

// StorageInfo returns disk usage for the storage root.
func (s *FileService) StorageInfo() (*StorageInfo, error) {
	total, used, available, err := DiskUsage(s.root)
	if err != nil {
		return nil, NewError(ErrInternal, "cannot retrieve storage info")
	}
	return &StorageInfo{
		StorageRoot:    "/",
		TotalBytes:     total,
		UsedBytes:      used,
		AvailableBytes: available,
	}, nil
}

// ResolvePath exposes safe path resolution for use by handlers.
func (s *FileService) ResolvePath(clientPath string) (string, error) {
	return ResolveSafePath(s.root, clientPath)
}

// --- helpers ---

func entryToItem(root, dir string, info os.FileInfo, isDir bool) FileItem {
	absPath := filepath.Join(dir, info.Name())
	item := FileItem{
		Name:       info.Name(),
		Path:       ToClientPath(root, absPath),
		ModifiedAt: info.ModTime().UTC(),
	}
	if isDir {
		item.Type = ItemTypeDirectory
	} else {
		item.Type = ItemTypeFile
		item.Size = info.Size()
		item.Extension = filepath.Ext(info.Name())
	}
	return item
}

func detectMIME(path string) string {
	ext := filepath.Ext(path)
	if t := mime.TypeByExtension(ext); t != "" {
		return t
	}
	f, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer f.Close()
	buf := make([]byte, 512)
	n, _ := f.Read(buf)
	return http.DetectContentType(buf[:n])
}

// readerWithCtx wraps a reader to abort on context cancellation.
type readerWithCtxT struct {
	ctx context.Context
	r   io.Reader
}

func readerWithCtx(ctx context.Context, r io.Reader) io.Reader {
	return &readerWithCtxT{ctx: ctx, r: r}
}

func (r *readerWithCtxT) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}
