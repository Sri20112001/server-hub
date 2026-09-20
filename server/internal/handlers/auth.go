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

func (h *AuthHandler) Login(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Username == "" || body.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username and password required"})
		return
	}
	var id int64
	var username, hash, role string
	err := h.DB.QueryRow(`SELECT id, username, password_hash, role FROM users WHERE username=?`, body.Username).
		Scan(&id, &username, &hash, &role)
	if err != nil {
		audit.Write(h.DB, body.Username, "login", "auth", "", "failed", "invalid username")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)); err != nil {
		audit.Write(h.DB, body.Username, "login", "auth", "", "failed", "invalid password")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":  username,
		"role": role,
		"exp":  time.Now().Add(24 * time.Hour).Unix(),
	})
	signed, err := token.SignedString([]byte(h.Cfg.JWTSecret))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not create session"})
		return
	}
	c.SetCookie("serverhub_session", signed, 86400, "/", h.Cfg.CookieDomain, h.Cfg.CookieSecure, true)
	audit.Write(h.DB, username, "login", "auth", "", "ok", "")
	c.JSON(http.StatusOK, gin.H{"username": username, "role": role, "token": signed})
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
