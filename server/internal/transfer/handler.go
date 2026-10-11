package transfer

import (
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
)

// Handler serves the file-transfer API. Authentication and role gating
// happen in middleware (AuthRequired + RequireRole); every method here
// assumes an authenticated caller.
type Handler struct {
	svc *FileService
	db  *database.DB
}

func NewHandler(svc *FileService, db *database.DB) *Handler {
	return &Handler{svc: svc, db: db}
}

func (h *Handler) actor(c *gin.Context) (username, role string) {
	username, role = middleware.CurrentUser(c)
	if username == "" {
		username = "unknown"
	}
	return username, role
}

// record mirrors a file operation into transfer history (nv_activity) and
// the central audit log. Best-effort: logging never fails the operation.
func (h *Handler) record(c *gin.Context, action, path, detail string, ok bool) {
	user, role := h.actor(c)
	result := "ok"
	status := http.StatusOK
	if !ok {
		result = "failed"
		status = http.StatusInternalServerError
	}
	LogTransfer(h.db, user, role, action, path, detail, c.ClientIP(), status)
	audit.Write(h.db, user, action, "file", path, result, detail)
}

// List  GET /files?path=
func (h *Handler) List(c *gin.Context) {
	path := c.DefaultQuery("path", "/")
	resp, err := h.svc.ListDirectory(path)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// Metadata  GET /files/metadata?path=
func (h *Handler) Metadata(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		respondError(c, NewError(ErrInvalidRequest, "path is required"))
		return
	}
	item, err := h.svc.GetMetadata(path)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, item)
}

// Download  GET /files/download?path=
func (h *Handler) Download(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		respondError(c, NewError(ErrInvalidRequest, "path is required"))
		return
	}
	f, info, err := h.svc.OpenForDownload(path)
	if err != nil {
		respondError(c, err)
		return
	}
	defer f.Close()

	mimeType := detectMIMEFromName(info.Name())
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, sanitizeHeaderName(info.Name())))
	c.Header("Content-Length", strconv.FormatInt(info.Size(), 10))
	h.record(c, ActionDownload, path, "", true)
	c.DataFromReader(http.StatusOK, info.Size(), mimeType, f, nil)
}

// Upload  POST /files/upload?path=
func (h *Handler) Upload(c *gin.Context) {
	destDir := c.DefaultQuery("path", "/")

	fh, err := c.FormFile("file")
	if err != nil {
		respondError(c, NewError(ErrInvalidRequest, "file field is required"))
		return
	}

	src, err := fh.Open()
	if err != nil {
		respondError(c, NewError(ErrInternal, "cannot open uploaded file"))
		return
	}
	defer src.Close()

	clientPath, svcErr := h.svc.SaveUpload(c.Request.Context(), destDir, filepath.Base(fh.Filename), src, fh.Size)
	if svcErr != nil {
		respondError(c, svcErr)
		return
	}
	h.record(c, ActionUpload, clientPath, fmt.Sprintf("%s (%d bytes)", fh.Filename, fh.Size), true)
	c.JSON(http.StatusCreated, gin.H{"message": "file uploaded successfully", "path": clientPath})
}

// Mkdir  POST /directories
func (h *Handler) Mkdir(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "path is required"))
		return
	}
	if err := h.svc.CreateDirectory(req.Path); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionMkdir, req.Path, "", true)
	c.JSON(http.StatusCreated, gin.H{"message": "directory created", "path": req.Path})
}

// Rename  PATCH /files
func (h *Handler) Rename(c *gin.Context) {
	var req struct {
		Path    string `json:"path"     binding:"required"`
		NewName string `json:"new_name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "path and new_name are required"))
		return
	}
	if err := h.svc.Rename(req.Path, req.NewName); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionRename, req.Path, "→ "+req.NewName, true)
	c.JSON(http.StatusOK, gin.H{"message": "renamed successfully"})
}

// Move  POST /files/move
func (h *Handler) Move(c *gin.Context) {
	var req struct {
		Source      string `json:"source"      binding:"required"`
		Destination string `json:"destination" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "source and destination are required"))
		return
	}
	if err := h.svc.Move(req.Source, req.Destination); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionMove, req.Source, "→ "+req.Destination, true)
	c.JSON(http.StatusOK, gin.H{"message": "moved successfully"})
}

// Copy  POST /files/copy
func (h *Handler) Copy(c *gin.Context) {
	var req struct {
		Source      string `json:"source"      binding:"required"`
		Destination string `json:"destination" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "source and destination are required"))
		return
	}
	if err := h.svc.Copy(c.Request.Context(), req.Source, req.Destination); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionCopy, req.Source, "→ "+req.Destination, true)
	c.JSON(http.StatusOK, gin.H{"message": "copied successfully"})
}

// Delete  DELETE /files
func (h *Handler) Delete(c *gin.Context) {
	var req struct {
		Path      string `json:"path"      binding:"required"`
		Recursive bool   `json:"recursive"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "path is required"))
		return
	}
	if err := h.svc.Delete(req.Path, req.Recursive); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionDelete, req.Path, "", true)
	c.JSON(http.StatusOK, gin.H{"message": "deleted successfully"})
}

// Search  GET /search?q=&path=
func (h *Handler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		respondError(c, NewError(ErrInvalidRequest, "q is required"))
		return
	}
	basePath := c.DefaultQuery("path", "/")
	results, err := h.svc.Search(query, basePath)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"query": query, "results": results})
}

// Storage  GET /storage
func (h *Handler) Storage(c *gin.Context) {
	info, err := h.svc.StorageInfo()
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, info)
}

// StorageBreakdown  GET /storage/breakdown
func (h *Handler) StorageBreakdown(c *gin.Context) {
	res, err := h.svc.StorageBreakdown()
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, res)
}

// Zip  POST /files/zip
func (h *Handler) Zip(c *gin.Context) {
	var req struct {
		Paths []string `json:"paths" binding:"required"`
		Name  string   `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Paths) == 0 {
		respondError(c, NewError(ErrInvalidRequest, "paths array is required"))
		return
	}
	name := sanitizeHeaderName(req.Name)
	if name == "" {
		name = "download.zip"
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	c.Header("Content-Type", "application/zip")
	c.Header("Transfer-Encoding", "chunked")
	c.Status(http.StatusOK)
	h.record(c, ActionZipDownload, "", fmt.Sprintf("%d items → %s", len(req.Paths), name), true)
	if err := h.svc.ZipStream(c.Request.Context(), req.Paths, c.Writer); err != nil {
		// Headers already sent; nothing we can do
		_ = err
	}
}

// FolderSize  GET /files/size?path=
func (h *Handler) FolderSize(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		respondError(c, NewError(ErrInvalidRequest, "path is required"))
		return
	}
	result, err := h.svc.FolderSize(path)
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// UploadChunk  POST /files/chunk?upload_id=&index=
func (h *Handler) UploadChunk(c *gin.Context) {
	uploadID := c.Query("upload_id")
	indexStr := c.Query("index")
	if uploadID == "" || indexStr == "" {
		respondError(c, NewError(ErrInvalidRequest, "upload_id and index are required"))
		return
	}
	index, err := strconv.Atoi(indexStr)
	if err != nil {
		respondError(c, NewError(ErrInvalidRequest, "index must be an integer"))
		return
	}
	if err := h.svc.SaveChunk(uploadID, index, c.Request.Body); err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "chunk received"})
}

// FinalizeChunk  POST /files/chunk/finalize
func (h *Handler) FinalizeChunk(c *gin.Context) {
	var req struct {
		UploadID string `json:"upload_id" binding:"required"`
		DestDir  string `json:"dest_dir"  binding:"required"`
		Filename string `json:"filename"  binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "upload_id, dest_dir, filename required"))
		return
	}
	path, err := h.svc.FinalizeChunk(c.Request.Context(), req.UploadID, req.DestDir, req.Filename)
	if err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionUpload, path, fmt.Sprintf("%s (chunked) → %s", req.Filename, req.DestDir), true)
	c.JSON(http.StatusCreated, gin.H{"message": "file assembled", "path": path})
}

// TrashMove  POST /trash
func (h *Handler) TrashMove(c *gin.Context) {
	var req struct {
		Path string `json:"path" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "path is required"))
		return
	}
	if err := h.svc.TrashMove(req.Path); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionTrashMove, req.Path, "", true)
	c.JSON(http.StatusOK, gin.H{"message": "moved to trash"})
}

// TrashList  GET /trash
func (h *Handler) TrashList(c *gin.Context) {
	items, err := h.svc.TrashList()
	if err != nil {
		respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// TrashRestore  POST /trash/restore
func (h *Handler) TrashRestore(c *gin.Context) {
	var req struct {
		ID string `json:"id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, NewError(ErrInvalidRequest, "id is required"))
		return
	}
	if err := h.svc.TrashRestore(req.ID); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionTrashRestore, req.ID, "", true)
	c.JSON(http.StatusOK, gin.H{"message": "restored"})
}

// TrashPurge  DELETE /trash
func (h *Handler) TrashPurge(c *gin.Context) {
	if err := h.svc.TrashPurge(); err != nil {
		respondError(c, err)
		return
	}
	h.record(c, ActionTrashPurge, "", "trash purged", true)
	c.JSON(http.StatusOK, gin.H{"message": "trash purged"})
}

// BulkRename  POST /files/bulk-rename
func (h *Handler) BulkRename(c *gin.Context) {
	var req struct {
		Renames []struct {
			Path    string `json:"path"`
			NewName string `json:"new_name"`
		} `json:"renames" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Renames) == 0 {
		respondError(c, NewError(ErrInvalidRequest, "renames array is required"))
		return
	}
	var errs []string
	for _, r := range req.Renames {
		if err := h.svc.Rename(r.Path, r.NewName); err != nil {
			errs = append(errs, r.Path+": "+err.Error())
		}
	}
	if len(errs) > 0 {
		c.JSON(http.StatusMultiStatus, gin.H{"errors": errs})
		return
	}
	h.record(c, ActionBulkRename, "", fmt.Sprintf("%d items renamed", len(req.Renames)), true)
	c.JSON(http.StatusOK, gin.H{"message": fmt.Sprintf("%d items renamed", len(req.Renames))})
}

// Activity  GET /transfers/activity?limit= — recent transfer history from
// nv_activity (migrated homelabs rows plus new merged-app operations).
func (h *Handler) Activity(c *gin.Context) {
	limit := 100
	if v, err := strconv.Atoi(c.DefaultQuery("limit", "100")); err == nil && v > 0 && v <= 500 {
		limit = v
	}
	rows, err := h.db.Query(`SELECT "time","user","role","action","path","detail","ip","status"
		FROM nv_activity ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not list activity"})
		return
	}
	defer rows.Close()
	out := []database.NvActivity{}
	for rows.Next() {
		var e database.NvActivity
		if err := rows.Scan(&e.Time, &e.User, &e.Role, &e.Action, &e.Path, &e.Detail, &e.IP, &e.Status); err == nil {
			out = append(out, e)
		}
	}
	c.JSON(http.StatusOK, out)
}

// --- helpers ---

func respondError(c *gin.Context, err error) {
	var appErr *AppError
	if errors.As(err, &appErr) {
		c.JSON(httpStatus(appErr.Code), gin.H{"error": appErr.Message})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func httpStatus(code ErrorCode) int {
	switch code {
	case ErrInvalidPath, ErrInvalidRequest, ErrInvalidFileName:
		return http.StatusBadRequest
	case ErrPathTraversal:
		return http.StatusForbidden
	case ErrFileNotFound, ErrDirNotFound:
		return http.StatusNotFound
	case ErrFileExists, ErrDirExists:
		return http.StatusConflict
	case ErrPermissionDenied:
		return http.StatusForbidden
	case ErrFileTooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusInternalServerError
	}
}

func detectMIMEFromName(name string) string {
	ext := filepath.Ext(name)
	switch ext {
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".mp4":
		return "video/mp4"
	case ".txt":
		return "text/plain"
	case ".html":
		return "text/html"
	case ".json":
		return "application/json"
	case ".zip":
		return "application/zip"
	}
	return "application/octet-stream"
}
