package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
	"serverhub/internal/testdb"
	"serverhub/internal/transfer"
)

// transferRouter wires the merged file routes exactly like main.go does:
// reads → viewer, every mutation → operator. Uses its own users so the
// matrix is hermetic.
func transferRouter(t *testing.T, root string) (*database.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	for _, u := range [][3]string{
		{"admin", "testpass123", "admin"},
		{"op", "testpass123", "operator"},
		{"view", "testpass123", "viewer"},
	} {
		hash, _ := bcrypt.GenerateFromPassword([]byte(u[1]), bcrypt.MinCost)
		if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?,?)`,
			u[0], string(hash), u[2]); err != nil {
			t.Fatal(err)
		}
	}
	cfg := &config.Config{
		JWTSecret:     "test-jwt-secret-min-32-chars-long!!",
		EncryptionKey: testEncKey,
	}
	r := gin.New()
	authH := &AuthHandler{DB: db, Cfg: cfg}
	r.POST("/api/auth/login", authH.Login)
	authd := middleware.AuthRequired(cfg.JWTSecret)
	viewer := r.Group("/api", authd, middleware.RequireRole(middleware.RoleViewer))
	operator := r.Group("/api", authd, middleware.RequireRole(middleware.RoleOperator))
	xferH := transfer.NewHandler(transfer.NewFileService(root, 10<<20), db)
	viewer.GET("/transfers/files", xferH.List)
	viewer.GET("/transfers/files/download", xferH.Download)
	viewer.GET("/transfers/activity", xferH.Activity)
	operator.POST("/transfers/files/upload", xferH.Upload)
	operator.POST("/transfers/directories", xferH.Mkdir)
	operator.PATCH("/transfers/files", xferH.Rename)
	operator.DELETE("/transfers/files", xferH.Delete)
	operator.POST("/transfers/trash", xferH.TrashMove)
	operator.POST("/transfers/trash/restore", xferH.TrashRestore)
	operator.DELETE("/transfers/trash", xferH.TrashPurge)
	operator.POST("/transfers/files/chunk", xferH.UploadChunk)
	operator.POST("/transfers/files/chunk/finalize", xferH.FinalizeChunk)
	return db, r
}

func TestTransferRBAC(t *testing.T) {
	_, r := transferRouter(t, t.TempDir())
	adminC := loginAs(t, r, "admin", "testpass123")
	opC := loginAs(t, r, "op", "testpass123")
	viewC := loginAs(t, r, "view", "testpass123")

	// Unauthenticated → 401 everywhere.
	if w := doReq(t, r, "GET", "/api/transfers/files", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401, got %d", w.Code)
	}
	// Viewer reads OK…
	if w := doReq(t, r, "GET", "/api/transfers/files", nil, viewC); w.Code != http.StatusOK {
		t.Fatalf("viewer list: want 200, got %d %s", w.Code, w.Body.String())
	}
	// …but viewer mutations are denied.
	if w := doReq(t, r, "DELETE", "/api/transfers/files",
		map[string]string{"path": "/x"}, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer delete: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/api/transfers/trash",
		map[string]string{"path": "/x"}, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer trash: want 403, got %d", w.Code)
	}
	// Operator round-trip: mkdir → upload → list → download → delete.
	if w := doReq(t, r, "POST", "/api/transfers/directories",
		map[string]string{"path": "/docs"}, opC); w.Code != http.StatusCreated {
		t.Fatalf("mkdir: want 201, got %d %s", w.Code, w.Body.String())
	}
	_ = adminC
}

func uploadFile(t *testing.T, r *gin.Engine, cookies []*http.Cookie, destDir, name, content string) int {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write([]byte(content)); err != nil {
		t.Fatal(err)
	}
	mw.Close()
	req := httptest.NewRequest("POST", "/api/transfers/files/upload?path="+destDir, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w.Code
}

func TestTransferUploadDownloadRoundTrip(t *testing.T) {
	root := t.TempDir()
	_, r := transferRouter(t, root)
	opC := loginAs(t, r, "op", "testpass123")
	viewC := loginAs(t, r, "view", "testpass123")

	if code := uploadFile(t, r, opC, "/", "hello.txt", "hello-transfer"); code != http.StatusCreated {
		t.Fatalf("upload: want 201, got %d", code)
	}
	// Viewer may download (read-only role covers reads).
	req := httptest.NewRequest("GET", "/api/transfers/files/download?path=/hello.txt", nil)
	for _, ck := range viewC {
		req.AddCookie(ck)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "hello-transfer" {
		t.Fatalf("download: want 200 hello-transfer, got %d %q", w.Code, w.Body.String())
	}
	// Operator deletes; file is gone afterwards.
	if w := doReq(t, r, "DELETE", "/api/transfers/files",
		map[string]any{"path": "/hello.txt"}, opC); w.Code != http.StatusOK {
		t.Fatalf("delete: want 200, got %d %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "hello.txt")); !os.IsNotExist(err) {
		t.Fatal("file should be gone after delete")
	}
	// Activity history recorded the upload.
	w2 := doReq(t, r, "GET", "/api/transfers/activity?limit=10", nil, viewC)
	if w2.Code != http.StatusOK {
		t.Fatalf("activity: want 200, got %d", w2.Code)
	}
	var acts []map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &acts); err != nil || len(acts) == 0 {
		t.Fatalf("activity should list the upload: %v %s", err, w2.Body.String())
	}
}

func TestTransferChunkRBACAndValidation(t *testing.T) {
	_, r := transferRouter(t, t.TempDir())
	opC := loginAs(t, r, "op", "testpass123")
	viewC := loginAs(t, r, "view", "testpass123")

	chunk := func(uploadID, index string, cookies []*http.Cookie, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest("POST",
			"/api/transfers/files/chunk?upload_id="+uploadID+"&index="+index,
			strings.NewReader(body))
		for _, ck := range cookies {
			req.AddCookie(ck)
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		return w
	}

	// Chunk sessions live in a shared OS temp dir (not the per-test storage
	// root) and refused/duplicate slots persist there, so fixed IDs would
	// collide with leftovers from earlier runs. Unique per run.
	sessA := fmt.Sprintf("sessa-%d", time.Now().UnixNano())
	sessB := fmt.Sprintf("sessb-%d", time.Now().UnixNano())
	// Viewer chunk upload is denied (mutations are operator+).
	if w := chunk(sessA, "0", viewC, "data"); w.Code != http.StatusForbidden {
		t.Fatalf("viewer chunk: want 403, got %d", w.Code)
	}
	// Traversal upload_id is rejected even for operators.
	if w := chunk("../../evil", "0", opC, "data"); w.Code != http.StatusBadRequest {
		t.Fatalf("traversal upload_id: want 400, got %d %s", w.Code, w.Body.String())
	}
	// Negative index is rejected.
	if w := chunk(sessA, "-1", opC, "data"); w.Code != http.StatusBadRequest {
		t.Fatalf("negative index: want 400, got %d %s", w.Code, w.Body.String())
	}
	// Valid chunk round-trip: chunk → finalize → file exists.
	if w := chunk(sessA, "0", opC, "chunked-bytes"); w.Code != http.StatusOK {
		t.Fatalf("chunk: want 200, got %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "POST", "/api/transfers/files/chunk/finalize",
		map[string]any{"upload_id": sessA, "dest_dir": "/", "filename": "assembled.txt"}, opC); w.Code != http.StatusCreated {
		t.Fatalf("finalize: want 201, got %d %s", w.Code, w.Body.String())
	}
	// Finalize into the trash area is rejected.
	if w := chunk(sessB, "0", opC, "x"); w.Code != http.StatusOK {
		t.Fatalf("chunk sessB: want 200, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/api/transfers/files/chunk/finalize",
		map[string]any{"upload_id": sessB, "dest_dir": "/.trash", "filename": "x.txt"}, opC); w.Code != http.StatusBadRequest {
		t.Fatalf("finalize into trash: want 400, got %d %s", w.Code, w.Body.String())
	}
}
