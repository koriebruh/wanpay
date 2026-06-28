//go:build e2e

package e2e

import (
	"context"
	"net/http"
	"testing"
)

func TestAdminLogin_Success(t *testing.T) {
	code, resp := req(t, http.MethodPost, "/admin/login", map[string]any{
		"email":    testAdminEmail,
		"password": testAdminPass,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("login: %d %s", code, apiErr(resp))
	}
	var tok struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
	}
	mustUnmarshal(t, resp.Data, &tok)
	if tok.AccessToken == "" {
		t.Error("access_token is empty")
	}
	if tok.RefreshToken == "" {
		t.Error("refresh_token is empty")
	}
}

func TestAdminLogin_WrongPassword(t *testing.T) {
	code, _ := req(t, http.MethodPost, "/admin/login", map[string]any{
		"email":    testAdminEmail,
		"password": "wrong-password",
	}, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", code)
	}
}

func TestAdminLogin_UnknownEmail(t *testing.T) {
	code, _ := req(t, http.MethodPost, "/admin/login", map[string]any{
		"email":    "nobody@e2e.local",
		"password": "whatever",
	}, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", code)
	}
}

func TestAdminLogin_MissingFields(t *testing.T) {
	code, _ := req(t, http.MethodPost, "/admin/login", map[string]any{
		"email": testAdminEmail,
		// password missing
	}, nil)
	if code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", code)
	}
}

func TestAdminMe_Authenticated(t *testing.T) {
	token := getAdminToken(t)
	code, resp := req(t, http.MethodGet, "/admin/me", nil, bearer(token))
	if code != http.StatusOK {
		t.Fatalf("get me: %d %s", code, apiErr(resp))
	}
	var admin struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	mustUnmarshal(t, resp.Data, &admin)
	if admin.Email != testAdminEmail {
		t.Errorf("email = %q, want %q", admin.Email, testAdminEmail)
	}
	if admin.Role != "super_admin" {
		t.Errorf("role = %q, want super_admin", admin.Role)
	}
}

func TestAdminMe_NoToken(t *testing.T) {
	code, _ := req(t, http.MethodGet, "/admin/me", nil, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", code)
	}
}

func TestAdminMe_InvalidToken(t *testing.T) {
	code, _ := req(t, http.MethodGet, "/admin/me", nil, bearer("not.a.valid.jwt"))
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", code)
	}
}

func TestAdminMe_TamperedToken(t *testing.T) {
	token := getAdminToken(t)
	// Replace last 4 chars to corrupt signature
	tampered := token[:len(token)-4] + "XXXX"
	code, _ := req(t, http.MethodGet, "/admin/me", nil, bearer(tampered))
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401 for tampered token, got %d", code)
	}
}

func TestAdminRefreshToken(t *testing.T) {
	// Login to get tokens
	code, resp := req(t, http.MethodPost, "/admin/login", map[string]any{
		"email":    testAdminEmail,
		"password": testAdminPass,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	var tok struct {
		RefreshToken string `json:"refresh_token"`
	}
	mustUnmarshal(t, resp.Data, &tok)

	// Use refresh token to get new access token
	code, resp = req(t, http.MethodPost, "/admin/token/refresh", map[string]any{
		"refresh_token": tok.RefreshToken,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("refresh: %d %s", code, apiErr(resp))
	}
	var newTok struct {
		AccessToken string `json:"access_token"`
	}
	mustUnmarshal(t, resp.Data, &newTok)
	if newTok.AccessToken == "" {
		t.Error("refreshed access_token is empty")
	}
}

func TestAdminRefreshToken_WithAccessToken(t *testing.T) {
	// Using access token as refresh token should fail
	accessToken := getAdminToken(t)
	code, _ := req(t, http.MethodPost, "/admin/token/refresh", map[string]any{
		"refresh_token": accessToken,
	}, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401 when using access token as refresh token, got %d", code)
	}
}

func TestAdminChangePassword_Success(t *testing.T) {
	// Create a separate admin to avoid breaking the shared test admin password.
	adminID := "e2e-pwchange-" + randHex(6)
	email := "pwchange-" + randHex(4) + "@e2e.local"
	oldPass := "OldPass123!"
	newPass := "NewPass456@"

	if err := seedAdmin(testDB, adminID, email, oldPass); err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	t.Cleanup(func() {
		_, _ = testDB.ExecContext(context.Background(), "DELETE FROM admins WHERE id=$1", adminID)
	})

	// Login as this admin
	code, resp := req(t, http.MethodPost, "/admin/login", map[string]any{
		"email":    email,
		"password": oldPass,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("login: %d", code)
	}
	var tok struct{ AccessToken string `json:"access_token"` }
	mustUnmarshal(t, resp.Data, &tok)

	// Change password
	code, resp = req(t, http.MethodPatch, "/admin/me/password", map[string]any{
		"old_password": oldPass,
		"new_password": newPass,
	}, bearer(tok.AccessToken))
	if code != http.StatusNoContent && code != http.StatusOK {
		t.Errorf("change password: %d %s", code, apiErr(resp))
	}

	// Old password should no longer work
	code, _ = req(t, http.MethodPost, "/admin/login", map[string]any{
		"email":    email,
		"password": oldPass,
	}, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("old password should fail after change, got %d", code)
	}
}

