package handlers

// Regression tests for the Phase 2 privileged-operation audit fixes:
// tail clamping, webhook body bound, exec-token log skip (middleware test),
// logout revoking body-carried refresh tokens, atomic refresh rotation, and
// backup download serving single-file archives.

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"serverhub/internal/config"
	"serverhub/internal/middleware"
	"serverhub/internal/testdb"
)

func TestParseTailParam(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", "200"},
		{"200", "200"},
		{"1", "1"},
		{"5000", "5000"},
		{"999999999", "5000"}, // unclamped tail pins memory via io.ReadAll
		{"-5", "200"},
		{"0", "200"},
		{"abc", "200"},
		{" 300 ", "300"},
	}
	for _, tc := range cases {
		if got := parseTailParam(tc.in); got != tc.want {
			t.Fatalf("parseTailParam(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestGitHubWebhookOversizedBodyRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := &config.Config{WebhookSecret: "audit-test-secret"}
	h := &WebhookHandler{DB: nil, Cfg: cfg}
	r := gin.New()
	r.POST("/webhooks/github", h.GitHub)

	// 2 MiB body with a signature that is VALID for the full bytes. With
	// the 1 MiB read bound the handler verifies against truncated bytes,
	// so the signature must fail instead of reaching deploy logic.
	big := strings.Repeat("a", 2<<20)
	sig := SignBody(cfg.WebhookSecret, []byte(big))
	req := httptest.NewRequest("POST", "/webhooks/github", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Hub-Signature-256", sig)
	req.Header.Set("X-GitHub-Event", "push")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("oversized webhook body: want 401, got %d %s", w.Code, w.Body.String())
	}
}

func TestDownloadServesSingleFileArchive(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := testdb.Open(t)
	pid, err := db.InsertID(`INSERT INTO projects (name) VALUES ('dl-proj')`)
	if err != nil {
		t.Fatal(err)
	}
	// Stored backups are single .tar.gz FILES, not directories.
	archive := filepath.Join(t.TempDir(), "snap.tar.gz")
	if err := os.WriteFile(archive, []byte("fake-tar-gzip-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	bid, err := db.InsertID(`INSERT INTO backups (project_id,kind,path,size_bytes,status) VALUES (?,?,?,?,?)`,
		pid, "snapshot", archive, 20, "SUCCESS")
	if err != nil {
		t.Fatal(err)
	}
	h := &BackupsHandler{DB: db, Dir: t.TempDir()}
	r := gin.New()
	r.GET("/backups/:id/download", h.Download)
	req := httptest.NewRequest("GET", "/backups/"+itoaInt(bid)+"/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("download: %d %s", w.Code, w.Body.String())
	}
	zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
	if err != nil {
		t.Fatalf("download is not a valid zip: %v", err)
	}
	if len(zr.File) != 1 {
		t.Fatalf("download zip has %d entries, want 1 (was empty before the fix)", len(zr.File))
	}
}

func TestLogoutRevokesBodyRefreshToken(t *testing.T) {
	db, cfg, r := testSetupSecure(t)
	authH := &AuthHandler{DB: db, Cfg: cfg}
	// testSetupSecure lacks logout-all routes; register plain logout + refresh.
	r.POST("/api/auth/logout-body", middleware.AuthRequired(cfg.JWTSecret), authH.Logout)

	cookies := loginAs(t, r, "admin", "testpass123")
	var sess, refr *http.Cookie
	for _, ck := range cookies {
		switch ck.Name {
		case "serverhub_session":
			sess = ck
		case "serverhub_refresh":
			refr = ck
		}
	}
	if sess == nil || refr == nil {
		t.Fatal("login did not set session cookies")
	}
	// Mobile-style logout: session cookie for auth, refresh token in body.
	w := doReq(t, r, "POST", "/api/auth/logout-body",
		map[string]string{"refreshToken": refr.Value}, []*http.Cookie{sess})
	if w.Code != http.StatusOK {
		t.Fatalf("logout: %d %s", w.Code, w.Body.String())
	}
	// The body-carried refresh token must now be dead.
	if w := doReq(t, r, "POST", "/api/auth/refresh",
		map[string]string{"refreshToken": refr.Value}, nil); w.Code != http.StatusUnauthorized {
		t.Fatalf("refresh after body logout: want 401, got %d", w.Code)
	}
}

// The 30s race leeway must ONLY soften rotation-race recovery: explicit
// security revocations (logout-all, password change) kill even fresh tokens.
func TestLogoutAllKillsFreshTokens(t *testing.T) {
	db, cfg, r := testSetupSecure(t)
	authH := &AuthHandler{DB: db, Cfg: cfg}
	r.POST("/api/auth/logout-all-fresh", middleware.AuthRequired(cfg.JWTSecret), authH.LogoutAll)

	cookies := loginAs(t, r, "admin", "testpass123")
	sess := cookieByName(cookies, "serverhub_session")
	refr := cookieByName(cookies, "serverhub_refresh")
	w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{refr})
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: %d", w.Code)
	}
	fresh := cookieByName(w.Result().Cookies(), "serverhub_refresh")
	if w := doReq(t, r, "POST", "/api/auth/logout-all-fresh", nil, []*http.Cookie{sess}); w.Code != http.StatusOK {
		t.Fatalf("logout-all: %d", w.Code)
	}
	if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{fresh}); w.Code != http.StatusUnauthorized {
		t.Fatalf("fresh token after logout-all: want 401, got %d", w.Code)
	}
}

func TestPasswordChangeKillsFreshTokens(t *testing.T) {
	db, cfg, r := testSetupSecure(t)
	authH := &AuthHandler{DB: db, Cfg: cfg}
	r.PUT("/api/auth/password-fresh", middleware.AuthRequired(cfg.JWTSecret), authH.ChangePassword)

	cookies := loginAs(t, r, "admin", "testpass123")
	sess := cookieByName(cookies, "serverhub_session")
	refr := cookieByName(cookies, "serverhub_refresh")
	w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{refr})
	if w.Code != http.StatusOK {
		t.Fatalf("rotate: %d", w.Code)
	}
	fresh := cookieByName(w.Result().Cookies(), "serverhub_refresh")
	w = doReq(t, r, "PUT", "/api/auth/password-fresh",
		map[string]string{"currentPassword": "testpass123", "newPassword": "newpass123456"}, []*http.Cookie{sess})
	if w.Code != http.StatusOK {
		t.Fatalf("change password: %d %s", w.Code, w.Body.String())
	}
	if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{fresh}); w.Code != http.StatusUnauthorized {
		t.Fatalf("fresh token after password change: want 401, got %d", w.Code)
	}
}

// Boundary behavior of the 30s leeway, using the DB clock on both sides
// (backdate + comparison both use CURRENT_TIMESTAMP: timezone-safe).
func TestLeewayBoundary(t *testing.T) {
	for _, tc := range []struct {
		ageSec int
		want   int
	}{
		{29, http.StatusOK},
		{31, http.StatusUnauthorized},
	} {
		db, _, r := testSetupSecure(t)
		cookies := loginAs(t, r, "admin", "testpass123")
		r1 := cookieByName(cookies, "serverhub_refresh")
		w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r1})
		if w.Code != http.StatusOK {
			t.Fatal("rotate")
		}
		r2 := cookieByName(w.Result().Cookies(), "serverhub_refresh")
		w = doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r2})
		if w.Code != http.StatusOK {
			t.Fatal("rotate r2")
		}
		r3 := cookieByName(w.Result().Cookies(), "serverhub_refresh")
		if _, err := db.Exec(`UPDATE refresh_tokens SET created_at = CURRENT_TIMESTAMP - make_interval(secs => ?) WHERE token_hash=?`,
			float64(tc.ageSec), hashRefresh(r3.Value)); err != nil {
			t.Fatal(err)
		}
		// Reuse the now-revoked r2: triggers ExceptRecent revocation.
		if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r2}); w.Code != http.StatusUnauthorized {
			t.Fatalf("reuse: want 401, got %d", w.Code)
		}
		if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{r3}); w.Code != tc.want {
			t.Fatalf("age %ds: want %d, got %d", tc.ageSec, tc.want, w.Code)
		}
	}
}

func TestConcurrentRefreshSingleWinner(t *testing.T) {
	_, _, r := testSetupSecure(t)
	cookies := loginAs(t, r, "admin", "testpass123")
	var refr *http.Cookie
	for _, ck := range cookies {
		if ck.Name == "serverhub_refresh" {
			refr = ck
		}
	}
	if refr == nil {
		t.Fatal("no refresh cookie")
	}
	// Fire two refreshes with the same token concurrently. The atomic
	// claim (UPDATE ... WHERE revoked=false) must let at most one win;
	// the loser is treated as reuse — but the 30s race leeway must spare
	// the winner's just-issued replacement.
	var wg sync.WaitGroup
	recs := make([]*httptest.ResponseRecorder, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			recs[i] = doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{refr})
		}(i)
	}
	wg.Wait()
	ok, unauth := 0, 0
	var winner *httptest.ResponseRecorder
	for _, rec := range recs {
		switch rec.Code {
		case http.StatusOK:
			ok++
			winner = rec
		case http.StatusUnauthorized:
			unauth++
		default:
			t.Fatalf("unexpected refresh code %d", rec.Code)
		}
	}
	if ok > 1 {
		t.Fatalf("both concurrent refreshes succeeded — rotation race is open")
	}
	if ok+unauth != 2 {
		t.Fatalf("want one winner at most, got ok=%d unauth=%d", ok, unauth)
	}
	if ok == 1 {
		// The winner's replacement must still be usable: the loser's
		// revoke-all (with race leeway) must not have nuked it.
		next := cookieByName(winner.Result().Cookies(), "serverhub_refresh")
		if next == nil {
			t.Fatal("winner issued no replacement refresh cookie")
		}
		if w := doReq(t, r, "POST", "/api/auth/refresh", nil, []*http.Cookie{next}); w.Code != http.StatusOK {
			t.Fatalf("winner replacement dead: want 200, got %d", w.Code)
		}
	}
}
