package midtrans

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/config"
)

const (
	sandboxBaseURL    = "https://api.sandbox.midtrans.com"
	productionBaseURL = "https://api.midtrans.com"
)

type Gateway struct {
	serverKey  string
	baseURL    string
	httpClient *http.Client
	log        *zap.Logger
}

func New(cfg config.MidtransConfig, log *zap.Logger) gateway.PaymentGateway {
	baseURL := productionBaseURL
	if !cfg.IsProduction {
		baseURL = sandboxBaseURL
	}
	return &Gateway{
		serverKey:  cfg.ServerKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
		log:        log,
	}
}

func (g *Gateway) ProviderName() entity.Provider { return entity.ProviderMidtrans }
func (g *Gateway) SupportedMethods() []entity.PaymentMethod {
	return []entity.PaymentMethod{entity.PaymentMethodVA, entity.PaymentMethodQRIS}
}

func (g *Gateway) CreateVA(ctx context.Context, req gateway.CreateVARequest) (*gateway.CreateVAResponse, error) {
	body := g.buildVARequest(req)

	var resp chargeResponse
	if err := g.post(ctx, "/v2/charge", body, &resp); err != nil {
		return nil, err
	}
	if err := resp.assertSuccess(); err != nil {
		return nil, fmt.Errorf("midtrans create_va: %w", err)
	}

	vaNumber, billerCode := g.extractVANumber(resp, req.BankCode)
	expiry, err := time.Parse("2006-01-02 15:04:05", resp.ExpiryTime)
	if err != nil || expiry.IsZero() {
		expiry = req.ExpiryAt
	}

	return &gateway.CreateVAResponse{
		ExternalID: resp.OrderID,
		VANumber:   vaNumber,
		BillerCode: billerCode,
		BankCode:   req.BankCode,
		Amount:     req.Amount,
		ExpiryAt:   expiry,
	}, nil
}

func (g *Gateway) buildVARequest(req gateway.CreateVARequest) map[string]any {
	expiryMinutes := int(time.Until(req.ExpiryAt).Minutes())
	if expiryMinutes < 1 {
		expiryMinutes = 1440
	}

	base := map[string]any{
		"transaction_details": map[string]any{
			"order_id":     req.ExternalID,
			"gross_amount": req.Amount,
		},
		"customer_details": map[string]any{
			"first_name": req.CustomerName,
			"email":      req.CustomerEmail,
			"phone":      req.CustomerPhone,
		},
		"custom_expiry": map[string]any{
			"expiry_duration": expiryMinutes,
			"unit":            "minute",
		},
	}

	if req.BankCode == entity.BankMandiri {
		base["payment_type"] = "echannel"
		base["echannel"] = map[string]any{
			"bill_info1": req.Description,
			"bill_info2": "Payment",
		}
	} else {
		base["payment_type"] = "bank_transfer"
		base["bank_transfer"] = map[string]any{
			"bank": strings.ToLower(string(req.BankCode)),
		}
	}

	return base
}

func (g *Gateway) extractVANumber(resp chargeResponse, bank entity.BankCode) (vaNumber, billerCode string) {
	switch bank {
	case entity.BankMandiri:
		return resp.BillKey, resp.BillerCode
	case entity.BankPermata:
		return resp.PermataVANumber, ""
	default:
		if len(resp.VANumbers) > 0 {
			return resp.VANumbers[0].VANumber, ""
		}
		return "", ""
	}
}

func (g *Gateway) CreateQRIS(ctx context.Context, req gateway.CreateQRISRequest) (*gateway.CreateQRISResponse, error) {
	body := map[string]any{
		"payment_type": "qris",
		"transaction_details": map[string]any{
			"order_id":     req.ExternalID,
			"gross_amount": req.Amount,
		},
		"customer_details": map[string]any{
			"first_name": req.CustomerName,
			"email":      req.CustomerEmail,
			"phone":      req.CustomerPhone,
		},
		"qris": map[string]any{"acquirer": "gopay"},
	}

	var resp chargeResponse
	if err := g.post(ctx, "/v2/charge", body, &resp); err != nil {
		return nil, err
	}
	if err := resp.assertSuccess(); err != nil {
		return nil, fmt.Errorf("midtrans create_qris: %w", err)
	}

	// Midtrans does not embed qr_string in the charge response.
	// Fetch it from the generate-qr-code action URL.
	qrImageURL := g.findActionURL(resp.Actions, "generate-qr-code")
	qrString := resp.QRString
	if qrString == "" && qrImageURL != "" {
		qrString = g.fetchQRString(ctx, qrImageURL)
	}

	return &gateway.CreateQRISResponse{
		ExternalID: resp.OrderID,
		QRString:   qrString,
		QRImageURL: qrImageURL,
		Amount:     req.Amount,
		ExpiryAt:   req.ExpiryAt,
	}, nil
}

func (g *Gateway) findActionURL(actions []action, name string) string {
	for _, a := range actions {
		if a.Name == name {
			return a.URL
		}
	}
	return ""
}

// fetchQRString makes a GET request to the Midtrans QR image URL.
// Some Midtrans integrations return the raw QR string via this endpoint;
// if not, we log a warning and return empty string — QRImageURL is still usable.
func (g *Gateway) fetchQRString(ctx context.Context, qrURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, qrURL, nil)
	if err != nil {
		g.log.Warn("midtrans: build qr fetch request failed", zap.Error(err))
		return ""
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(g.serverKey+":")))

	resp, err := g.httpClient.Do(req)
	if err != nil {
		g.log.Warn("midtrans: qr fetch failed", zap.Error(err))
		return ""
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		g.log.Warn("midtrans: read qr response failed", zap.Error(err))
		return ""
	}

	var result struct {
		QRString string `json:"qr_string"`
	}
	if err := json.Unmarshal(b, &result); err == nil && result.QRString != "" {
		return result.QRString
	}
	return ""
}

func (g *Gateway) CancelPayment(ctx context.Context, externalID string) error {
	var resp chargeResponse
	if err := g.post(ctx, "/v2/"+externalID+"/cancel", nil, &resp); err != nil {
		return err
	}
	if err := resp.assertSuccess(); err != nil {
		return fmt.Errorf("midtrans cancel: %w", err)
	}
	return nil
}

func (g *Gateway) GetStatus(ctx context.Context, externalID string) (entity.PaymentStatus, error) {
	var resp statusResponse
	if err := g.get(ctx, "/v2/"+externalID+"/status", &resp); err != nil {
		return "", err
	}
	return mapStatus(resp.TransactionStatus, resp.FraudStatus), nil
}

func (g *Gateway) ParseWebhook(_ context.Context, _ map[string]string, body []byte) (*gateway.WebhookEvent, error) {
	var n notification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("midtrans webhook: unmarshal: %w", err)
	}

	if !g.verifySignature(n.OrderID, n.StatusCode, n.GrossAmount, n.SignatureKey) {
		return nil, fmt.Errorf("midtrans webhook: invalid signature")
	}

	status := mapStatus(n.TransactionStatus, n.FraudStatus)

	var paidAt *time.Time
	if status == entity.PaymentStatusPaid && n.SettlementTime != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", n.SettlementTime); err == nil {
			paidAt = &t
		}
	}

	grossAmount, _ := strconv.ParseInt(strings.Split(n.GrossAmount, ".")[0], 10, 64) //nolint:errcheck // GrossAmount from Midtrans is always a valid decimal string

	return &gateway.WebhookEvent{
		ExternalID: n.OrderID,
		Status:     status,
		PaidAt:     paidAt,
		Amount:     grossAmount,
		RawPayload: body,
	}, nil
}

func (g *Gateway) verifySignature(orderID, statusCode, grossAmount, expected string) bool {
	h := sha512.New()
	h.Write([]byte(orderID + statusCode + grossAmount + g.serverKey))
	computed := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(computed), []byte(expected))
}

func (g *Gateway) post(ctx context.Context, path string, body any, dst any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(g.serverKey+":")))

	return g.do(req, dst)
}

func (g *Gateway) get(ctx context.Context, path string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, g.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(g.serverKey+":")))

	return g.do(req, dst)
}

func (g *Gateway) do(req *http.Request, dst any) error {
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("midtrans http: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	return json.Unmarshal(b, dst)
}

type chargeResponse struct {
	StatusCode        string   `json:"status_code"`
	StatusMessage     string   `json:"status_message"`
	OrderID           string   `json:"order_id"`
	TransactionStatus string   `json:"transaction_status"`
	GrossAmount       string   `json:"gross_amount"`
	ExpiryTime        string   `json:"expiry_time"`
	VANumbers         []vaNum  `json:"va_numbers"`
	PermataVANumber   string   `json:"permata_va_number"`
	BillKey           string   `json:"bill_key"`
	BillerCode        string   `json:"biller_code"`
	QRString          string   `json:"qr_string"`
	Actions           []action `json:"actions"`
}

func (r chargeResponse) assertSuccess() error {
	if r.StatusCode != "200" && r.StatusCode != "201" {
		return fmt.Errorf("status %s: %s", r.StatusCode, r.StatusMessage)
	}
	return nil
}

type vaNum struct {
	Bank     string `json:"bank"`
	VANumber string `json:"va_number"`
}

type action struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type statusResponse struct {
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	StatusCode        string `json:"status_code"`
	StatusMessage     string `json:"status_message"`
}

type notification struct {
	OrderID           string `json:"order_id"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	GrossAmount       string `json:"gross_amount"`
	StatusCode        string `json:"status_code"`
	SignatureKey      string `json:"signature_key"`
	SettlementTime    string `json:"settlement_time"`
}

func mapStatus(txStatus, fraudStatus string) entity.PaymentStatus {
	switch txStatus {
	case "settlement", "capture":
		if fraudStatus == "accept" || fraudStatus == "" {
			return entity.PaymentStatusPaid
		}
		return entity.PaymentStatusFailed
	case "pending":
		return entity.PaymentStatusPending
	case "expire":
		return entity.PaymentStatusExpired
	case "cancel":
		return entity.PaymentStatusCancelled
	default:
		return entity.PaymentStatusFailed
	}
}
