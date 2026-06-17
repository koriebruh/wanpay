package middleware

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"wanpey/core/internal/domain/repository"
	"wanpey/core/pkg/apperror"
)

const (
	apiKeyHeader = "X-API-Key"

	// ContextKeyMerchantID is the Echo context key set by APIKeyAuth.
	// Downstream middleware (Idempotency) and handlers read this key.
	ContextKeyMerchantID = "merchant_id"
)

// APIKeyAuth authenticates every inbound request using the X-API-Key header.
//
// Flow:
//  1. Read X-API-Key header — 401 if missing.
//  2. SHA256-hash the raw key.
//  3. Look up merchant by hash — 401 if not found or any DB error (avoids key enumeration).
//  4. Check merchant.CanTransact() — 403 if account is not active/suspended.
//  5. Set "merchant_id" in Echo context for downstream use.
func APIKeyAuth(repo repository.MerchantRepository) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			raw := c.Request().Header.Get(apiKeyHeader)
			if raw == "" {
				return apperror.Unauthorized("missing %s header", apiKeyHeader)
			}

			hashed := hashAPIKey(raw)
			merchant, err := repo.FindByAPIKey(c.Request().Context(), hashed)
			if err != nil {
				// Log unexpected errors (DB down, etc.) without exposing them to the caller.
				// Both "not found" and infra errors map to 401 — never distinguish the two
				// to prevent key enumeration attacks.
				zap.L().Debug("auth: FindByAPIKey failed", zap.Error(err))
				return apperror.Unauthorized("invalid API key")
			}

			if !merchant.CanTransact() {
				return apperror.Forbidden("merchant account is %s", merchant.Status)
			}

			c.Set(ContextKeyMerchantID, merchant.ID)
			return next(c)
		}
	}
}

// hashAPIKey returns the lowercase hex-encoded SHA256 hash of the raw API key.
// This matches how APIKey is stored in the DB (SHA256 hash, never plain text).
func hashAPIKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
