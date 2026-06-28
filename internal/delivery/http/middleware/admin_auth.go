package middleware

import (
	"strings"

	"github.com/labstack/echo/v4"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/pkg/apperror"
	"wanpey/core/pkg/jwtutil"
)

const (
	// ContextKeyAdminID / ContextKeyAdminRole are set by AdminJWTAuth.
	ContextKeyAdminID   = "admin_id"
	ContextKeyAdminRole = "admin_role"
)

// AdminJWTAuth validates the Authorization: Bearer <token> header for admin routes.
// Only access tokens are accepted (refresh tokens are rejected).
func AdminJWTAuth(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw := c.Request().Header.Get("Authorization")
			token, ok := strings.CutPrefix(raw, "Bearer ")
			if !ok || token == "" {
				return apperror.Unauthorized("missing bearer token")
			}
			claims, err := jwtutil.Verify(secret, token)
			if err != nil {
				return apperror.Unauthorized("invalid token")
			}
			if claims.Type != jwtutil.TokenTypeAccess {
				return apperror.Unauthorized("not an access token")
			}
			c.Set(ContextKeyAdminID, claims.Sub)
			c.Set(ContextKeyAdminRole, claims.Role)
			return next(c)
		}
	}
}

// RequireRole restricts a route to the given admin roles. Must run after AdminJWTAuth.
func RequireRole(roles ...entity.AdminRole) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			role, ok := c.Get(ContextKeyAdminRole).(string)
			if !ok {
				return apperror.Forbidden("insufficient role")
			}
			for _, r := range roles {
				if string(r) == role {
					return next(c)
				}
			}
			return apperror.Forbidden("insufficient role")
		}
	}
}
