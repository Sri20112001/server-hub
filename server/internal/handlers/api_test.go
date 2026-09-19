package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/middleware"
)

const testEncKey = "00112233445566778899aabbccddeeff00112233445566778899aabbccddeeff"

func testSetup(t *testing.T) (*sql.DB, *config.Config, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass123"), bcrypt.MinCost)
	_, err = db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('admin', ?, 'admin')`, string(hash))
	if err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{
		JWTSecret:     "test-jwt-secret-min-32-chars-long!!",
		EncryptionKey: testEncKey,
		WebhookSecret: "webhook-test-secret",
	}
	r := gin.New()
	authH := &AuthHandler{DB: db, Cfg: cfg}
	r.POST("/api/auth/login", authH.Login)
	r.POST("/api/webhooks/github", (&WebhookHandler{DB: db, Cfg: cfg}).GitHub)

	api := r.Group("/api", middleware.AuthRequired(cfg.JWTSecret))
	projH := &ProjectHandler{DB: db}
	api.GET("/projects", projH.List)
	api.POST("/projects", projH.Create)
	api.GET("/projects/:id", projH.Get)
	api.PUT("/projects/:id", projH.Update)
	svcH := &ServiceHandler{DB: db}
	api.POST("/projects/:id/services", svcH.Create)
	ctrH := &ContainerHandler{DB: db, Docker: dockerx.New()}
	api.POST("/containers/:id/stop", ctrH.Stop)
	api.POST("/containers/:id/start", ctrH.Start)
	secH := &SecretHandler{DB: db, Cfg: cfg}
	api.GET("/projects/:id/secrets", secH.List)
	api.POST("/projects/:id/secrets", secH.Upsert)
	api.PUT("/secrets/:id", secH.Update)
	api.POST("/secrets/:id/reveal", secH.Reveal)
	lifeH := &ProjectLifecycle{DB: db}
	api.POST("/projects/:id/stop", lifeH.Stop)
	return db, cfg, r
}

func doReq(t *testing.T, r *gin.Engine, method, path string, body interface{}, cookies []*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if s, ok := body.(string); ok {
			buf.WriteString(s)
		} else if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	for _, ck := range cookies {
		req.AddCookie(ck)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func login(t *testing.T, r *gin.Engine) []*http.Cookie {
	t.Helper()
	w := doReq(t, r, "POST", "/api/auth/login",
		map[string]string{"username": "admin", "password": "testpass123"}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	resp := w.Result()
	return resp.Cookies()
}

func TestAuth(t *testing.T) {
	_, _, r := testSetup(t)
	// bad password
	w := doReq(t, r, "POST", "/api/auth/login",
		map[string]string{"username": "admin", "password": "wrong"}, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	// unauthenticated access
	w = doReq(t, r, "GET", "/api/projects", nil, nil)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	// ok
	cookies := login(t, r)
	w = doReq(t, r, "GET", "/api/projects", nil, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
}

func TestProjectCRUDWithAutoDeploy(t *testing.T) {
	_, _, r := testSetup(t)
	cookies := login(t, r)
	w := doReq(t, r, "POST", "/api/projects",
		map[string]interface{}{"name": "P1", "repository": "https://github.com/acme/p1", "branch": "main", "autoDeploy": true}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var p map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &p); err != nil {
		t.Fatal(err)
	}
	if p["autoDeploy"] != true {
		t.Fatalf("autoDeploy not persisted: %v", p)
	}
	id := p["id"]
	w = doReq(t, r, "PUT", "/api/projects/"+itoa(id),
		map[string]interface{}{"name": "P1", "repository": "https://github.com/acme/p1", "branch": "main", "autoDeploy": false}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("update: %d %s", w.Code, w.Body.String())
	}
	var p2 map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &p2)
	if p2["autoDeploy"] != false {
		t.Fatalf("autoDeploy not updated: %v", p2)
	}
}

func itoa(v interface{}) string {
	switch n := v.(type) {
	case float64:
		return strconv.FormatInt(int64(n), 10)
	case int:
		return strconv.Itoa(n)
	case int64:
		return strconv.FormatInt(n, 10)
	case string:
		return n
	default:
		return strings.Trim(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(
			strings.TrimSpace(string(mustJSON(v))), `"`, ""), " ", "")), `"`)
	}
}

func mustJSON(v interface{}) []byte {
	b, _ := json.Marshal(v)
	return b
}

func TestServiceValidation(t *testing.T) {
	_, _, r := testSetup(t)
	cookies := login(t, r)
	w := doReq(t, r, "POST", "/api/projects",
		map[string]interface{}{"name": "P1"}, cookies)
	var p map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &p)
	pid := "1"
	w = doReq(t, r, "POST", "/api/projects/"+pid+"/services",
		map[string]interface{}{"name": "web", "type": "bogus"}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for bad service type, got %d", w.Code)
	}
	w = doReq(t, r, "POST", "/api/projects/"+pid+"/services",
		map[string]interface{}{"name": "web", "type": "backend"}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d %s", w.Code, w.Body.String())
	}
}

func TestSecretsMetadataOnlyAndRotate(t *testing.T) {
	db, _, r := testSetup(t)
	cookies := login(t, r)
	_ = doReq(t, r, "POST", "/api/projects", map[string]interface{}{"name": "P1"}, cookies)
	// upsert
	w := doReq(t, r, "POST", "/api/projects/1/secrets",
		map[string]interface{}{"name": "JWT_SECRET", "value": "v1"}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("upsert: %d %s", w.Code, w.Body.String())
	}
	// list must not leak value
	w = doReq(t, r, "GET", "/api/projects/1/secrets", nil, cookies)
	if strings.Contains(w.Body.String(), "v1") {
		t.Fatal("secret value leaked in list response")
	}
	var meta []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &meta); err != nil || len(meta) != 1 {
		t.Fatalf("unexpected list: %s", w.Body.String())
	}
	var id int64
	_ = db.QueryRow(`SELECT id FROM secrets WHERE project_id=1`).Scan(&id)
	// reveal
	w = doReq(t, r, "POST", "/api/secrets/"+itoa(float64(id))+"/reveal", nil, cookies)
	var rev map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &rev)
	if rev["value"] != "v1" {
		t.Fatalf("reveal wrong value: %v", rev)
	}
	// rotate via PUT
	w = doReq(t, r, "PUT", "/api/secrets/"+itoa(float64(id)),
		map[string]interface{}{"value": "v2"}, cookies)
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: %d %s", w.Code, w.Body.String())
	}
	w = doReq(t, r, "POST", "/api/secrets/"+itoa(float64(id))+"/reveal", nil, cookies)
	rev = map[string]string{}
	_ = json.Unmarshal(w.Body.Bytes(), &rev)
	if rev["value"] != "v2" {
		t.Fatalf("rotation did not take effect: %v", rev)
	}
}

func TestContainerStopRequiresConfirm(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:12345") // Force docker down for test
	_, _, r := testSetup(t)
	cookies := login(t, r)
	// no confirm → 400 (gate works even without docker)
	w := doReq(t, r, "POST", "/api/containers/abc/stop", map[string]interface{}{}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without confirm, got %d %s", w.Code, w.Body.String())
	}
	// with confirm but no docker daemon → 503, not 500
	w = doReq(t, r, "POST", "/api/containers/abc/stop?confirm=true", nil, cookies)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503 without docker, got %d %s", w.Code, w.Body.String())
	}
}

func TestProjectStopRequiresConfirm(t *testing.T) {
	_, _, r := testSetup(t)
	cookies := login(t, r)
	_ = doReq(t, r, "POST", "/api/projects", map[string]interface{}{"name": "P1"}, cookies)
	w := doReq(t, r, "POST", "/api/projects/1/stop", map[string]interface{}{}, cookies)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without confirm, got %d %s", w.Code, w.Body.String())
	}
}

func TestWebhook(t *testing.T) {
	db, cfg, r := testSetup(t)
	payload := `{"ref":"refs/heads/main","after":"abc123","repository":{"full_name":"acme/p1","html_url":"https://github.com/acme/p1"}}`
	// bad signature
	req := httptest.NewRequest("POST", "/api/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", "sha256=deadbeef")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	// ping
	req = httptest.NewRequest("POST", "/api/webhooks/github", strings.NewReader(`{}`))
	req.Header.Set("X-GitHub-Event", "ping")
	req.Header.Set("X-Hub-Signature-256", SignBody(cfg.WebhookSecret, []byte(`{}`)))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "pong") {
		t.Fatalf("ping failed: %d %s", w.Code, w.Body.String())
	}
	// seed project with autoDeploy disabled
	cookies := login(t, r)
	_ = doReq(t, r, "POST", "/api/projects",
		map[string]interface{}{"name": "P1", "repository": "https://github.com/acme/p1", "branch": "main"}, cookies)
	// push → recorded PENDING, not deployed
	req = httptest.NewRequest("POST", "/api/webhooks/github", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", SignBody(cfg.WebhookSecret, []byte(payload)))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("push: %d %s", w.Code, w.Body.String())
	}
	var out map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["matched"] != float64(1) {
		t.Fatalf("expected 1 match: %s", w.Body.String())
	}
	var status, trigger string
	_ = db.QueryRow(`SELECT status, trigger FROM deployments ORDER BY id DESC LIMIT 1`).Scan(&status, &trigger)
	if status != "PENDING" || trigger != "github-webhook" {
		t.Fatalf("unexpected deployment row: %s %s", status, trigger)
	}
	// push to non-matching branch → 0 matches
	payload2 := `{"ref":"refs/heads/dev","after":"def456","repository":{"full_name":"acme/p1","html_url":"https://github.com/acme/p1"}}`
	req = httptest.NewRequest("POST", "/api/webhooks/github", strings.NewReader(payload2))
	req.Header.Set("X-GitHub-Event", "push")
	req.Header.Set("X-Hub-Signature-256", SignBody(cfg.WebhookSecret, []byte(payload2)))
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	out = map[string]interface{}{}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["matched"] != float64(0) {
		t.Fatalf("expected 0 matches: %s", w.Body.String())
	}
}
