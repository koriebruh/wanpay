package ipaymu

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/config"
)

const (
	sandboxBaseURL    = "https://sandbox.ipaymu.com/api/v2"
	productionBaseURL = "https://my.ipaymu.com/api/v2"
)

// vaChannel maps BankCode to iPaymu's paymentChannel value.
var vaChannel = map[entity.BankCode]string{
	entity.BankBCA:     "bca",
	entity.BankBNI:     "bni",
	entity.BankBRI:     "bri",
	entity.BankBSI:     "bsi",
	entity.BankMandiri: "mandiri",
	entity.BankPermata: "permata",
	entity.BankCIMB:    "cimb",
}

type Gateway struct {
	apiKey     string
	va         string
	baseURL    string
	notifyURL  string
	httpClient *http.Client
	log        *zap.Logger
}

func New(cfg config.IPaymuConfig, log *zap.Logger) (gateway.PaymentGateway, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.APIKey == "" || cfg.VA == "" {
		return nil, fmt.Errorf("ipaymu: api_key and va are required when enabled")
	}
	baseURL := sandboxBaseURL
	if cfg.IsProduction {
		baseURL = productionBaseURL
	}
	notifyURL := cfg.NotifyURL
	if notifyURL == "" {
		notifyURL = baseURL + "/notify" // fallback — replace with real webhook URL in production
	}
	return &Gateway{
		apiKey:     cfg.APIKey,
		va:         cfg.VA,
		baseURL:    baseURL,
		notifyURL:  notifyURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		log:        log,
	}, nil
}

func (g *Gateway) ProviderName() entity.Provider { return entity.ProviderIPaymu }
func (g *Gateway) SupportedMethods() []entity.PaymentMethod {
	return []entity.PaymentMethod{entity.PaymentMethodVA, entity.PaymentMethodQRIS}
}

func (g *Gateway) CreateVA(ctx context.Context, req gateway.CreateVARequest) (*gateway.CreateVAResponse, error) {
	channel, ok := vaChannel[req.BankCode]
	if !ok {
		return nil, fmt.Errorf("ipaymu: unsupported bank code %s", req.BankCode)
	}

	body := buildBody(req.Description, req.Amount, req.CustomerName, req.CustomerEmail,
		req.CustomerPhone, req.ExternalID, req.ExpiryAt, "va", channel, g.notifyURL)

	var resp paymentResponse
	if err := g.post(ctx, "/payment/direct", body, &resp); err != nil {
		return nil, fmt.Errorf("ipaymu create_va: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("ipaymu create_va: %s", resp.Message)
	}

	return &gateway.CreateVAResponse{
		ExternalID:        resp.Data.ReferenceID,
		ProviderPaymentID: resp.Data.SessionID,
		VANumber:          resp.Data.PaymentNo,
		BankCode:          req.BankCode,
		Amount:            req.Amount,
		ExpiryAt:          req.ExpiryAt,
	}, nil
}

func (g *Gateway) CreateQRIS(ctx context.Context, req gateway.CreateQRISRequest) (*gateway.CreateQRISResponse, error) {
	body := buildBody(req.Description, req.Amount, req.CustomerName, req.CustomerEmail,
		req.CustomerPhone, req.ExternalID, req.ExpiryAt, "qris", "qris", g.notifyURL)

	var resp paymentResponse
	if err := g.post(ctx, "/payment/direct", body, &resp); err != nil {
		return nil, fmt.Errorf("ipaymu create_qris: %w", err)
	}
	if resp.Status != 200 {
		return nil, fmt.Errorf("ipaymu create_qris: %s", resp.Message)
	}

	return &gateway.CreateQRISResponse{
		ExternalID:        resp.Data.ReferenceID,
		ProviderPaymentID: resp.Data.SessionID,
		QRString:          resp.Data.QrString,
		QRImageURL:        resp.Data.QrImage,
		Amount:            req.Amount,
		ExpiryAt:          req.ExpiryAt,
	}, nil
}

// CancelPayment is not supported by iPaymu v2 direct API.
func (g *Gateway) CancelPayment(_ context.Context, _ string) error {
	return fmt.Errorf("ipaymu: cancel payment not supported")
}

func (g *Gateway) GetStatus(ctx context.Context, providerPaymentID string) (entity.PaymentStatus, error) {
	body := map[string]any{
		"transactionId": providerPaymentID,
	}
	var resp transactionResponse
	if err := g.post(ctx, "/transaction", body, &resp); err != nil {
		return "", fmt.Errorf("ipaymu get_status: %w", err)
	}
	return mapStatus(resp.Data.PaidStatus), nil
}

func (g *Gateway) ParseWebhook(_ context.Context, _ map[string]string, body []byte) (*gateway.WebhookEvent, error) {
	var n notification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("ipaymu webhook: unmarshal: %w", err)
	}

	return &gateway.WebhookEvent{
		ExternalID: n.ReferenceID,
		Status:     mapStatus(n.StatusCode),
		Amount:     n.Amount,
		RawPayload: body,
	}, nil
}

func (g *Gateway) post(ctx context.Context, path string, body any, dst any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	ts := time.Now().Format("20060102150405")
	sig := g.signature(b)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("va", g.va)
	req.Header.Set("signature", sig)
	req.Header.Set("timestamp", ts)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ipaymu http: %w", err)
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	return json.Unmarshal(rb, dst)
}

// buildBody creates the request body omitting empty string fields.
// iPaymu normalizes the body (strips empty strings) before computing hash on their side,
// so we must do the same to ensure our hash matches theirs.
func buildBody(description string, amount int64, name, email, phone, referenceID string,
	expiryAt time.Time, method, channel, notifyURL string) map[string]any {
	if description == "" {
		description = referenceID
	}
	b := map[string]any{
		"product":        []string{description},
		"qty":            []int{1},
		"price":          []int64{amount},
		"amount":         amount,
		"description":    []string{description},
		"referenceId":    referenceID,
		"expiredDate":    expiryAt.Format("2006-01-02 15:04:05"),
		"paymentMethod":  method,
		"paymentChannel": channel,
		"notifyUrl":      notifyURL,
	}
	if name != "" {
		b["name"] = name
	}
	if email != "" {
		b["email"] = email
	}
	if phone != "" {
		b["phone"] = phone
	}
	return b
}

// signature computes iPaymu's HMAC-SHA256 request signature.
// Formula: HMAC-SHA256(apiKey, "POST:{va}:{lowercase(SHA256(body))}:{apiKey}")
// Note: timestamp is sent as a header but NOT included in the signature string.
func (g *Gateway) signature(body []byte) string {
	h := sha256.New()
	h.Write(body)
	bodyHash := strings.ToLower(hex.EncodeToString(h.Sum(nil)))

	stringToSign := "POST:" + g.va + ":" + bodyHash + ":" + g.apiKey
	mac := hmac.New(sha256.New, []byte(g.apiKey))
	mac.Write([]byte(stringToSign))
	return hex.EncodeToString(mac.Sum(nil))
}

func mapStatus(status string) entity.PaymentStatus {
	switch strings.ToLower(status) {
	case "paid", "settled", "1":
		return entity.PaymentStatusPaid
	case "pending", "waiting", "0":
		return entity.PaymentStatusPending
	case "expired", "2":
		return entity.PaymentStatusExpired
	case "cancelled", "3":
		return entity.PaymentStatusCancelled
	default:
		return entity.PaymentStatusFailed
	}
}

type paymentResponse struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
	Data    struct {
		SessionID   string `json:"SessionId"`
		ReferenceID string `json:"ReferenceId"`
		PaymentNo   string `json:"PaymentNo"`
		PaymentName string `json:"PaymentName"`
		QrString    string `json:"QrString"` // QRIS: raw QR string
		QrImage     string `json:"QrImage"`  // QRIS: image URL
		Expired     string `json:"Expired"`
	} `json:"Data"`
}

type transactionResponse struct {
	Status  int    `json:"Status"`
	Message string `json:"Message"`
	Data    struct {
		TransactionID int64  `json:"TransactionId"`
		ReferenceID   string `json:"ReferenceId"`
		SessionID     string `json:"SessionId"`
		Status        int    `json:"Status"`     // 0=pending, 1=paid, 2=expired, 3=cancelled
		PaidStatus    string `json:"PaidStatus"` // "unpaid", "paid"
		Amount        int64  `json:"Amount"`
	} `json:"Data"`
}

type notification struct {
	ReferenceID string `json:"reference_id"`
	StatusCode  string `json:"status_code"`
	Status      string `json:"status"`
	Amount      int64  `json:"amount"`
	CreatedAt   string `json:"created_at"`
}
