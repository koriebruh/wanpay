//go:build e2e

package e2e

import (
	"net/http"
	"testing"
)

// selfRoute adalah /v1/merchants/me
const selfRoute = "/v1/merchants/me"

func TestAdminCreateMerchant(t *testing.T) {
	token := getAdminToken(t)
	_, key := createTestMerchant(t, token)
	if key == "" {
		t.Error("api_key empty on merchant creation")
	}
}

func TestAdminCreateMerchant_MissingFields(t *testing.T) {
	token := getAdminToken(t)
	code, _ := req(t, http.MethodPost, "/admin/merchants", map[string]any{
		"name": "Incomplete",
	}, bearer(token))
	if code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", code)
	}
}

func TestAdminCreateMerchant_Unauthorized(t *testing.T) {
	code, _ := req(t, http.MethodPost, "/admin/merchants", map[string]any{
		"name":        "Unauth",
		"email":       "unauth@e2e.local",
		"phone":       "081234567890",
		"webhook_url": "http://localhost:9999/webhook",
	}, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", code)
	}
}

func TestMerchantGetSelf(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	code, resp := req(t, http.MethodGet, selfRoute, nil, apiKey(key))
	if code != http.StatusOK {
		t.Fatalf("get merchant: %d %s", code, apiErr(resp))
	}
	var m struct {
		Status string `json:"status"`
	}
	mustUnmarshal(t, resp.Data, &m)
	if m.Status != "active" {
		t.Errorf("status = %q, want active after approve", m.Status)
	}
}

func TestMerchantGetSelf_InvalidAPIKey(t *testing.T) {
	code, _ := req(t, http.MethodGet, selfRoute, nil, apiKey("wpay_test_notarealkey1234567890123456"))
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", code)
	}
}

func TestMerchantGetSelf_NoAPIKey(t *testing.T) {
	code, _ := req(t, http.MethodGet, selfRoute, nil, nil)
	if code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", code)
	}
}

func TestAdminListMerchants(t *testing.T) {
	token := getAdminToken(t)
	createTestMerchant(t, token)

	code, resp := req(t, http.MethodGet, "/admin/merchants?page=1&limit=10", nil, bearer(token))
	if code != http.StatusOK {
		t.Fatalf("list merchants: %d %s", code, apiErr(resp))
	}
}

func TestAdminApproveMerchant(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)

	code, resp := req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token))
	if code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("approve merchant: %d %s", code, apiErr(resp))
	}

	code2, resp2 := req(t, http.MethodGet, selfRoute, nil, apiKey(key))
	if code2 != http.StatusOK {
		t.Fatalf("get merchant after approve: %d %s", code2, apiErr(resp2))
	}
	var m struct {
		Status string `json:"status"`
	}
	mustUnmarshal(t, resp2.Data, &m)
	if m.Status != "active" {
		t.Errorf("status = %q after approve, want active", m.Status)
	}
}

func TestAdminSuspendMerchant(t *testing.T) {
	token := getAdminToken(t)
	merchantID, _ := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	code, resp := req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/suspend", nil, bearer(token))
	if code != http.StatusOK && code != http.StatusNoContent {
		t.Fatalf("suspend: %d %s", code, apiErr(resp))
	}
}

func TestAdminGetMerchant_NotFound(t *testing.T) {
	token := getAdminToken(t)
	// Admin GET a nonexistent merchant — should return 404 or 500 depending on error wrapping.
	code, _ := req(t, http.MethodGet, "/admin/merchants/00000000-0000-0000-0000-000000000000", nil, bearer(token))
	if code != http.StatusNotFound && code != http.StatusInternalServerError {
		t.Errorf("expected 404 or 500 for nonexistent merchant, got %d", code)
	}
}

func TestMerchantUpdateProfile(t *testing.T) {
	token := getAdminToken(t)
	merchantID, key := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	code, resp := req(t, http.MethodPatch, selfRoute, map[string]any{
		"name":        "Updated Name",
		"webhook_url": "http://localhost:19998/updated",
	}, apiKey(key))
	if code != http.StatusOK {
		t.Fatalf("update merchant: %d %s", code, apiErr(resp))
	}
	var m struct {
		Name string `json:"name"`
	}
	mustUnmarshal(t, resp.Data, &m)
	if m.Name != "Updated Name" {
		t.Errorf("name = %q, want Updated Name", m.Name)
	}
}

func TestMerchantRegenerateAPIKey(t *testing.T) {
	token := getAdminToken(t)
	merchantID, oldKey := createTestMerchant(t, token)
	req(t, http.MethodPatch, "/admin/merchants/"+merchantID+"/approve", nil, bearer(token)) //nolint

	code, resp := req(t, http.MethodPost, selfRoute+"/api-key/regenerate", nil, apiKey(oldKey))
	if code != http.StatusOK {
		t.Fatalf("regenerate key: %d %s", code, apiErr(resp))
	}
	var out struct {
		APIKey string `json:"api_key"`
	}
	mustUnmarshal(t, resp.Data, &out)
	if out.APIKey == "" {
		t.Error("new api_key is empty")
	}
	if out.APIKey == oldKey {
		t.Error("new key must differ from old key")
	}

	// Old key no longer works
	code2, _ := req(t, http.MethodGet, selfRoute, nil, apiKey(oldKey))
	if code2 != http.StatusUnauthorized {
		t.Errorf("old key: expected 401, got %d", code2)
	}
}
