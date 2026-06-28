//go:build !integration

package jwtutil

import (
	"strings"
	"testing"
	"time"
)

const testSecret = "test-jwt-secret-for-unit-tests-32chars"

func makeClaims(ttl time.Duration, typ TokenType) Claims {
	return Claims{
		Sub:   "admin-id-123",
		Email: "admin@wanpey.id",
		Role:  "super_admin",
		Type:  typ,
		Exp:   time.Now().Add(ttl).Unix(),
	}
}

func TestGenerate_Verify_RoundTrip(t *testing.T) {
	claims := makeClaims(time.Hour, TokenTypeAccess)
	token := Generate(testSecret, claims)

	if token == "" {
		t.Fatal("Generate returned empty token")
	}
	if len(strings.Split(token, ".")) != 3 {
		t.Fatalf("token not 3-part JWT: %q", token)
	}

	got, err := Verify(testSecret, token)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if got.Sub != claims.Sub {
		t.Errorf("Sub = %q, want %q", got.Sub, claims.Sub)
	}
	if got.Email != claims.Email {
		t.Errorf("Email = %q, want %q", got.Email, claims.Email)
	}
	if got.Role != claims.Role {
		t.Errorf("Role = %q, want %q", got.Role, claims.Role)
	}
	if got.Type != TokenTypeAccess {
		t.Errorf("Type = %q, want access", got.Type)
	}
}

func TestVerify_WrongSecret(t *testing.T) {
	token := Generate(testSecret, makeClaims(time.Hour, TokenTypeAccess))
	_, err := Verify("wrong-secret", token)
	if err == nil {
		t.Error("Verify should fail with wrong secret")
	}
}

func TestVerify_ExpiredToken(t *testing.T) {
	claims := makeClaims(-time.Second, TokenTypeAccess) // expired 1s ago
	token := Generate(testSecret, claims)
	_, err := Verify(testSecret, token)
	if err == nil {
		t.Error("Verify should fail for expired token")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error should mention 'expired', got: %v", err)
	}
}

func TestVerify_InvalidFormat_Onepart(t *testing.T) {
	_, err := Verify(testSecret, "notavalidtoken")
	if err == nil {
		t.Error("Verify should fail for non-JWT string")
	}
}

func TestVerify_InvalidFormat_TwoParts(t *testing.T) {
	_, err := Verify(testSecret, "header.payload")
	if err == nil {
		t.Error("Verify should fail for 2-part token")
	}
}

func TestVerify_TamperedPayload(t *testing.T) {
	token := Generate(testSecret, makeClaims(time.Hour, TokenTypeAccess))
	parts := strings.Split(token, ".")
	// Tamper the payload (replace with a different base64 string)
	parts[1] = base64url(`{"sub":"hacker","email":"h@x.com","role":"super_admin","type":"access","exp":9999999999,"iat":0}`)
	tampered := strings.Join(parts, ".")
	_, err := Verify(testSecret, tampered)
	if err == nil {
		t.Error("Verify should fail for tampered payload")
	}
}

func TestVerify_TamperedSignature(t *testing.T) {
	token := Generate(testSecret, makeClaims(time.Hour, TokenTypeAccess))
	// Append garbage to the signature part
	_, err := Verify(testSecret, token+"garbage")
	if err == nil {
		t.Error("Verify should fail for tampered signature")
	}
}

func TestRefreshToken_TypePreserved(t *testing.T) {
	claims := makeClaims(7*24*time.Hour, TokenTypeRefresh)
	token := Generate(testSecret, claims)

	got, err := Verify(testSecret, token)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if got.Type != TokenTypeRefresh {
		t.Errorf("Type = %q, want refresh", got.Type)
	}
}

func TestIat_SetAutomatically(t *testing.T) {
	before := time.Now().Unix()
	claims := makeClaims(time.Hour, TokenTypeAccess)
	token := Generate(testSecret, claims)
	after := time.Now().Unix()

	got, err := Verify(testSecret, token)
	if err != nil {
		t.Fatalf("Verify failed: %v", err)
	}
	if got.Iat < before || got.Iat > after {
		t.Errorf("Iat = %d, want between %d and %d", got.Iat, before, after)
	}
}

func TestGenerate_DifferentSecrets_DifferentTokens(t *testing.T) {
	claims := makeClaims(time.Hour, TokenTypeAccess)
	t1 := Generate("secret-a", claims)
	t2 := Generate("secret-b", claims)
	if t1 == t2 {
		t.Error("tokens from different secrets must differ")
	}
}
