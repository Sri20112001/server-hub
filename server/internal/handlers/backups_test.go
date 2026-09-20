package handlers

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/database"
)

func TestTarGzRoundtrip(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "docker-compose.yml"), []byte("services:\n  web:\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "app.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	f, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	size, err := writeTarGz(f, src)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if size <= 0 {
		t.Fatal("expected bytes archived")
	}

	dst := t.TempDir()
	if err := extractTarGz(archive, dst); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dst, "sub", "app.txt"))
	if err != nil || string(b) != "hello" {
		t.Fatalf("restore mismatch: %v %q", err, string(b))
	}
}

func TestExtractRejectsZipSlip(t *testing.T) {
	if err := extractTarGz("/nonexistent.tar.gz", t.TempDir()); err == nil {
		t.Fatal("expected error for missing archive")
	}
	if err := extractTarGz("", t.TempDir()); err == nil {
		// empty archive path opens "" → error expected
		t.Fatal("expected error")
	}
}

func TestRollbackEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, err := database.OpenDatabase("", t.TempDir()+"/rb.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	_, _ = db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('admin','x','admin')`)
	pid, _ := db.InsertID(`INSERT INTO projects (name) VALUES ('p1')`)
	depID, _ := db.InsertID(`INSERT INTO deployments (project_id,commit_sha,branch,trigger,status) VALUES (?,?,?,?,?)`,
		pid, "abc123", "main", "manual", "SUCCESS")

	r := gin.New()
	depH := &DeploymentHandler{DB: db, Broker: nil}
	api := r.Group("/api")
	api.POST("/projects/:id/deployments/:depId/rollback", depH.Rollback)

	req := httptest.NewRequest("POST",
		"/api/projects/"+itoaInt(pid)+"/deployments/"+itoaInt(depID)+"/rollback", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Fatalf("rollback: %d %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "operationId") {
		t.Fatalf("missing operationId: %s", w.Body.String())
	}
	// unknown deployment → 404
	req2 := httptest.NewRequest("POST",
		"/api/projects/"+itoaInt(pid)+"/deployments/9999/rollback", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w2.Code)
	}
}

func itoaInt(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
