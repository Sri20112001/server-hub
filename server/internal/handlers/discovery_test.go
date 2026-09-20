package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/middleware"
)

func strReader(s string) *strings.Reader { return strings.NewReader(s) }
func contains(hay, needle string) bool   { return strings.Contains(hay, needle) }

func discoverySetup(t *testing.T) (*gin.Engine, *config.Config) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db, err := database.OpenDatabase("", t.TempDir()+"/disc.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	hash, _ := bcrypt.GenerateFromPassword([]byte("testpass123"), bcrypt.MinCost)
	_, _ = db.Exec(`INSERT INTO users (username, password_hash, role) VALUES ('admin', ?, 'admin')`, string(hash))
	cfg := &config.Config{
		JWTSecret:     "test-jwt-secret-min-32-chars-long!!",
		EncryptionKey: testEncKey,
	}
	r := gin.New()
	authH := &AuthHandler{DB: db, Cfg: cfg}
	r.POST("/api/auth/login", authH.Login)
	discH := &DiscoveryHandler{DB: db, Docker: dockerx.New()}
	api := r.Group("/api", middleware.AuthRequired(cfg.JWTSecret))
	api.GET("/discovery", discH.Scan)
	api.POST("/discovery/import", discH.Import)
	return r, cfg
}

func TestGuessType(t *testing.T) {
	cases := map[[2]string]string{
		{"db", "postgres:15"}:          "database",
		{"mongo", "mongo:7"}:           "database",
		{"cache", "redis:7"}:           "cache",
		{"queue", "app:latest"}:        "worker",
		{"api", "node:20"}:             "backend",
		{"web", "nginx:alpine"}:        "frontend",
		{"mystery", "mystery:1"}:       "other",
		{"auth-service", "auth:1.2.3"}: "other",
	}
	for in, want := range cases {
		if got := guessType(in[0], in[1]); got != want {
			t.Errorf("guessType(%q,%q) = %q, want %q", in[0], in[1], got, want)
		}
	}
}

func TestDiscoveryWithoutDocker(t *testing.T) {
	t.Setenv("DOCKER_HOST", "tcp://127.0.0.1:12345") // Force docker down for test
	r, _ := discoverySetup(t)
	// login
	body := `{"username":"admin","password":"testpass123"}`
	req := httptest.NewRequest("POST", "/api/auth/login", strReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("login: %d", w.Code)
	}
	var cookies []*http.Cookie
	for _, c := range w.Result().Cookies() {
		cookies = append(cookies, c)
	}
	// scan without a daemon → friendly empty result, not an error
	req2 := httptest.NewRequest("GET", "/api/discovery", nil)
	for _, c := range cookies {
		req2.AddCookie(c)
	}
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("scan: %d %s", w2.Code, w2.Body.String())
	}
	if got := w2.Body.String(); !contains(got, `"dockerAvailable":false`) {
		t.Fatalf("expected dockerAvailable false, got %s", got)
	}
	// import without a daemon → 503
	req3 := httptest.NewRequest("POST", "/api/discovery/import", strReader(`{"name":"x"}`))
	req3.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req3.AddCookie(c)
	}
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	if w3.Code != http.StatusServiceUnavailable {
		t.Fatalf("import without docker: %d %s", w3.Code, w3.Body.String())
	}
	// import validation
	req4 := httptest.NewRequest("POST", "/api/discovery/import", strReader(`{}`))
	req4.Header.Set("Content-Type", "application/json")
	for _, c := range cookies {
		req4.AddCookie(c)
	}
	w4 := httptest.NewRecorder()
	r.ServeHTTP(w4, req4)
	if w4.Code != http.StatusBadRequest {
		t.Fatalf("import validation: %d", w4.Code)
	}
}
