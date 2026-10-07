package middleware

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"serverhub/internal/database"
)

// AgentAuth validates the Bearer token sent by a ServerHub Agent.
// On success it sets "agentServerId" (uint) in the context.
func AgentAuth(db *database.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		raw := strings.TrimPrefix(h, "Bearer ")
		if raw == "" || raw == h {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "agent token required"})
			return
		}
		sum := sha256.Sum256([]byte(raw))
		hash := hex.EncodeToString(sum[:])

		var tokenID, serverID uint
		var revoked bool
		err := db.QueryRow(
			`SELECT id, server_id, revoked FROM agent_tokens WHERE token_hash=$1`, hash,
		).Scan(&tokenID, &serverID, &revoked)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid agent token"})
			return
		}
		if revoked {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "agent token revoked"})
			return
		}
		// Update last_used_at (best-effort, non-blocking).
		go func() {
			_, _ = db.Exec(`UPDATE agent_tokens SET last_used_at=$1 WHERE id=$2`,
				time.Now().UTC(), tokenID)
		}()
		c.Set("agentServerId", serverID)
		c.Next()
	}
}

// AgentServerID extracts the server ID set by AgentAuth.
func AgentServerID(c *gin.Context) (uint, bool) {
	v, ok := c.Get("agentServerId")
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok
}
