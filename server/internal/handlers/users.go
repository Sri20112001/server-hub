package handlers

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"

	"serverhub/internal/audit"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
)

// User management (admin only). Roles: viewer < operator < admin (see
// middleware roles). Guards: usernames are validated, passwords require 8+
// chars, and the last admin account can never be demoted or deleted.

type UserHandler struct {
	DB *database.DB
}

var usernameRe = regexp.MustCompile(`^[a-zA-Z0-9._-]{2,64}$`)

func validUsername(u string) bool {
	return usernameRe.MatchString(u)
}

func (h *UserHandler) adminCount() int {
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE role='admin'`).Scan(&n)
	return n
}

type userView struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"createdAt"`
}

// GET /server-hub/api/users
func (h *UserHandler) List(c *gin.Context) {
	rows, err := h.DB.Query(`SELECT username, role, created_at FROM users ORDER BY username`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []userView{}
	for rows.Next() {
		var u userView
		var created sql.NullString
		if err := rows.Scan(&u.Username, &u.Role, &created); err == nil {
			u.CreatedAt = nullStr(created)
			out = append(out, u)
		}
	}
	c.JSON(http.StatusOK, out)
}

// POST /server-hub/api/users {username, password, role}
func (h *UserHandler) Create(c *gin.Context) {
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "username, password and role are required"})
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if !validUsername(body.Username) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid username (2-64 chars: letters, digits, . _ -)"})
		return
	}
	if len(body.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}
	if !middleware.ValidRole(body.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be viewer, operator or admin"})
		return
	}
	var exists int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM users WHERE username=?`, body.Username).Scan(&exists)
	if exists > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "user already exists"})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(body.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not hash password"})
		return
	}
	if _, err := h.DB.Exec(`INSERT INTO users (username, password_hash, role) VALUES (?,?,?)`,
		body.Username, string(hash), body.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "create-user", "user", body.Username, "ok", "role="+body.Role)
	c.JSON(http.StatusCreated, gin.H{"username": body.Username, "role": body.Role})
}

// PUT /server-hub/api/users/:username/role {role}
func (h *UserHandler) SetRole(c *gin.Context) {
	target := c.Param("username")
	var body struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || !middleware.ValidRole(body.Role) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role must be viewer, operator or admin"})
		return
	}
	var current string
	if err := h.DB.QueryRow(`SELECT role FROM users WHERE username=?`, target).Scan(&current); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	self, _ := middleware.CurrentUser(c)
	if target == self && body.Role != current {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot change your own role"})
		return
	}
	if current == "admin" && body.Role != "admin" && h.adminCount() <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot demote the last admin"})
		return
	}
	if _, err := h.DB.Exec(`UPDATE users SET role=? WHERE username=?`, body.Role, target); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// A demotion must kill live sessions immediately; access tokens expire
	// within 30 minutes on their own, refresh tokens are revoked here.
	if body.Role != current {
		_, _ = h.DB.Exec(`UPDATE refresh_tokens SET revoked=true WHERE username=?`, target)
	}
	audit.Write(h.DB, self, "set-role", "user", target, "ok", current+"->"+body.Role)
	c.JSON(http.StatusOK, gin.H{"username": target, "role": body.Role})
}

// DELETE /server-hub/api/users/:username
func (h *UserHandler) Delete(c *gin.Context) {
	target := c.Param("username")
	var current string
	if err := h.DB.QueryRow(`SELECT role FROM users WHERE username=?`, target).Scan(&current); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	self, _ := middleware.CurrentUser(c)
	if target == self {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete your own account"})
		return
	}
	if current == "admin" && h.adminCount() <= 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete the last admin"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM users WHERE username=?`, target)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	_, _ = h.DB.Exec(`UPDATE refresh_tokens SET revoked=true WHERE username=?`, target)
	audit.Write(h.DB, self, "delete-user", "user", target, "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
