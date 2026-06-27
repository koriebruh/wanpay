package jwtutil

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// TokenType distinguishes access tokens from refresh tokens.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// Claims is the JWT payload for admin sessions.
type Claims struct {
	Sub      string    `json:"sub"`
	Username string    `json:"username"`
	Role     string    `json:"role"`
	Type     TokenType `json:"type"`
	Exp      int64     `json:"exp"`
	Iat      int64     `json:"iat"`
}

var jwtHeader = base64url(`{"alg":"HS256","typ":"JWT"}`)

// Generate creates a signed HS256 JWT.
func Generate(secret string, claims Claims) string {
	claims.Iat = time.Now().Unix()
	payload := base64url(mustJSON(claims))
	sig := sign(secret, jwtHeader+"."+payload)
	return jwtHeader + "." + payload + "." + sig
}

// Verify parses and validates a JWT, returning its claims on success.
func Verify(secret, token string) (*Claims, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid token format")
	}

	expectedSig := sign(secret, parts[0]+"."+parts[1])
	if !hmac.Equal([]byte(expectedSig), []byte(parts[2])) {
		return nil, fmt.Errorf("invalid signature")
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid token payload encoding")
	}

	var c Claims
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("invalid token payload")
	}

	if time.Now().Unix() > c.Exp {
		return nil, fmt.Errorf("token expired")
	}

	return &c, nil
}

func sign(secret, data string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}

func base64url(s string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(s))
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v) //nolint:errcheck // marshalling known-safe struct
	return string(b)
}
