package handlers

// Phase 2 follow-up regression tests: deployment-path containment,
// health-check target validation, restore pre-checks, and self-protection.

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/testdb"
)

func TestProjectCreateRejectsOutsideRoots(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	root := t.TempDir()
	h := &ProjectHandler{DB: db, DeployRoots: []string{root}}
	r := gin.New()
	r.POST("/projects", h.Create)

	w := doReq(t, r, "POST", "/projects", map[string]string{
		"name": "evil", "deploymentPath": "/etc",
	}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("outside roots: want 400, got %d %s", w.Code, w.Body.String())
	}
	w = doReq(t, r, "POST", "/projects", map[string]string{
		"name": "evil2", "deploymentPath": root + "/../outside",
	}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("traversal: want 400, got %d %s", w.Code, w.Body.String())
	}
	w = doReq(t, r, "POST", "/projects", map[string]string{
		"name": "evil3", "deploymentPath": filepath.Join(root, "app"),
		"composeFile": "../other.yml",
	}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad compose file: want 400, got %d %s", w.Code, w.Body.String())
	}
	w = doReq(t, r, "POST", "/projects", map[string]string{
		"name": "evil4", "deploymentPath": filepath.Join(root, "app"),
		"healthUrl": "ftp://x/y",
	}, nil)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad health url: want 400, got %d %s", w.Code, w.Body.String())
	}
	w = doReq(t, r, "POST", "/projects", map[string]string{
		"name": "good", "deploymentPath": filepath.Join(root, "app"),
	}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("inside roots: want 200, got %d %s", w.Code, w.Body.String())
	}
}

func TestHealthCheckTargetValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	h := &HealthChecksHandler{DB: db}
	r := gin.New()
	r.POST("/checks", h.Create)

	for _, bad := range []map[string]string{
		{"name": "a", "type": "http", "target": "ftp://x/y"},
		{"name": "b", "type": "http", "target": "http://u:p@h/"},
		{"name": "c", "type": "ping", "target": "-h"},
		{"name": "d", "type": "bogus", "target": "x"},
	} {
		if w := doReq(t, r, "POST", "/checks", bad, nil); w.Code != http.StatusBadRequest {
			t.Fatalf("target %v: want 400, got %d", bad, w.Code)
		}
	}
	if w := doReq(t, r, "POST", "/checks",
		map[string]string{"name": "ok", "type": "http", "target": "http://localhost:8080/h"}, nil); w.Code != http.StatusCreated {
		t.Fatalf("valid target: want 201, got %d %s", w.Code, w.Body.String())
	}
}

func TestRestorePrechecks(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	root := t.TempDir()
	live := filepath.Join(root, "app")
	if err := os.MkdirAll(live, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(live, "compose.yml"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	pid, _ := db.InsertID(`INSERT INTO projects (name, deployment_path) VALUES ('rp', ?)`, live)
	h := &BackupsHandler{DB: db, Dir: t.TempDir(), DeployRoots: []string{root}}
	r := gin.New()
	r.POST("/backups/:id/restore", h.Restore)

	// FAILED status → 409, no operation minted.
	failedID, _ := db.InsertID(`INSERT INTO backups (project_id,kind,path,size_bytes,status) VALUES (?,?,?,?,?)`,
		pid, "snapshot", "/nonexistent.tar.gz", 0, "FAILED")
	w := doReq(t, r, "POST", "/backups/"+itoaInt(failedID)+"/restore?confirm=true", nil, nil)
	if w.Code != http.StatusConflict {
		t.Fatalf("failed backup: want 409, got %d %s", w.Code, w.Body.String())
	}
	// Missing archive file → 404.
	missingID, _ := db.InsertID(`INSERT INTO backups (project_id,kind,path,size_bytes,status) VALUES (?,?,?,?,?)`,
		pid, "snapshot", filepath.Join(root, "gone.tar.gz"), 0, "SUCCESS")
	w = doReq(t, r, "POST", "/backups/"+itoaInt(missingID)+"/restore?confirm=true", nil, nil)
	if w.Code != http.StatusNotFound {
		t.Fatalf("missing archive: want 404, got %d %s", w.Code, w.Body.String())
	}
}

func TestProtectedTarget(t *testing.T) {
	if got := protectedTarget("serverhub-postgres", "abc123", "other"); got == "" {
		t.Fatal("infra name must be protected")
	}
	if got := protectedTarget("/serverhub/", "abc123", "other"); got == "" {
		t.Fatal("infra name with slashes must be protected")
	}
	if got := protectedTarget("myapp", "a1b2c3d4e5f6a1b2c3d4e5f6", "a1b2c3d4e5f6"); got == "" {
		t.Fatal("own container id (full vs short) must be protected")
	}
	if got := protectedTarget("myapp", "a1b2c3d4e5f6", "a1b2c3d4e5f6"); got == "" {
		t.Fatal("own short id must be protected")
	}
	for _, tc := range [][3]string{
		{"myapp", "deadbeefcafe1234", "a1b2c3d4e5f6"},
		{"myapp", "myapp", "somehost"},
		{"", "", ""},
	} {
		if got := protectedTarget(tc[0], tc[1], tc[2]); got != "" {
			t.Fatalf("protectedTarget(%q,%q,%q) = %q, want allowed", tc[0], tc[1], tc[2], got)
		}
	}
}
