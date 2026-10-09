package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/audit"
	"serverhub/internal/config"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
)

// Session lifetimes. Access tokens are short (30 minutes); long sessions
// come from rotating refresh tokens (7 days). See Refresh below.
const (
	accessTTL  = 30 * time.Minute
	refreshTTL = 7 * 24 * time.Hour
)

type AuthHandler struct {
	DB  *database.DB
	Cfg *config.Config
}

// setSessionCookies writes the access + refresh cookies. SameSite=Lax blocks
// cross-site cookie sends (CSRF) while keeping top-level navigation logins
// working; Secure follows COOKIE_SECURE (true behind the TLS proxy).
func setSessionCookies(c *gin.Context, cfg *config.Config, access, refresh string) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("serverhub_session", access, int(accessTTL.Seconds()), "/", cfg.CookieDomain, cfg.CookieSecure, true)
	c.SetCookie("serverhub_refresh", refresh, int(refreshTTL.Seconds()), "/", cfg.CookieDomain, cfg.CookieSecure, true)
}

func clearSessionCookies(c *gin.Context, cfg *config.Config) {
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("serverhub_session", "", -1, "/", cfg.CookieDomain, cfg.CookieSecure, true)
	c.SetCookie("serverhub_refresh", "", -1, "/", cfg.CookieDomain, cfg.CookieSecure, true)
}

func signAccess(cfg *config.Config, username, role string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  username,
		"role": role,
		"typ":  "access",
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(accessTTL).Unix(),
	})
	return token.SignedString([]byte(cfg.JWTSecret))
}

func hashRefresh(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func mintRefresh() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// storeRefresh persists the refresh token hash. Only the hash is stored so
// a database leak does not yield live sessions.
func (h *AuthHandler) storeRefresh(username, token string) error {
	_, err := h.DB.Exec(`INSERT INTO refresh_tokens (token_hash, username, expires_at, created_at, revoked)
		VALUES (?,?,?,CURRENT_TIMESTAMP,false)`,
		hashRefresh(token), username, time.Now().Add(refreshTTL).UTC())
	return err
}

// issuePair creates a fresh access + refresh pair for an authenticated user.
func (h *AuthHandler) issuePair(username, role string) (access, refresh string, err error) {
	access, err = signAccess(h.Cfg, username, role)
	if err != nil {
		return "", "", err
	}
	refresh, err = mintRefresh()
	if err != nil {
		return "", "", err
	}
	if err := h.storeRefresh(username, refresh); err != nil {
		return "", "", err
	}
	return access, refresh, nil
}

// revokeRefresh marks one refresh token revoked (idempotent).
func (h *AuthHandler) revokeRefresh(token string) {
	_, _ = h.DB.Exec(`UPDATE refresh_tokens SET revoked=true WHERE token_hash=?`, hashRefresh(token))
}

// revokeAllRefreshes revokes every refresh token for a user (used on
// password change and explicit logout-all).
func (h *AuthHandler) revokeAllRefreshes(username string) {
	_, _ = h.DB.Exec(`UPDATE refresh_tokens SET revoked=true WHERE username=?`, username)
}

// revokeAllRefreshesExceptRecent revokes every refresh token for a user
// EXCEPT ones created in the last `leeway` (DB clock). Used on the
// reuse/race paths: a concurrent-refresh loser must not nuke the winner's
// just-issued replacement, while a genuinely stolen old token still kills
// every other (older) session.
func (h *AuthHandler) revokeAllRefreshesExceptRecent(username string, leeway time.Duration) {
	secs := int(leeway.Seconds())
	if secs < 1 {
		secs = 1
	}
	_, _ = h.DB.Exec(`UPDATE refresh_tokens SET revoked=true WHERE username=?
		AND created_at < CURRENT_TIMESTAMP - make_interval(secs => ?)`,
		username, float64(secs))
}

// Login sets an HttpOnly session cookie pair for web clients.
func (h *AuthHandler) Login(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Username == "" || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}
	username, role, errMsg, status := h.verifyCredentials(body.Username, body.Password)
	if errMsg != "" {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	access, refresh, err := h.issuePair(username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}
	setSessionCookies(c, h.Cfg, access, refresh)
	audit.Write(h.DB, username, "login", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"username": username, "role": role, "token": access})
}

// Token is the mobile/API-client login endpoint that returns tokens in the
// response body instead of setting cookies. The client must store the
// refresh token securely (SecureStore/keychain) and rotate via /refresh.
func (h *AuthHandler) Token(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Username == "" || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}
	username, role, errMsg, status := h.verifyCredentials(body.Username, body.Password)
	if errMsg != "" {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	access, refresh, err := h.issuePair(username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}
	audit.Write(h.DB, username, "token", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"username": username, "role": role, "token": access, "refreshToken": refresh})
}

// Refresh rotates a refresh token into a fresh access + refresh pair.
// Accepts the `serverhub_refresh` cookie (web) or {"refreshToken": "..."}
// in the body (mobile/API clients). Rotation is mandatory: the presented
// token is revoked and a new one issued.
//
// Reuse detection: presenting an already-revoked token suggests theft, so
// every refresh token for that user is revoked and the caller gets 401.
func (h *AuthHandler) Refresh(c *gin.Context) {
	presented, err := c.Cookie("serverhub_refresh")
	if err != nil || presented == "" {
		var body struct {
			RefreshToken string `json:"refreshToken"`
		}
		_ = c.ShouldBindJSON(&body)
		presented = body.RefreshToken
	}
	if presented == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token required"})
		return
	}
	var username string
	var revoked bool
	var expires time.Time
	err = h.DB.QueryRow(`SELECT username, revoked, expires_at FROM refresh_tokens WHERE token_hash=?`,
		hashRefresh(presented)).Scan(&username, &revoked, &expires)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
		return
	}
	if revoked {
		// Possible token theft: kill sessions, sparing tokens minted in
		// the last 30s so a concurrent-refresh loser cannot nuke the
		// winner's just-issued replacement (see the atomic claim below).
		h.revokeAllRefreshesExceptRecent(username, 30*time.Second)
		audit.Write(h.DB, username, "refresh-reuse", "auth", "", "failed", "revoked token reused; sessions revoked")
		clearSessionCookies(c, h.Cfg)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
		return
	}
	if time.Now().After(expires) {
		_, _ = h.DB.Exec(`DELETE FROM refresh_tokens WHERE token_hash=?`, hashRefresh(presented))
		clearSessionCookies(c, h.Cfg)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "session expired"})
		return
	}
	var role string
	if err := h.DB.QueryRow(`SELECT role FROM users WHERE username=?`, username).Scan(&role); err != nil {
		h.revokeRefresh(presented)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
		return
	}
	// Rotate with an atomic claim: the UPDATE only matches a still-valid
	// token, so two concurrent requests with the same token cannot both
	// win — the loser sees zero rows affected and is treated as reuse.
	res, err := h.DB.Exec(`UPDATE refresh_tokens SET revoked=true WHERE token_hash=? AND revoked=false`,
		hashRefresh(presented))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not refresh session"})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// Lost a concurrent race (or DB state changed under us): fail
		// closed like a revoked-token reuse, but spare just-minted
		// replacements so the winner's session survives.
		h.revokeAllRefreshesExceptRecent(username, 30*time.Second)
		audit.Write(h.DB, username, "refresh-reuse", "auth", "", "failed", "concurrent refresh race; sessions revoked")
		clearSessionCookies(c, h.Cfg)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
		return
	}
	access, refresh, err := h.issuePair(username, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not refresh session"})
		return
	}
	setSessionCookies(c, h.Cfg, access, refresh)
	audit.Write(h.DB, username, "refresh", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"username": username, "role": role, "token": access, "refreshToken": refresh})
}

// verifyCredentials checks the password and returns the identity. It never
// mints tokens — callers decide which session type to issue.
func (h *AuthHandler) verifyCredentials(username, password string) (user, role, errMsg string, status int) {
	var id int64
	var hash string
	err := h.DB.QueryRow(`SELECT id, username, password_hash, role FROM users WHERE username=?`, username).
		Scan(&id, &user, &hash, &role)
	if err != nil {
		audit.Write(h.DB, username, "login", "auth", "", "failed", "invalid username")
		return "", "", "invalid credentials", http.StatusUnauthorized
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		audit.Write(h.DB, username, "login", "auth", "", "failed", "invalid password")
		return "", "", "invalid credentials", http.StatusUnauthorized
	}
	return user, role, "", http.StatusOK
}

func (h *AuthHandler) Logout(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	refresh, err := c.Cookie("serverhub_refresh")
	if err != nil || refresh == "" {
		// Mobile/API clients carry the refresh token in the body, not a
		// cookie — without this, their logout revokes nothing.
		var body struct {
			RefreshToken string `json:"refreshToken"`
		}
		_ = c.ShouldBindJSON(&body)
		refresh = body.RefreshToken
	}
	if refresh != "" {
		h.revokeRefresh(refresh)
	}
	clearSessionCookies(c, h.Cfg)
	audit.Write(h.DB, u, "logout", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// LogoutAll revokes every refresh token for the caller (all devices) and
// clears this session's cookies.
func (h *AuthHandler) LogoutAll(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	h.revokeAllRefreshes(u)
	clearSessionCookies(c, h.Cfg)
	audit.Write(h.DB, u, "logout-all", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) Me(c *gin.Context) {
	u, r := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{"username": u, "role": r})
}

// ChangePassword updates the current user's password. Requires the current
// password (re-authentication for a sensitive operation) and revokes every
// refresh token so existing sessions (including stolen ones) die.
func (h *AuthHandler) ChangePassword(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	var body struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.CurrentPassword == "" || body.NewPassword == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "currentPassword and newPassword are required"})
		return
	}
	if len(body.NewPassword) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "new password must be at least 8 characters"})
		return
	}
	var hash string
	if err := h.DB.QueryRow(`SELECT password_hash FROM users WHERE username=?`, u).Scan(&hash); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.CurrentPassword)); err != nil {
		audit.Write(h.DB, u, "change-password", "auth", "", "failed", "wrong current password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "current password is incorrect"})
		return
	}
	newHash, err := bcrypt.GenerateFromPassword([]byte(body.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET password_hash=? WHERE username=?`, string(newHash), u); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not update password"})
		return
	}
	h.revokeAllRefreshes(u)
	audit.Write(h.DB, u, "change-password", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
