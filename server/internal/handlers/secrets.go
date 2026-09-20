package handlers

import (
	"database/sql"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"serverhub/internal/audit"
	"serverhub/internal/config"
	"serverhub/internal/crypto"
	"serverhub/internal/database"
	"serverhub/internal/middleware"
	"serverhub/internal/models"
)

type SecretHandler struct {
	DB  *database.DB
	Cfg *config.Config
}

func encKey(h *SecretHandler) ([]byte, error) {
	return crypto.KeyFromHex(h.Cfg.EncryptionKey)
}

// List returns metadata only — never values.
func (h *SecretHandler) List(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	env := c.DefaultQuery("environment", "")
	q := `SELECT id,project_id,name,environment,service_id,updated_at FROM secrets WHERE project_id=?`
	args := []interface{}{pid}
	if env != "" {
		q += ` AND environment=?`
		args = append(args, env)
	}
	q += ` ORDER BY name`
	rows, err := h.DB.Query(q, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []models.SecretMeta{}
	for rows.Next() {
		var m models.SecretMeta
		var svc sql.NullInt64
		var updated sql.NullString
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Name, &m.Environment, &svc, &updated); err == nil {
			if svc.Valid {
				v := svc.Int64
				m.ServiceID = &v
			}
			m.Configured = true
			m.UpdatedAt = nullStr(updated)
			out = append(out, m)
		}
	}
	c.JSON(http.StatusOK, out)
}

func (h *SecretHandler) Upsert(c *gin.Context) {
	pid, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid project id"})
		return
	}
	var body struct {
		Name        string `json:"name"`
		Value       string `json:"value"`
		Environment string `json:"environment"`
		ServiceID   *int64 `json:"serviceId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Name == "" || body.Value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and value are required"})
		return
	}
	if body.Environment == "" {
		body.Environment = "production"
	}
	key, err := encKey(h)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server encryption misconfigured"})
		return
	}
	ct, nonce, err := crypto.Encrypt(key, body.Value)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encryption failed"})
		return
	}
	var svc interface{}
	if body.ServiceID != nil {
		svc = *body.ServiceID
	}
	_, err = h.DB.Exec(`INSERT INTO secrets (project_id,name,environment,service_id,encrypted_value,nonce,updated_at)
		VALUES (?,?,?,?,?,?,CURRENT_TIMESTAMP)
		ON CONFLICT(project_id, environment, name) DO UPDATE SET encrypted_value=excluded.encrypted_value, nonce=excluded.nonce, service_id=excluded.service_id, updated_at=CURRENT_TIMESTAMP`,
		pid, body.Name, body.Environment, svc, ct, nonce)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "upsert-secret", "secret", body.Name, "ok", "project="+strconv.FormatInt(pid, 10))
	c.JSON(http.StatusOK, gin.H{"name": body.Name, "environment": body.Environment, "configured": true})
}

// Update rotates the value of an existing secret by id.
// Only the value (and optionally environment/service assignment) changes;
// the name is immutable to keep audit trails stable.
func (h *SecretHandler) Update(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var body struct {
		Value       string `json:"value"`
		Environment string `json:"environment"`
		ServiceID   *int64 `json:"serviceId"`
	}
	if err := c.ShouldBindJSON(&body); err != nil || body.Value == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "value is required"})
		return
	}
	var name, env string
	var pid int64
	if err := h.DB.QueryRow(`SELECT project_id, name, environment FROM secrets WHERE id=?`, id).Scan(&pid, &name, &env); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	if body.Environment != "" {
		env = body.Environment
	}
	key, err := encKey(h)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server encryption misconfigured"})
		return
	}
	ct, nonce, err := crypto.Encrypt(key, body.Value)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "encryption failed"})
		return
	}
	var svc interface{}
	if body.ServiceID != nil {
		svc = *body.ServiceID
	}
	_, err = h.DB.Exec(`UPDATE secrets SET encrypted_value=?, nonce=?, environment=?, service_id=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		ct, nonce, env, svc, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "rotate-secret", "secret", strconv.FormatInt(id, 10), "ok", "project="+strconv.FormatInt(pid, 10)+" name="+name)
	c.JSON(http.StatusOK, gin.H{"name": name, "environment": env, "configured": true})
}

func (h *SecretHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	res, err := h.DB.Exec(`DELETE FROM secrets WHERE id=?`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "delete-secret", "secret", strconv.FormatInt(id, 10), "ok", "")
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// Reveal explicitly returns the plaintext value. This is the ONLY endpoint
// that does so, and it is audit-logged.
func (h *SecretHandler) Reveal(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var name, ct, nonce string
	var pid int64
	err = h.DB.QueryRow(`SELECT project_id,name,encrypted_value,nonce FROM secrets WHERE id=?`, id).
		Scan(&pid, &name, &ct, &nonce)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "secret not found"})
		return
	}
	key, err := encKey(h)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "server encryption misconfigured"})
		return
	}
	pt, err := crypto.Decrypt(key, ct, nonce)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decryption failed"})
		return
	}
	u, _ := middleware.CurrentUser(c)
	audit.Write(h.DB, u, "reveal-secret", "secret", strconv.FormatInt(id, 10), "ok", "project="+strconv.FormatInt(pid, 10)+" name="+name)
	c.JSON(http.StatusOK, gin.H{"name": name, "value": pt})
}
