//go:build !integration

package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"wanpey/core/pkg/apperror"
)

func allowlistMiddleware(t *testing.T, allowedByProvider map[string][]string) echo.MiddlewareFunc {
	t.Helper()
	return WebhookIPAllowlist(allowedByProvider, zap.NewNop())
}

func callAllowlist(t *testing.T, mw echo.MiddlewareFunc, provider, remoteIP string) error {
	t.Helper()
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/"+provider+"/payment", nil)
	req.RemoteAddr = remoteIP + ":54321"
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("provider")
	c.SetParamValues(provider)

	reached := false
	handler := mw(func(c echo.Context) error {
		reached = true
		return c.String(http.StatusOK, "ok")
	})

	err := handler(c)
	if err == nil && !reached {
		t.Error("handler was not called but no error returned")
	}
	return err
}

// --- Tests ---

func TestWebhookAllowlist_IPAllowed_ExactIP(t *testing.T) {
	mw := allowlistMiddleware(t, map[string][]string{
		"midtrans": {"203.0.113.10"},
	})
	if err := callAllowlist(t, mw, "midtrans", "203.0.113.10"); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestWebhookAllowlist_IPAllowed_CIDR(t *testing.T) {
	mw := allowlistMiddleware(t, map[string][]string{
		"midtrans": {"203.0.113.0/24"},
	})
	if err := callAllowlist(t, mw, "midtrans", "203.0.113.55"); err != nil {
		t.Errorf("IP in CIDR should be allowed, got %v", err)
	}
}

func TestWebhookAllowlist_IPBlocked(t *testing.T) {
	mw := allowlistMiddleware(t, map[string][]string{
		"midtrans": {"203.0.113.0/24"},
	})
	err := callAllowlist(t, mw, "midtrans", "192.0.2.1")
	if err == nil {
		t.Error("IP outside CIDR should be blocked")
	}
	ae, ok := apperror.Is(err)
	if !ok {
		t.Fatalf("error should be *AppError, got %T", err)
	}
	if ae.HTTPCode() != http.StatusForbidden {
		t.Errorf("HTTPCode = %d, want 403", ae.HTTPCode())
	}
}

func TestWebhookAllowlist_ProviderNotConfigured_AllowAll(t *testing.T) {
	// xendit not in config → no restriction
	mw := allowlistMiddleware(t, map[string][]string{
		"midtrans": {"203.0.113.10"},
	})
	if err := callAllowlist(t, mw, "xendit", "1.2.3.4"); err != nil {
		t.Errorf("unconfigured provider should allow all IPs, got %v", err)
	}
}

func TestWebhookAllowlist_EmptyConfig_AllowAll(t *testing.T) {
	mw := allowlistMiddleware(t, map[string][]string{})
	if err := callAllowlist(t, mw, "midtrans", "1.2.3.4"); err != nil {
		t.Errorf("empty config should allow all IPs, got %v", err)
	}
}

func TestWebhookAllowlist_EmptyListForProvider_AllowAll(t *testing.T) {
	// Provider key exists but empty slice → allow all
	mw := allowlistMiddleware(t, map[string][]string{
		"midtrans": {},
	})
	if err := callAllowlist(t, mw, "midtrans", "1.2.3.4"); err != nil {
		t.Errorf("empty allowlist should allow all IPs, got %v", err)
	}
}

func TestWebhookAllowlist_InvalidCIDR_Skipped(t *testing.T) {
	// Invalid CIDRs are skipped — so if ALL are invalid, effectively no restriction
	mw := allowlistMiddleware(t, map[string][]string{
		"midtrans": {"not-a-cidr"},
	})
	// All valid-looking CIDRs were invalid, so provider ends up with empty nets → allow all
	if err := callAllowlist(t, mw, "midtrans", "1.2.3.4"); err != nil {
		t.Errorf("all-invalid CIDR list should allow all IPs, got %v", err)
	}
}

func TestWebhookAllowlist_MultipleRanges_OneMatches(t *testing.T) {
	mw := allowlistMiddleware(t, map[string][]string{
		"xendit": {"10.0.0.0/8", "172.16.0.0/12", "203.0.113.0/24"},
	})
	if err := callAllowlist(t, mw, "xendit", "203.0.113.100"); err != nil {
		t.Errorf("IP in last CIDR should be allowed, got %v", err)
	}
}

func TestWebhookAllowlist_MultipleRanges_NoneMatches(t *testing.T) {
	mw := allowlistMiddleware(t, map[string][]string{
		"xendit": {"10.0.0.0/8", "172.16.0.0/12"},
	})
	err := callAllowlist(t, mw, "xendit", "203.0.113.100")
	if err == nil {
		t.Error("IP not in any range should be blocked")
	}
}

func TestWebhookAllowlist_DifferentProviders_IndependentRules(t *testing.T) {
	mw := allowlistMiddleware(t, map[string][]string{
		"midtrans": {"203.0.113.0/24"},
		"xendit":   {"198.51.100.0/24"},
	})
	// Midtrans IP not in xendit range
	err := callAllowlist(t, mw, "xendit", "203.0.113.1")
	if err == nil {
		t.Error("midtrans IP should be blocked on xendit")
	}
	// Xendit IP not in midtrans range
	err = callAllowlist(t, mw, "midtrans", "198.51.100.1")
	if err == nil {
		t.Error("xendit IP should be blocked on midtrans")
	}
	// Each allowed in their own range
	if err := callAllowlist(t, mw, "midtrans", "203.0.113.1"); err != nil {
		t.Errorf("midtrans IP in midtrans range should pass, got %v", err)
	}
	if err := callAllowlist(t, mw, "xendit", "198.51.100.1"); err != nil {
		t.Errorf("xendit IP in xendit range should pass, got %v", err)
	}
}
