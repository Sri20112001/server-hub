package handlers

import (
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

type AuthHandler struct {
	DB  *database.DB
	Cfg *config.Config
}

// Login sets an HttpOnly session cookie for web clients.
func (h *AuthHandler) Login(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Username == "" || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}
	username, role, signed, status, errMsg := h.authenticate(body.Username, body.Password)
	if errMsg != "" {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	c.SetCookie("serverhub_session", signed, 86400, "/", h.Cfg.CookieDomain, h.Cfg.CookieSecure, true)
	audit.Write(h.DB, username, "login", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"username": username, "role": role})
}

// Token is the mobile/API-client login endpoint that returns a Bearer token
// in the response body instead of setting a cookie.
func (h *AuthHandler) Token(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Username == "" || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}
	username, role, signed, status, errMsg := h.authenticate(body.Username, body.Password)
	if errMsg != "" {
		c.JSON(status, gin.H{"error": errMsg})
		return
	}
	audit.Write(h.DB, username, "token", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"username": username, "role": role, "token": signed})
}

// authenticate validates credentials and returns a signed JWT.
func (h *AuthHandler) authenticate(username, password string) (user, role, signed string, status int, errMsg string) {
	var id int64
	var hash string
	err := h.DB.QueryRow(`SELECT id, username, password_hash, role FROM users WHERE username=?`, username).
		Scan(&id, &user, &hash, &role)
	if err != nil {
		audit.Write(h.DB, username, "login", "auth", "", "failed", "invalid username")
		return "", "", "", http.StatusUnauthorized, "invalid credentials"
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); err != nil {
		audit.Write(h.DB, username, "login", "auth", "", "failed", "invalid password")
		return "", "", "", http.StatusUnauthorized, "invalid credentials"
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  user,
		"role": role,
		"iat":  time.Now().Unix(),
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
	})
	s, err := token.SignedString([]byte(h.Cfg.JWTSecret))
	if err != nil {
		return "", "", "", http.StatusInternalServerError, "could not create session"
	}
	return user, role, s, http.StatusOK, ""
}

func (h *AuthHandler) Logout(c *gin.Context) {
	u, _ := middleware.CurrentUser(c)
	c.SetCookie("serverhub_session", "", -1, "/", h.Cfg.CookieDomain, h.Cfg.CookieSecure, true)
	audit.Write(h.DB, u, "logout", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AuthHandler) Me(c *gin.Context) {
	u, r := middleware.CurrentUser(c)
	c.JSON(http.StatusOK, gin.H{"username": u, "role": r})
}

// ChangePassword updates the current user's password. Requires the current
// password (re-authentication for a sensitive operation).
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
	audit.Write(h.DB, u, "change-password", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
