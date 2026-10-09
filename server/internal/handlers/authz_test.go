package handlers

// Tests for the P0/P1 security hardening: RBAC route matrix, admin user
// management guards, login rate limiting, and refresh-token rotation with
// reuse detection.

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/dockerx"
	"serverhub/internal/middleware"
	"serverhub/internal/testdb"
)

func testSetupSecure(t *testing.T) (*database.DB, *config.Config, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	seed := func(user, pass, role string) {
		hash, _ := bcrypt.GenerateFromPassword([]byte(pass), bcrypt.MinCost)
		if _, err := db.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?,?)`,
			user, string(hash), role); err != nil {
			t.Fatal(err)
		}
	}
	seed("admin", "testpass123", "admin")
	seed("op", "testpass123", "operator")
	seed("view", "testpass123", "viewer")

	cfg := &config.Config{
		JWTSecret:     "test-jwt-secret-min-32-chars-long!!",
		EncryptionKey: testEncKey,
		WebhookSecret: "webhook-test-secret",
	}
	r := gin.New()
	authH := &AuthHandler{DB: db, Cfg: cfg}
	limiter := middleware.NewLoginLimiter()
	r.POST("/api/auth/login", middleware.LoginRateLimit(nil, limiter), authH.Login)
	r.POST("/api/auth/token", middleware.LoginRateLimit(nil, limiter), authH.Token)
	r.POST("/api/auth/refresh", authH.Refresh)
	r.POST("/api/auth/logout", middleware.AuthRequired(cfg.JWTSecret), authH.Logout)

	authd := middleware.AuthRequired(cfg.JWTSecret)
	viewer := r.Group("/api", authd, middleware.RequireRole(middleware.RoleViewer))
	operator := r.Group("/api", authd, middleware.RequireRole(middleware.RoleOperator))
	admin := r.Group("/api", authd, middleware.RequireRole(middleware.RoleAdmin))

	projH := &ProjectHandler{DB: db}
	viewer.GET("/projects", projH.List)
	operator.POST("/projects", projH.Create)
	ctrH := &ContainerHandler{DB: db, Docker: dockerx.New()}
	operator.POST("/containers/:id/start", ctrH.Start)
	admin.POST("/containers/:id/stop", ctrH.Stop)
	execH := &ExecHandler{DB: db, Docker: dockerx.New(), Broker: nil}
	admin.POST("/containers/:id/exec", execH.Create)
	depH := &DeploymentHandler{DB: db, Broker: nil}
	operator.POST("/projects/:id/deploy", depH.Deploy)
	operator.POST("/projects/:id/services", (&ServiceHandler{DB: db}).Create)
	backH := &BackupsHandler{DB: db, Broker: nil}
	admin.POST("/backups/:id/restore", backH.Restore)
	secH := &SecretHandler{DB: db, Cfg: cfg}
	admin.POST("/secrets/:id/reveal", secH.Reveal)
	usrH := &UserHandler{DB: db}
	admin.GET("/users", usrH.List)
	admin.POST("/users", usrH.Create)
	admin.PUT("/users/:username/role", usrH.SetRole)
	admin.DELETE("/users/:username", usrH.Delete)
	return db, cfg, r
}

func loginAs(t *testing.T, r *gin.Engine, username, password string) []*http.Cookie {
	t.Helper()
	w := doReq(t, r, "POST", "/api/auth/login",
		map[string]string{"username": username, "password": password}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("login %s failed: %d %s", username, w.Code, w.Body.String())
	}
	return w.Result().Cookies()
}

func cookieByName(cookies []*http.Cookie, name string) *http.Cookie {
	for _, c := range cookies {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func mintToken(t *testing.T, secret, username, role string) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub": username, "role": role,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix(),
	})
	s, err := tok.SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestRBACMatrix(t *testing.T) {
	_, cfg, r := testSetupSecure(t)
	adminC := loginAs(t, r, "admin", "testpass123")
	opC := loginAs(t, r, "op", "testpass123")
	viewC := loginAs(t, r, "view", "testpass123")

	// viewer: reads OK, writes forbidden
	if w := doReq(t, r, "GET", "/api/projects", nil, viewC); w.Code != http.StatusOK {
		t.Fatalf("viewer list: %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "POST", "/api/projects", map[string]string{"name": "x"}, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer create: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/api/users", nil, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer users: want 403, got %d", w.Code)
	}

	// operator: project writes OK, admin routes forbidden
	if w := doReq(t, r, "POST", "/api/projects", map[string]string{"name": "op-proj"}, opC); w.Code != http.StatusOK {
		t.Fatalf("operator create: %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "GET", "/api/users", nil, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator users: want 403, got %d", w.Code)
	}

	// admin: everything OK
	if w := doReq(t, r, "GET", "/api/users", nil, adminC); w.Code != http.StatusOK {
		t.Fatalf("admin users: %d %s", w.Code, w.Body.String())
	}

	// unauthenticated: 401 everywhere
	if w := doReq(t, r, "GET", "/api/projects", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon list: want 401, got %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/api/users", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon users: want 401, got %d", w.Code)
	}

	// unknown role fails closed even with a valid signature
	bad := &http.Cookie{Name: "serverhub_session", Value: mintToken(t, cfg.JWTSecret, "mallory", "superuser")}
	if w := doReq(t, r, "GET", "/api/projects", nil, []*http.Cookie{bad}); w.Code != http.StatusForbidden {
		t.Fatalf("unknown role: want 403, got %d", w.Code)
	}
	// roleless legacy-shaped token also fails closed on role-gated routes
	roleless := &http.Cookie{Name: "serverhub_session", Value: mintToken(t, cfg.JWTSecret, "mallory", "")}
	if w := doReq(t, r, "GET", "/api/projects", nil, []*http.Cookie{roleless}); w.Code != http.StatusForbidden {
		t.Fatalf("roleless token: want 403, got %d", w.Code)
	}
}

// TestPrivilegedMatrix exercises every destructive/privileged route as
// anon, viewer, operator and admin. Docker-dependent handlers run without
// a daemon here: viewer/operator must be stopped by middleware (403)
// before any handler logic, while admin must reach the handler (4xx/5xx
// from the handler itself proves authorization passed).
func TestPrivilegedMatrix(t *testing.T) {
	_, _, r := testSetupSecure(t)
	adminC := loginAs(t, r, "admin", "testpass123")
	opC := loginAs(t, r, "op", "testpass123")
	viewC := loginAs(t, r, "view", "testpass123")

	type tc struct {
		method, path string
		body         interface{}
		anon, viewer int
		operator     int
		admin        int
	}
	cases := []tc{
		// exec mint (admin-only; no confirm → 400 proves admin got through)
		{"POST", "/api/containers/abc/exec", nil,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusForbidden, http.StatusBadRequest},
		// container stop (admin-only, confirm-gated; no daemon → 503
		// from the handler proves admin passed authorization)
		{"POST", "/api/containers/abc/stop?confirm=true", nil,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusForbidden, http.StatusServiceUnavailable},
		// restore (admin-only since hardening; missing row → 404 for admin)
		{"POST", "/api/backups/999999/restore?confirm=true", nil,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusForbidden, http.StatusNotFound},
		// secret reveal (admin-only; missing row → 404 for admin)
		{"POST", "/api/secrets/999999/reveal", nil,
			http.StatusUnauthorized, http.StatusForbidden, http.StatusForbidden, http.StatusNotFound},
		// deploy (operator+; missing project → 404 past authz)
		{"POST", "/api/projects/999999/deploy", map[string]string{},
			http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound, http.StatusNotFound},
		// service create with bad health_url (operator+; 400 validation)
		{"POST", "/api/projects/1/services",
			map[string]string{"name": "s", "type": "other", "healthUrl": "ftp://x/y"},
			http.StatusUnauthorized, http.StatusForbidden, http.StatusBadRequest, http.StatusBadRequest},
	}
	for _, c := range cases {
		if w := doReq(t, r, c.method, c.path, c.body, nil); w.Code != c.anon {
			t.Fatalf("%s %s anon: want %d, got %d", c.method, c.path, c.anon, w.Code)
		}
		if w := doReq(t, r, c.method, c.path, c.body, viewC); w.Code != c.viewer {
			t.Fatalf("%s %s viewer: want %d, got %d", c.method, c.path, c.viewer, w.Code)
		}
		if w := doReq(t, r, c.method, c.path, c.body, opC); w.Code != c.operator {
			t.Fatalf("%s %s operator: want %d, got %d %s", c.method, c.path, c.operator, w.Code, w.Body.String())
		}
		if w := doReq(t, r, c.method, c.path, c.body, adminC); w.Code != c.admin {
			t.Fatalf("%s %s admin: want %d, got %d %s", c.method, c.path, c.admin, w.Code, w.Body.String())
		}
	}
}

func TestUserManagementGuards(t *testing.T) {
	_, _, r := testSetupSecure(t)
	adminC := loginAs(t, r, "admin", "testpass123")

	// create viewer
	w := doReq(t, r, "POST", "/api/users",
		map[string]string{"username": "newview", "password": "longenough1", "role": "viewer"}, adminC)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	// duplicate
	if w := doReq(t, r, "POST", "/api/users",
		map[string]string{"username": "newview", "password": "longenough1", "role": "viewer"}, adminC); w.Code != http.StatusConflict {
		t.Fatalf("duplicate: want 409, got %d", w.Code)
	}
	// weak password
	if w := doReq(t, r, "POST", "/api/users",
		map[string]string{"username": "weak", "password": "short", "role": "viewer"}, adminC); w.Code != http.StatusBadRequest {
		t.Fatalf("weak password: want 400, got %d", w.Code)
	}
	// bad role
	if w := doReq(t, r, "POST", "/api/users",
		map[string]string{"username": "x", "password": "longenough1", "role": "root"}, adminC); w.Code != http.StatusBadRequest {
		t.Fatalf("bad role: want 400, got %d", w.Code)
	}
	// bad username
	if w := doReq(t, r, "POST", "/api/users",
		map[string]string{"username": "../evil", "password": "longenough1", "role": "viewer"}, adminC); w.Code != http.StatusBadRequest {
		t.Fatalf("bad username: want 400, got %d", w.Code)
	}
	// non-admin cannot manage users
	opC := loginAs(t, r, "op", "testpass123")
	if w := doReq(t, r, "POST", "/api/users",
		map[string]string{"username": "nope", "password": "longenough1", "role": "viewer"}, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator create user: want 403, got %d", w.Code)
	}
	// cannot demote self, delete self, demote/delete last admin
	if w := doReq(t, r, "PUT", "/api/users/admin/role", map[string]string{"role": "viewer"}, adminC); w.Code != http.StatusBadRequest {
		t.Fatalf("self demote: want 400, got %d", w.Code)
	}
	if w := doReq(t, r, "DELETE", "/api/users/admin", nil, adminC); w.Code != http.StatusBadRequest {
		t.Fatalf("self delete: want 400, got %d", w.Code)
	}
	// create second admin, demote first admin OK, then demoting the last one fails
	if w := doReq(t, r, "POST", "/api/users",
		map[string]string{"username": "admin2", "password": "longenough1", "role": "admin"}, adminC); w.Code != http.StatusCreated {
		t.Fatalf("create admin2: %d %s", w.Code, w.Body.String())
	}
	admin2C := loginAs(t, r, "admin2", "longenough1")
	if w := doReq(t, r, "PUT", "/api/users/admin/role", map[string]string{"role": "operator"}, admin2C); w.Code != http.StatusOK {
		t.Fatalf("demote non-last admin: %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "DELETE", "/api/users/admin2", nil, admin2C); w.Code != http.StatusBadRequest {
		t.Fatalf("delete self (last admin): want 400, got %d", w.Code)
	}
}

func TestLoginRateLimit(t *testing.T) {
	_, _, r := testSetupSecure(t)
	bad := map[string]string{"username": "admin", "password": "wrong"}
	for i := 0; i < 5; i++ {
		if w := doReq(t, r, "POST", "/api/auth/login", bad, nil); w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: want 401, got %d", i+1, w.Code)
		}
	}
	w := doReq(t, r, "POST", "/api/auth/login", bad, nil)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("attempt 6: want 429, got %d %s", w.Code, w.Body.String())
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("429 without Retry-After header")
	}
}

func TestRefreshRotationAndReuse(t *testing.T) {
	db, _, r := testSetupSecure(t)
	cookies := loginAs(t, r, "admin", "testpass123")
	r1 := cookieByName(cookies, "serverhub_refresh")
	if r1 == nil || r1.Value == "" {
		t.Fatal("login did not set serverhub_refresh cookie")
	}
	if c := cookieByName(cookies, "serverhub_session"); c == nil || c.Value == "" {
		t.Fatal("login did not set serverhub_session cookie")
	}

	// rotate with the cookie
	w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r1})
	if w.Code != http.StatusOK {
		t.Fatalf("refresh: %d %s", w.Code, w.Body.String())
	}
	r2 := cookieByName(w.Result().Cookies(), "serverhub_refresh")
	if r2 == nil || r2.Value == "" || r2.Value == r1.Value {
		t.Fatal("refresh did not rotate the refresh cookie")
	}

	// reuse of the old token → theft response: 401 + sessions revoked.
	// Revocation spares tokens minted in the last 30s (race leeway), so
	// backdate r2 to prove older sessions die while fresh ones survive.
	if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r1}); w.Code != http.StatusUnauthorized {
		t.Fatalf("reuse: want 401, got %d", w.Code)
	}
	w2 := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r2})
	if w2.Code != http.StatusOK {
		t.Fatalf("fresh post-reuse refresh: want 200 (race leeway), got %d", w2.Code)
	}
	// An OLD session does not survive reuse: backdate r2 past the 30s
	// race leeway, rotate once more, then reuse the pre-rotation token.
	r2fresh := cookieByName(w2.Result().Cookies(), "serverhub_refresh")
	if r2fresh == nil || r2fresh.Value == "" {
		t.Fatal("rotation issued no replacement refresh cookie")
	}
	if _, err := db.Exec(`UPDATE refresh_tokens SET created_at = CURRENT_TIMESTAMP - INTERVAL '1 hour' WHERE token_hash=?`,
		hashRefresh(r2fresh.Value)); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	w2 = doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r2fresh})
	if w2.Code != http.StatusOK {
		t.Fatalf("rotate backdated token: %d %s", w2.Code, w2.Body.String())
	}
	r3 := cookieByName(w2.Result().Cookies(), "serverhub_refresh")
	// Age r3 past the leeway too, then reuse must kill it.
	if _, err := db.Exec(`UPDATE refresh_tokens SET created_at = CURRENT_TIMESTAMP - INTERVAL '1 hour' WHERE token_hash=?`,
		hashRefresh(r3.Value)); err != nil {
		t.Fatalf("backdate r3: %v", err)
	}
	if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r2fresh}); w.Code != http.StatusUnauthorized {
		t.Fatalf("reuse of backdated token: want 401, got %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r3}); w.Code != http.StatusUnauthorized {
		t.Fatalf("old post-reuse refresh: want 401, got %d", w.Code)
	}

	// mobile-style body refresh works too
	cookies2 := loginAs(t, r, "admin", "testpass123")
	old := cookieByName(cookies2, "serverhub_refresh")
	w = doReq(t, r, "POST", "/api/auth/refresh", map[string]string{"refreshToken": old.Value}, nil)
	if w.Code != http.StatusOK {
		t.Fatalf("body refresh: %d %s", w.Code, w.Body.String())
	}

	// logout revokes: refresh after logout fails
	cookies3 := loginAs(t, r, "admin", "testpass123")
	lr := cookieByName(cookies3, "serverhub_refresh")
	if w := doReq(t, r, "POST", "/api/auth/logout", nil, cookies3); w.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{lr}); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after logout: want 401, got %d", w.Code)
	}
}

func TestAccessTokenExpiry(t *testing.T) {
	_, _, r := testSetupSecure(t)
	cookies := loginAs(t, r, "admin", "testpass123")
	sess := cookieByName(cookies, "serverhub_session")
	if sess == nil {
		t.Fatal("no session cookie")
	}
	// access tokens carry a ~30 minute expiry, not 24h
	parser := jwt.NewParser()
	claims := jwt.MapClaims{}
	if _, _, err := parser.ParseUnverified(sess.Value, claims); err != nil {
		t.Fatalf("parse access token: %v", err)
	}
	exp, _ := claims["exp"].(float64)
	iat, _ := claims["iat"].(float64)
	if exp-iat > float64((31 * time.Minute).Seconds()) {
		t.Fatalf("access token lifetime too long: %.0fs", exp-iat)
	}
	if typ, _ := claims["typ"].(string); typ != "access" {
		t.Fatalf("access token typ=%q, want access", typ)
	}
}
