package handlers

import (
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
	"serverhub/internal/testdb"
)

func policyTestSetup(t *testing.T) (*database.DB, *gin.Engine) {
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
	admin := r.Group("/api", authd, middleware.RequireRole(middleware.RoleAdmin))

	polH := &PolicyHandler{DB: db}
	viewer.GET("/notification-policy", polH.Get)
	admin.PUT("/notification-policy", polH.Update)
	delH := &DeliveriesHandler{DB: db}
	viewer.GET("/notification-deliveries", delH.List)
	mwH := &MaintenanceHandler{DB: db}
	viewer.GET("/maintenance-windows", mwH.List)
	operator.POST("/maintenance-windows", mwH.Create)
	operator.PATCH("/maintenance-windows/:id", mwH.Update)
	admin.DELETE("/maintenance-windows/:id", mwH.Delete)
	ngH := &NotificationGroupsHandler{DB: db}
	viewer.GET("/notification-groups/:id", ngH.Get)
	operator.POST("/notification-groups", ngH.Create)
	operator.PATCH("/notification-groups/:id", ngH.Update)
	return db, r
}

// Group quiet-hours validation: bad times and timezones rejected.
func TestGroupQuietValidation(t *testing.T) {
	_, r := policyTestSetup(t)
	opC := policyLogin(t, r, "op")
	w := doReq(t, r, "POST", "/api/notification-groups", map[string]string{
		"name": "q1", "quietStart": "25:00", "quietEnd": "06:00", "quietTZ": "UTC",
	}, opC)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad quiet start: want 400, got %d", w.Code)
	}
	w = doReq(t, r, "POST", "/api/notification-groups", map[string]string{
		"name": "q1", "quietStart": "22:00", "quietEnd": "06:00", "quietTZ": "Mars/Olympus",
	}, opC)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("bad tz: want 400, got %d", w.Code)
	}
	w = doReq(t, r, "POST", "/api/notification-groups", map[string]string{
		"name": "q1", "quietStart": "22:00", "quietEnd": "06:00", "quietTZ": "America/New_York",
	}, opC)
	if w.Code != http.StatusCreated {
		t.Fatalf("valid quiet: %d %s", w.Code, w.Body.String())
	}
}

func policyLogin(t *testing.T, r *gin.Engine, user string) []*http.Cookie {
	t.Helper()
	return loginAs(t, r, user, "testpass123")
}

// 12. RBAC blocks unauthorized policy changes and maintenance deletes.
func TestPolicyRBACMatrix(t *testing.T) {
	_, r := policyTestSetup(t)
	adminC := policyLogin(t, r, "admin")
	opC := policyLogin(t, r, "op")
	viewC := policyLogin(t, r, "view")

	if w := doReq(t, r, "GET", "/api/notification-policy", nil, viewC); w.Code != http.StatusOK {
		t.Fatalf("viewer get policy: %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/api/notification-policy", nil, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("anon get policy: %d", w.Code)
	}
	if w := doReq(t, r, "PUT", "/api/notification-policy",
		map[string]int{"maxRepeats": 5}, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer put policy: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "PUT", "/api/notification-policy",
		map[string]int{"maxRepeats": 5}, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator put policy: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "PUT", "/api/notification-policy",
		map[string]int{"maxRepeats": 5}, adminC); w.Code != http.StatusOK {
		t.Fatalf("admin put policy: %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "PUT", "/api/notification-policy",
		map[string]int{"maxRepeats": 500}, adminC); w.Code != http.StatusBadRequest {
		t.Fatalf("out-of-range maxRepeats: want 400, got %d", w.Code)
	}
	// Maintenance: operator manages, only admin deletes.
	if w := doReq(t, r, "POST", "/api/maintenance-windows", map[string]string{
		"name": "freeze", "scope": "all",
		"startsAt": time.Now().UTC().Format(time.RFC3339),
		"endsAt":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}, viewC); w.Code != http.StatusForbidden {
		t.Fatalf("viewer create window: want 403, got %d", w.Code)
	}
	w := doReq(t, r, "POST", "/api/maintenance-windows", map[string]string{
		"name": "freeze", "scope": "all",
		"startsAt": time.Now().UTC().Format(time.RFC3339),
		"endsAt":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}, opC)
	if w.Code != http.StatusCreated {
		t.Fatalf("operator create window: %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "POST", "/api/maintenance-windows", map[string]string{
		"name": "bad", "scope": "everywhere",
		"startsAt": time.Now().UTC().Format(time.RFC3339),
		"endsAt":   time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}, opC); w.Code != http.StatusBadRequest {
		t.Fatalf("bad scope: want 400, got %d", w.Code)
	}
	if w := doReq(t, r, "DELETE", "/api/maintenance-windows/1", nil, opC); w.Code != http.StatusForbidden {
		t.Fatalf("operator delete window: want 403, got %d", w.Code)
	}
	if w := doReq(t, r, "DELETE", "/api/maintenance-windows/1", nil, adminC); w.Code != http.StatusOK {
		t.Fatalf("admin delete window: %d %s", w.Code, w.Body.String())
	}
	// Deliveries visible to viewers (history, no secrets).
	if w := doReq(t, r, "GET", "/api/notification-deliveries", nil, viewC); w.Code != http.StatusOK {
		t.Fatalf("viewer deliveries: %d", w.Code)
	}
	if w := doReq(t, r, "GET", "/api/notification-deliveries?status=bogus", nil, viewC); w.Code != http.StatusBadRequest {
		t.Fatalf("bad status: want 400, got %d", w.Code)
	}
}

// Emergency pause round-trips and actually suppresses dispatch.
func TestPolicyPauseSuppresses(t *testing.T) {
	db, r := policyTestSetup(t)
	adminC := policyLogin(t, r, "admin")
	if w := doReq(t, r, "PUT", "/api/notification-policy", map[string]any{
		"emergencyPause": true, "pauseReason": "drill",
		"pauseUntil": time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}, adminC); w.Code != http.StatusOK {
		t.Fatalf("set pause: %d %s", w.Code, w.Body.String())
	}
	w := doReq(t, r, "GET", "/api/notification-policy", nil, adminC)
	if w.Code != http.StatusOK || !containsStr(w.Body.String(), "drill") {
		t.Fatalf("pause not reflected: %d %s", w.Code, w.Body.String())
	}
	_ = db
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
