//go:build e2e

// Package e2e contains end-to-end tests that require live infrastructure:
// PostgreSQL (via PgBouncer on :6432) and Redis (:6379).
//
// Run with:
//
//	make infra-up
//	go test -tags e2e -v ./tests/e2e/...
//
// Tests share one httptest.Server and one test admin. Each test creates
// its own merchant + data and cleans up in t.Cleanup().
package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	deliveryHTTP "wanpey/core/internal/delivery/http"
	"wanpey/core/internal/delivery/http/handler"
	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/cache"
	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres"
	"wanpey/core/internal/usecase/impl"
	"wanpey/core/pkg/apperror"
	"wanpey/core/pkg/response"
	"wanpey/core/pkg/validator"
)

// shared across all tests in the package
var (
	testSrv        *httptest.Server
	testCfg        *config.Config
	testDB         database.SQLDB
	testFakeGW     *fakePaymentGateway
	testAdminEmail = "e2e-admin@test.local"
	testAdminPass  = "E2eTestPass123!"
	testAdminID    string
)

func TestMain(m *testing.M) {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: load config: %v\n", err)
		os.Exit(1)
	}
	testCfg = cfg

	db, err := postgres.Connect(cfg.Database)
	if err != nil {
		fmt.Fprintf(os.Stderr, "e2e: connect db: %v\n", err)
		os.Exit(1)
	}
	testDB = db

	var c cache.Cache
	if cfg.Redis.Enabled {
		c, err = cache.NewRedisCache(cfg.Redis)
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e: connect redis: %v\n", err)
			os.Exit(1)
		}
	} else {
		c = cache.NewMemoryCache()
	}

	// Seed test admin
	testAdminID = "e2e-admin-" + randHex(8)
	if err := seedAdmin(db, testAdminID, testAdminEmail, testAdminPass); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: seed admin: %v\n", err)
		os.Exit(1)
	}

	// Build fake gateway
	testFakeGW = newFakeGateway()
	payGWs := map[entity.Provider]gateway.PaymentGateway{
		entity.ProviderMidtrans: testFakeGW,
	}

	// Build test server
	testSrv = buildTestServer(cfg, db, c, payGWs)

	code := m.Run()

	// Global cleanup
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM admins WHERE id = $1", testAdminID)
	testSrv.Close()
	os.Exit(code)
}

// buildTestServer creates a fully-wired Echo server with real DB/Redis but fake payment gateways.
func buildTestServer(
	cfg *config.Config,
	db database.SQLDB,
	c cache.Cache,
	payGWs map[entity.Provider]gateway.PaymentGateway,
) *httptest.Server {
	log := zap.NewNop()

	merchantRepo := postgres.NewMerchantRepo(db)
	paymentRepo := postgres.NewPaymentRepo(db)
	disbursementRepo := postgres.NewDisbursementRepo(db)
	mutationRepo := postgres.NewMutationRepo(db)
	auditRepo := postgres.NewAuditRepo(db)
	outboxRepo := postgres.NewOutboxRepo(db)
	adminRepo := postgres.NewAdminRepo(db)
	providerBalanceRepo := postgres.NewProviderBalanceRepo(db)
	feeRepo := postgres.NewFeeRepo(db)
	feeResolver := impl.NewFeeResolver(feeRepo)

	paymentUC := impl.NewPaymentUsecase(payGWs, paymentRepo, mutationRepo, auditRepo, outboxRepo, merchantRepo, feeResolver, db, log)
	disbursementUC := impl.NewDisbursementUsecase(map[entity.Provider]gateway.DisbursementGateway{}, disbursementRepo, mutationRepo, outboxRepo, merchantRepo, feeResolver, db, log)
	mutationUC := impl.NewMutationUsecase(mutationRepo)
	merchantUC := impl.NewMerchantUsecase(merchantRepo, mutationRepo, outboxRepo, db)
	adminUC := impl.NewAdminUsecase(adminRepo, merchantRepo, merchantUC, paymentRepo, disbursementRepo, mutationRepo, providerBalanceRepo, feeRepo, cfg.Admin)

	e := echo.New()
	e.HideBanner = true
	e.Validator = validator.New()
	e.Use(middleware.Recover())
	e.Use(middleware.RequestIDWithConfig(middleware.RequestIDConfig{
		RequestIDHandler: func(c echo.Context, id string) { c.Set("request_id", id) },
	}))
	e.HTTPErrorHandler = testErrHandler(log)

	e.GET("/health", func(c echo.Context) error {
		if err := db.PingContext(c.Request().Context()); err != nil {
			return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "degraded", "db": err.Error()})
		}
		return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
	})

	deliveryHTTP.Register(e, deliveryHTTP.Routes{
		MerchantRepo:      merchantRepo,
		Cache:             c,
		Payment:           handler.NewPaymentHandler(paymentUC),
		Disbursement:      handler.NewDisbursementHandler(disbursementUC),
		Mutation:          handler.NewMutationHandler(mutationUC),
		Merchant:          handler.NewMerchantHandler(merchantUC),
		Webhook:           handler.NewWebhookHandler(paymentUC, disbursementUC),
		Admin:             handler.NewAdminHandler(adminUC),
		AdminJWTSecret:    cfg.Admin.JWTSecret,
		WebhookAllowedIPs: map[string][]string{}, // no IP restriction in tests
		Log:               log,
	})

	return httptest.NewServer(e)
}

func testErrHandler(log *zap.Logger) echo.HTTPErrorHandler {
	return func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}
		status := http.StatusInternalServerError
		msg := "internal server error"
		var details []response.FieldDetail

		var ae *apperror.AppError
		if errors.As(err, &ae) {
			status = ae.HTTPCode()
			msg = ae.Message
			for _, d := range ae.Details {
				details = append(details, response.FieldDetail{Field: d.Field, Message: d.Message})
			}
		} else if he, ok := err.(*echo.HTTPError); ok {
			status = he.Code
			if s, ok := he.Message.(string); ok {
				msg = s
			}
		} else {
			log.Error("unhandled e2e error", zap.Error(err))
		}
		_ = response.Err(c, status, msg, details...)
	}
}

// seedAdmin inserts a test admin directly into the DB.
func seedAdmin(db database.SQLDB, id, email, password string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(context.Background(), `
		INSERT INTO admins (id, email, password_hash, role, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, 'super_admin', true, NOW(), NOW())
		ON CONFLICT (email) DO UPDATE SET id = $1, password_hash = $3, is_active = true
	`, id, email, string(hash))
	return err
}

// ── HTTP helpers ──────────────────────────────────────────────────────────────

type apiResp struct {
	Success bool            `json:"success"`
	Data    json.RawMessage `json:"data"`
	Error   *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// req makes a JSON HTTP request to the test server and returns status + parsed body.
func req(t *testing.T, method, path string, body any, headers map[string]string) (int, *apiResp) {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("marshal body: %v", err)
		}
	}
	r, err := http.NewRequest(method, testSrv.URL+path, &buf)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(r)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()

	var out apiResp
	// Ignore EOF — some endpoints (e.g. webhook, NoContent) return empty body.
	if decErr := json.NewDecoder(resp.Body).Decode(&out); decErr != nil && resp.ContentLength != 0 {
		// Best-effort: ignore decode errors for non-JSON responses.
		_ = decErr
	}
	return resp.StatusCode, &out
}

// getAdminToken logs in as the shared test admin and returns the access token.
func getAdminToken(t *testing.T) string {
	t.Helper()
	code, resp := req(t, http.MethodPost, "/admin/login", map[string]any{
		"email":    testAdminEmail,
		"password": testAdminPass,
	}, nil)
	if code != http.StatusOK {
		t.Fatalf("admin login: status=%d err=%s", code, apiErr(resp))
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	mustUnmarshal(t, resp.Data, &tok)
	return tok.AccessToken
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func apiKey(key string) map[string]string {
	return map[string]string{"X-API-Key": key}
}

func apiErr(r *apiResp) string {
	if r != nil && r.Error != nil {
		return r.Error.Message
	}
	return "(no error)"
}

func mustUnmarshal(t *testing.T, raw json.RawMessage, v any) {
	t.Helper()
	if err := json.Unmarshal(raw, v); err != nil {
		t.Fatalf("unmarshal: %v (raw=%s)", err, string(raw))
	}
}

// createTestMerchant creates a merchant via admin API, registers cleanup, and returns
// (merchantID, rawAPIKey). Safe to call from any test.
func createTestMerchant(t *testing.T, token string) (id, rawKey string) {
	t.Helper()
	code, resp := req(t, http.MethodPost, "/admin/merchants", map[string]any{
		"name":        "E2E Merchant " + randHex(4),
		"email":       "merchant-" + randHex(6) + "@e2e.local",
		"phone":       "08123456" + randHex(2),
		"webhook_url": "http://localhost:19999/webhook",
	}, bearer(token))
	if code != http.StatusCreated {
		t.Fatalf("create merchant: %d %s", code, apiErr(resp))
	}
	var m struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	mustUnmarshal(t, resp.Data, &m)

	t.Cleanup(func() {
		ctx := context.Background()
		_, _ = testDB.ExecContext(ctx, `DELETE FROM payment_audits WHERE payment_id IN (SELECT id FROM payments WHERE merchant_id=$1)`, m.ID)
		_, _ = testDB.ExecContext(ctx, `DELETE FROM outbox WHERE merchant_id=$1`, m.ID)
		_, _ = testDB.ExecContext(ctx, `DELETE FROM mutations WHERE merchant_id=$1`, m.ID)
		_, _ = testDB.ExecContext(ctx, `DELETE FROM payments WHERE merchant_id=$1`, m.ID)
		_, _ = testDB.ExecContext(ctx, `DELETE FROM disbursements WHERE merchant_id=$1`, m.ID)
		_, _ = testDB.ExecContext(ctx, `DELETE FROM merchant_bank_accounts WHERE merchant_id=$1`, m.ID)
		_, _ = testDB.ExecContext(ctx, `DELETE FROM merchants WHERE id=$1`, m.ID)
	})
	return m.ID, m.APIKey
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)[:n]
}
