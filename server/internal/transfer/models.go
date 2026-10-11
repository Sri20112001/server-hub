package transfer

import "time"

// File-transfer API shapes. JSON contracts intentionally match the
// homelabs-upload file service so existing clients and migrated tooling
// keep working against the merged endpoints.

type ItemType string

const (
	ItemTypeFile      ItemType = "file"
	ItemTypeDirectory ItemType = "directory"
)

// FileItem represents a file or directory entry returned to the client.
type FileItem struct {
	Name       string    `json:"name"`
	Path       string    `json:"path"`
	Type       ItemType  `json:"type"`
	Size       int64     `json:"size"`
	Extension  string    `json:"extension,omitempty"`
	MimeType   string    `json:"mime_type,omitempty"`
	ModifiedAt time.Time `json:"modified_at"`
}

// ListResponse is the response for directory listing.
type ListResponse struct {
	Path  string     `json:"path"`
	Items []FileItem `json:"items"`
}

// StorageInfo holds disk usage statistics.
type StorageInfo struct {
	StorageRoot    string `json:"storage_root"`
	TotalBytes     uint64 `json:"total_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
}

// TrashItem is a file/dir that has been soft-deleted.
type TrashItem struct {
	ID           string    `json:"id"`
	OriginalPath string    `json:"original_path"`
	Name         string    `json:"name"`
	IsDir        bool      `json:"is_dir"`
	Size         int64     `json:"size"`
	DeletedAt    time.Time `json:"deleted_at"`
}

// FolderSizeResult is the response for an async folder size calculation.
type FolderSizeResult struct {
	Path      string `json:"path"`
	SizeBytes int64  `json:"size_bytes"`
	FileCount int    `json:"file_count"`
}

// FolderStat is one row of the storage breakdown (per top-level directory).
type FolderStat struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	FileCount int    `json:"files"`
}

// StorageBreakdownResult is the response for storage breakdown.
type StorageBreakdownResult struct {
	Folders []FolderStat `json:"folders"`
}

// ErrorCode is a machine-readable error identifier.
type ErrorCode string

const (
	ErrInvalidPath      ErrorCode = "INVALID_PATH"
	ErrPathTraversal    ErrorCode = "PATH_TRAVERSAL"
	ErrFileNotFound     ErrorCode = "FILE_NOT_FOUND"
	ErrDirNotFound      ErrorCode = "DIRECTORY_NOT_FOUND"
	ErrFileExists       ErrorCode = "FILE_EXISTS"
	ErrDirExists        ErrorCode = "DIRECTORY_EXISTS"
	ErrPermissionDenied ErrorCode = "PERMISSION_DENIED"
	ErrInvalidFileName  ErrorCode = "INVALID_FILE_NAME"
	ErrFileTooLarge     ErrorCode = "FILE_TOO_LARGE"
	ErrInvalidRequest   ErrorCode = "INVALID_REQUEST"
	ErrInternal         ErrorCode = "INTERNAL_ERROR"
)

// AppError is a structured application error.
type AppError struct {
	Code    ErrorCode `json:"code"`
	Message string    `json:"message"`
}

func (e *AppError) Error() string { return string(e.Code) + ": " + e.Message }

func NewError(code ErrorCode, message string) *AppError {
	return &AppError{Code: code, Message: message}
}
