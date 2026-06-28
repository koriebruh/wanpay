package xendit

import (
	"bytes"
	"context"
	"encoding/base64"
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
	baseURL    = "https://api.xendit.co"
	apiVersion = "2024-11-11"
)

// vaChannelCode maps BankCode to Xendit Payment Request API v3 channel code.
var vaChannelCode = map[entity.BankCode]string{
	entity.BankBCA:     "BCA_VIRTUAL_ACCOUNT",
	entity.BankBNI:     "BNI_VIRTUAL_ACCOUNT",
	entity.BankBRI:     "BRI_VIRTUAL_ACCOUNT",
	entity.BankBSI:     "BSI_VIRTUAL_ACCOUNT",
	entity.BankMandiri: "MANDIRI_VIRTUAL_ACCOUNT",
	entity.BankPermata: "PERMATA_VIRTUAL_ACCOUNT",
	entity.BankCIMB:    "CIMB_VIRTUAL_ACCOUNT",
}

// disbChannelCode maps BankCode to Xendit Payouts v2 channel code (separate API, different prefix).
var disbChannelCode = map[entity.BankCode]string{
	entity.BankBCA:     "ID_BCA",
	entity.BankBNI:     "ID_BNI",
	entity.BankBRI:     "ID_BRI",
	entity.BankBSI:     "ID_BSI",
	entity.BankMandiri: "ID_MANDIRI",
	entity.BankPermata: "ID_PERMATA",
	entity.BankCIMB:    "ID_CIMB",
}

type Gateway struct {
	secretKey    string
	webhookToken string
	httpClient   *http.Client
	log          *zap.Logger
}

func New(cfg config.XenditConfig, log *zap.Logger) (*Gateway, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.SecretKey == "" {
		return nil, fmt.Errorf("xendit: secret_key is required when enabled")
	}
	return &Gateway{
		secretKey:    cfg.SecretKey,
		webhookToken: cfg.WebhookToken,
		httpClient:   &http.Client{Timeout: 15 * time.Second},
		log:          log,
	}, nil
}

func (g *Gateway) ProviderName() entity.Provider { return entity.ProviderXendit }
func (g *Gateway) SupportedMethods() []entity.PaymentMethod {
	return []entity.PaymentMethod{entity.PaymentMethodVA, entity.PaymentMethodQRIS}
}
func (g *Gateway) Capabilities() []gateway.ProviderCapability {
	return []gateway.ProviderCapability{gateway.CapabilityCashIn, gateway.CapabilityCashOut}
}

func (g *Gateway) CreateVA(ctx context.Context, req gateway.CreateVARequest) (*gateway.CreateVAResponse, error) {
	channelCode, ok := vaChannelCode[req.BankCode]
	if !ok {
		return nil, fmt.Errorf("xendit: unsupported bank code %s for VA", req.BankCode)
	}

	channelProps := map[string]any{
		"display_name": req.CustomerName,
	}
	if !req.ExpiryAt.IsZero() {
		channelProps["expires_at"] = req.ExpiryAt.UTC().Format(time.RFC3339)
	}

	body := map[string]any{
		"reference_id":       req.ExternalID,
		"type":               "PAY",
		"country":            "ID",
		"currency":           "IDR",
		"request_amount":     req.Amount,
		"channel_code":       channelCode,
		"channel_properties": channelProps,
	}
	if req.Description != "" {
		body["description"] = req.Description
	}

	var resp paymentRequestResponse
	if err := g.postV3(ctx, "/v3/payment_requests", body, &resp); err != nil {
		return nil, fmt.Errorf("xendit create_va: %w", err)
	}

	vaNumber := resp.findAction("VIRTUAL_ACCOUNT_NUMBER")
	if vaNumber == "" {
		return nil, fmt.Errorf("xendit create_va: VIRTUAL_ACCOUNT_NUMBER not found in actions")
	}

	return &gateway.CreateVAResponse{
		ExternalID:        req.ExternalID,
		ProviderPaymentID: resp.PaymentRequestID,
		VANumber:          vaNumber,
		BankCode:          req.BankCode,
		Amount:            req.Amount,
		ExpiryAt:          req.ExpiryAt,
	}, nil
}

func (g *Gateway) CreateQRIS(ctx context.Context, req gateway.CreateQRISRequest) (*gateway.CreateQRISResponse, error) {
	channelProps := map[string]any{
		"qr_string_type": "DYNAMIC",
	}
	if !req.ExpiryAt.IsZero() {
		channelProps["expires_at"] = req.ExpiryAt.UTC().Format(time.RFC3339)
	}

	body := map[string]any{
		"reference_id":       req.ExternalID,
		"type":               "PAY",
		"country":            "ID",
		"currency":           "IDR",
		"request_amount":     req.Amount,
		"channel_code":       "QRIS",
		"channel_properties": channelProps,
	}
	if req.Description != "" {
		body["description"] = req.Description
	}

	var resp paymentRequestResponse
	if err := g.postV3(ctx, "/v3/payment_requests", body, &resp); err != nil {
		return nil, fmt.Errorf("xendit create_qris: %w", err)
	}

	qrString := resp.findAction("QR_STRING")
	if qrString == "" {
		return nil, fmt.Errorf("xendit create_qris: QR_STRING not found in actions")
	}

	return &gateway.CreateQRISResponse{
		ExternalID:        req.ExternalID,
		ProviderPaymentID: resp.PaymentRequestID,
		QRString:          qrString,
		Amount:            req.Amount,
		ExpiryAt:          req.ExpiryAt,
	}, nil
}

// CancelPayment cancels a payment request by payment_request_id (the ProviderPaymentID from CreateVA/CreateQRIS).
func (g *Gateway) CancelPayment(ctx context.Context, providerPaymentID string) error {
	var resp paymentRequestResponse
	if err := g.postV3(ctx, "/v3/payment_requests/"+providerPaymentID+"/cancel", nil, &resp); err != nil {
		return fmt.Errorf("xendit cancel: %w", err)
	}
	return nil
}

// GetStatus retrieves the current payment status by payment_request_id (the ProviderPaymentID from CreateVA/CreateQRIS).
func (g *Gateway) GetStatus(ctx context.Context, providerPaymentID string) (entity.PaymentStatus, error) {
	var resp paymentRequestResponse
	if err := g.getV3(ctx, "/v3/payment_requests/"+providerPaymentID, &resp); err != nil {
		return "", fmt.Errorf("xendit get_status: %w", err)
	}
	return mapPaymentStatus(resp.Status), nil
}

func (g *Gateway) ParseWebhook(_ context.Context, headers map[string]string, body []byte) (*gateway.WebhookEvent, error) {
	if headers["x-callback-token"] != g.webhookToken {
		return nil, fmt.Errorf("xendit webhook: invalid callback token")
	}

	var n paymentNotification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("xendit webhook: unmarshal: %w", err)
	}

	return &gateway.WebhookEvent{
		ExternalID: n.Data.ReferenceID,
		Status:     mapPaymentStatus(n.Data.Status),
		Amount:     n.Data.Amount,
		RawPayload: body,
	}, nil
}

func (g *Gateway) Disburse(ctx context.Context, req gateway.DisburseRequest) (*gateway.DisburseResponse, error) {
	channelCode, ok := disbChannelCode[req.BankCode]
	if !ok {
		return nil, fmt.Errorf("xendit disburse: unsupported bank code %s", req.BankCode)
	}

	body := map[string]any{
		"reference_id": req.ExternalID,
		"channel_code": channelCode,
		"channel_properties": map[string]any{
			"account_holder_name": req.AccountName,
			"account_number":      req.AccountNumber,
		},
		"amount":      req.Amount,
		"currency":    "IDR",
		"description": req.Description,
	}

	var resp disbResponse
	if err := g.postWithIdempotency(ctx, "/v2/payouts", req.ExternalID, body, &resp); err != nil {
		return nil, fmt.Errorf("xendit disburse: %w", err)
	}

	return &gateway.DisburseResponse{
		ExternalID: resp.ID,
		Status:     mapDisbStatus(resp.Status),
		Amount:     req.Amount,
	}, nil
}

func (g *Gateway) GetDisbursementStatus(ctx context.Context, externalID string) (*gateway.DisburseResponse, error) {
	var resp disbResponse
	if err := g.get(ctx, "/v2/payouts/"+externalID, &resp); err != nil {
		return nil, fmt.Errorf("xendit get_disbursement_status: %w", err)
	}
	return &gateway.DisburseResponse{
		ExternalID: resp.ID,
		Status:     mapDisbStatus(resp.Status),
		Amount:     resp.Amount,
	}, nil
}

func (g *Gateway) ParseDisbursementWebhook(_ context.Context, headers map[string]string, body []byte) (*gateway.DisbursementWebhookEvent, error) {
	if headers["x-callback-token"] != g.webhookToken {
		return nil, fmt.Errorf("xendit disbursement webhook: invalid callback token")
	}

	var n disbNotification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("xendit disbursement webhook: unmarshal: %w", err)
	}

	return &gateway.DisbursementWebhookEvent{
		ExternalID:    n.Data.ID,
		Status:        mapDisbStatus(n.Data.Status),
		FailureReason: n.Data.FailureCode,
		Amount:        n.Data.Amount,
		RawPayload:    body,
	}, nil
}

// HTTP helpers

func (g *Gateway) postV3(ctx context.Context, path string, body any, dst any) error {
	return g.requestV3(ctx, http.MethodPost, path, body, dst)
}

func (g *Gateway) getV3(ctx context.Context, path string, dst any) error {
	return g.requestV3(ctx, http.MethodGet, path, nil, dst)
}

func (g *Gateway) postWithIdempotency(ctx context.Context, path, idempotencyKey string, body any, dst any) error {
	return g.request(ctx, http.MethodPost, path, idempotencyKey, body, dst)
}

func (g *Gateway) get(ctx context.Context, path string, dst any) error {
	return g.request(ctx, http.MethodGet, path, "", nil, dst)
}

// requestV3 sends a request to a /v3/* endpoint with the mandatory api-version header.
func (g *Gateway) requestV3(ctx context.Context, method, path string, body any, dst any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(g.secretKey+":")))
	req.Header.Set("api-version", apiVersion)

	return g.doRequest(req, dst)
}

func (g *Gateway) request(ctx context.Context, method, path, idempotencyKey string, body any, dst any) error {
	var reqBody io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(b)
	}

	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, reqBody)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(g.secretKey+":")))
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-key", idempotencyKey)
	}

	return g.doRequest(req, dst)
}

func (g *Gateway) doRequest(req *http.Request, dst any) error {
	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("xendit http: %w", err)
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var apiErr struct {
			ErrorCode string   `json:"error_code"`
			Message   string   `json:"message"`
			Errors    []string `json:"errors"`
		}
		if jsonErr := json.Unmarshal(b, &apiErr); jsonErr == nil && apiErr.Message != "" {
			if len(apiErr.Errors) > 0 {
				return fmt.Errorf("%s: %s — %v", apiErr.ErrorCode, apiErr.Message, apiErr.Errors)
			}
			return fmt.Errorf("%s: %s", apiErr.ErrorCode, apiErr.Message)
		}
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	return json.Unmarshal(b, dst)
}

// Response types for Payment Request API v3

type paymentRequestAction struct {
	Type       string `json:"type"`
	Descriptor string `json:"descriptor"`
	Value      string `json:"value"`
}

type paymentRequestResponse struct {
	PaymentRequestID string                 `json:"payment_request_id"`
	ReferenceID      string                 `json:"reference_id"`
	Status           string                 `json:"status"`
	RequestAmount    int64                  `json:"request_amount"`
	ChannelCode      string                 `json:"channel_code"`
	Actions          []paymentRequestAction `json:"actions"`
}

func (r *paymentRequestResponse) findAction(descriptor string) string {
	for _, a := range r.Actions {
		if a.Descriptor == descriptor {
			return a.Value
		}
	}
	return ""
}

// paymentNotification is the v3 webhook payload for payment events.
type paymentNotification struct {
	Event      string `json:"event"`
	BusinessID string `json:"business_id"`
	Data       struct {
		PaymentRequestID string `json:"payment_request_id"`
		ReferenceID      string `json:"reference_id"`
		Status           string `json:"status"`
		Amount           int64  `json:"amount"`
		ChannelCode      string `json:"channel_code"`
	} `json:"data"`
}

// Response types for Payouts API v2

type disbResponse struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Amount int64  `json:"amount"`
}

type disbNotification struct {
	Event string `json:"event"`
	Data  struct {
		ID          string `json:"id"`
		ReferenceID string `json:"reference_id"`
		Status      string `json:"status"`
		Amount      int64  `json:"amount"`
		FailureCode string `json:"failure_code"`
	} `json:"data"`
}

func mapPaymentStatus(s string) entity.PaymentStatus {
	switch strings.ToUpper(s) {
	case "SUCCEEDED":
		return entity.PaymentStatusPaid
	case "REQUIRES_ACTION", "ACCEPTING_PAYMENTS", "AUTHORIZED":
		return entity.PaymentStatusPending
	case "EXPIRED":
		return entity.PaymentStatusExpired
	case "CANCELED":
		return entity.PaymentStatusCancelled
	default:
		return entity.PaymentStatusFailed
	}
}

func mapDisbStatus(s string) entity.DisbursementStatus {
	switch strings.ToUpper(s) {
	case "ACCEPTED", "REQUESTED":
		return entity.DisbursementStatusProcessing
	case "SUCCEEDED":
		return entity.DisbursementStatusCompleted
	case "FAILED", "CANCELLED", "REVERSED":
		return entity.DisbursementStatusFailed
	default:
		return entity.DisbursementStatusPending
	}
}
