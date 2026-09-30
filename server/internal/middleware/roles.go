package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Role-based access control.
//
// Roles form a hierarchy: viewer < operator < admin.
//
//	viewer   — read-only: dashboards, telemetry, logs, metadata.
//	operator — viewer + routine mutations: projects/services, deploys,
//	           backups, secret values (upsert), discovery imports.
//	admin    — operator + destructive & privileged operations: container
//	           stop/restart, exec shells, secret rotate/delete/reveal,
//	           notification settings, user management.
//
// Unknown or empty roles are denied everywhere (fail closed).
const (
	RoleViewer   = "viewer"
	RoleOperator = "operator"
	RoleAdmin    = "admin"
)

func roleLevel(role string) int {
	switch role {
	case RoleAdmin:
		return 3
	case RoleOperator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}

// ValidRole reports whether r is a known role name.
func ValidRole(r string) bool {
	return r == RoleViewer || r == RoleOperator || r == RoleAdmin
}

// RequireRole denies the request with 403 unless the caller's role (set by
// AuthRequired) meets the minimum level. Must be chained AFTER AuthRequired.
func RequireRole(minRole string) gin.HandlerFunc {
	min := roleLevel(minRole)
	return func(c *gin.Context) {
		_, role := CurrentUser(c)
		if roleLevel(role) < min {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "forbidden: requires " + minRole + " role"})
			return
		}
		c.Next()
	}
}
