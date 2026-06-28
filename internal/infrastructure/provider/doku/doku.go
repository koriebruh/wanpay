package doku

import (
	"bytes"
	"context"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/domain/gateway"
	"wanpey/core/internal/infrastructure/config"
)

const (
	sandboxBaseURL    = "https://api-sandbox.doku.com"
	productionBaseURL = "https://api.doku.com"
)

// DOKU SNAP authentication — two-step:
//  1. B2B access token via POST /authorization/v1/access-token/b2b
//     Signed with SHA256withRSA(privateKey, clientID+"|"+timestamp)
//  2. Every request: Authorization: Bearer {token}
//     + HMAC-SHA512(secretKey, METHOD+":"+path+":"+token+":"+sha256(body)+":"+timestamp)

type Gateway struct {
	clientID   string
	secretKey  string // Secret Key from dashboard — used for HMAC-SHA512 request signatures
	hmacKey    string // same as secretKey; kept as a named field for signing calls
	privateKey *rsa.PrivateKey
	baseURL    string
	httpClient *http.Client
	log        *zap.Logger

	tokenMu  sync.Mutex
	token    string
	tokenExp time.Time
}

func New(cfg config.DokuConfig, log *zap.Logger) (*Gateway, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.ClientID == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("doku: client_id and secret_key are required when enabled")
	}
	if cfg.PrivateKeyPEM == "" {
		return nil, fmt.Errorf("doku: private_key is required when enabled (RSA PKCS8 PEM for B2B token)")
	}

	privateKey, err := parseRSAPrivateKey(cfg.PrivateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("doku: parse private_key_pem: %w", err)
	}

	// DOKU SNAP: transactional requests are signed with Secret Key (not API Key).
	// Secret Key is found in DOKU dashboard under "Merchant Credential" → "Secret Key".
	hmacKey := strings.TrimSpace(cfg.SecretKey)

	baseURL := sandboxBaseURL
	if cfg.IsProduction {
		baseURL = productionBaseURL
	}

	return &Gateway{
		clientID:   cfg.ClientID,
		secretKey:  cfg.SecretKey,
		hmacKey:    hmacKey,
		privateKey: privateKey,
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 15 * time.Second},
		log:        log,
	}, nil
}

func (g *Gateway) ProviderName() entity.Provider { return entity.ProviderDoku }
func (g *Gateway) SupportedMethods() []entity.PaymentMethod {
	return []entity.PaymentMethod{entity.PaymentMethodVA, entity.PaymentMethodQRIS}
}
func (g *Gateway) Capabilities() []gateway.ProviderCapability {
	return []gateway.ProviderCapability{gateway.CapabilityCashIn, gateway.CapabilityCashOut}
}

// bankChannel maps BankCode to DOKU's additionalInfo.channel value.
var bankChannel = map[entity.BankCode]string{
	entity.BankBCA:     "VIRTUAL_ACCOUNT_BANK_BCA",
	entity.BankBNI:     "VIRTUAL_ACCOUNT_BANK_BNI",
	entity.BankBRI:     "VIRTUAL_ACCOUNT_BANK_BRI",
	entity.BankBSI:     "VIRTUAL_ACCOUNT_BANK_BSI",
	entity.BankMandiri: "VIRTUAL_ACCOUNT_BANK_MANDIRI",
	entity.BankPermata: "VIRTUAL_ACCOUNT_BANK_PERMATA",
	entity.BankCIMB:    "VIRTUAL_ACCOUNT_BANK_CIMB",
}

func (g *Gateway) CreateVA(ctx context.Context, req gateway.CreateVARequest) (*gateway.CreateVAResponse, error) {
	wib := time.FixedZone("WIB", 7*3600)
	channel := bankChannel[req.BankCode]
	body := map[string]any{
		"partnerServiceId":    g.clientID, // use clientID as partnerServiceId — DOKU may assign differently
		"customerNo":          req.ExternalID,
		"virtualAccountNo":    g.clientID + req.ExternalID,
		"virtualAccountName":  req.CustomerName,
		"virtualAccountEmail": req.CustomerEmail,
		"virtualAccountPhone": req.CustomerPhone,
		"trxId":               req.ExternalID,
		"totalAmount": map[string]any{
			"value":    fmt.Sprintf("%.2f", float64(req.Amount)),
			"currency": "IDR",
		},
		"additionalInfo": map[string]any{
			"channel": channel,
		},
		"virtualAccountTrxType": "C",
		"expiredDate":           req.ExpiryAt.In(wib).Format("2006-01-02T15:04:05+07:00"),
	}

	var resp vaCreateResponse
	if err := g.post(ctx, "/virtual-accounts/bi-snap-va/v1.1/transfer-va/create-va", req.ExternalID, body, &resp); err != nil {
		return nil, fmt.Errorf("doku create_va: %w", err)
	}
	if resp.ResponseCode != "2002700" {
		return nil, fmt.Errorf("doku create_va: %s", resp.ResponseMessage)
	}

	vaNumber := resp.VirtualAccountData.VirtualAccountNo
	if vaNumber == "" {
		return nil, fmt.Errorf("doku create_va: empty virtualAccountNo in response")
	}

	return &gateway.CreateVAResponse{
		ExternalID: req.ExternalID,
		VANumber:   vaNumber,
		BankCode:   req.BankCode,
		Amount:     req.Amount,
		ExpiryAt:   req.ExpiryAt,
	}, nil
}

func (g *Gateway) CreateQRIS(ctx context.Context, req gateway.CreateQRISRequest) (*gateway.CreateQRISResponse, error) {
	body := map[string]any{
		"partnerReferenceNo": req.ExternalID,
		"amount": map[string]any{
			"value":    fmt.Sprintf("%.2f", float64(req.Amount)),
			"currency": "IDR",
		},
		"feeAmount":      map[string]any{"value": "0.00", "currency": "IDR"},
		"validityPeriod": req.ExpiryAt.UTC().Format(time.RFC3339),
	}

	var resp qrCreateResponse
	if err := g.post(ctx, "/snap-adapter/b2b/v1.0/qr/qr-mpm-generate", req.ExternalID, body, &resp); err != nil {
		return nil, fmt.Errorf("doku create_qris: %w", err)
	}

	return &gateway.CreateQRISResponse{
		ExternalID: resp.ReferenceNo,
		QRString:   resp.QRContent,
		Amount:     req.Amount,
		ExpiryAt:   req.ExpiryAt,
	}, nil
}

func (g *Gateway) CancelPayment(ctx context.Context, externalID string) error {
	body := map[string]any{"partnerReferenceNo": externalID}
	var resp baseResponse
	if err := g.post(ctx, "/snap-adapter/b2b/v1.0/qr/qr-expire", externalID, body, &resp); err != nil {
		return fmt.Errorf("doku cancel: %w", err)
	}
	return nil
}

// GetStatus uses POST (DOKU SNAP standard — not GET like Midtrans/Xendit).
func (g *Gateway) GetStatus(ctx context.Context, externalID string) (entity.PaymentStatus, error) {
	body := map[string]any{"partnerServiceId": "", "trxId": externalID}
	var resp vaInquiryResponse
	if err := g.post(ctx, "/virtual-accounts/bi-snap-va/v1.1/transfer-va/inquiry", externalID, body, &resp); err != nil {
		return "", fmt.Errorf("doku get_status: %w", err)
	}
	return mapStatus(resp.ResponseCode), nil
}

func (g *Gateway) ParseWebhook(_ context.Context, headers map[string]string, body []byte) (*gateway.WebhookEvent, error) {
	ts := headers["x-timestamp"]
	sig := headers["x-signature"]
	if !g.verifyWebhookSignature(headers["x-http-method"], headers["x-endpoint-url"], ts, body, sig) {
		return nil, fmt.Errorf("doku webhook: invalid signature")
	}

	var n vaNotification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("doku webhook: unmarshal: %w", err)
	}

	return &gateway.WebhookEvent{
		ExternalID: n.TrxID,
		Status:     mapStatus(n.ResponseCode),
		Amount:     n.Amount,
		RawPayload: body,
	}, nil
}

func (g *Gateway) Disburse(ctx context.Context, req gateway.DisburseRequest) (*gateway.DisburseResponse, error) {
	body := map[string]any{
		"partnerReferenceNo":     req.ExternalID,
		"beneficiaryAccountName": req.AccountName,
		"beneficiaryAccountNo":   req.AccountNumber,
		"beneficiaryBankCode":    string(req.BankCode),
		"amount": map[string]any{
			"value":    fmt.Sprintf("%.2f", float64(req.Amount)),
			"currency": "IDR",
		},
		"remark": req.Description,
	}

	var resp disbResponse
	if err := g.post(ctx, "/snap/v1.1/emoney/transfer-bank", req.ExternalID, body, &resp); err != nil {
		return nil, fmt.Errorf("doku disburse: %w", err)
	}

	return &gateway.DisburseResponse{
		ExternalID: resp.PartnerReferenceNo,
		Status:     mapDisbStatus(resp.ResponseCode),
		Amount:     req.Amount,
	}, nil
}

func (g *Gateway) GetDisbursementStatus(ctx context.Context, externalID string) (*gateway.DisburseResponse, error) {
	body := map[string]any{"partnerReferenceNo": externalID}
	var resp disbResponse
	if err := g.post(ctx, "/snap/v1.1/emoney/transfer-bank-status", externalID, body, &resp); err != nil {
		return nil, fmt.Errorf("doku get_disbursement_status: %w", err)
	}
	return &gateway.DisburseResponse{
		ExternalID: resp.PartnerReferenceNo,
		Status:     mapDisbStatus(resp.ResponseCode),
	}, nil
}

func (g *Gateway) ParseDisbursementWebhook(_ context.Context, headers map[string]string, body []byte) (*gateway.DisbursementWebhookEvent, error) {
	ts := headers["x-timestamp"]
	sig := headers["x-signature"]
	if !g.verifyWebhookSignature(headers["x-http-method"], headers["x-endpoint-url"], ts, body, sig) {
		return nil, fmt.Errorf("doku disbursement webhook: invalid signature")
	}

	var n disbNotification
	if err := json.Unmarshal(body, &n); err != nil {
		return nil, fmt.Errorf("doku disbursement webhook: unmarshal: %w", err)
	}

	return &gateway.DisbursementWebhookEvent{
		ExternalID:    n.PartnerReferenceNo,
		Status:        mapDisbStatus(n.ResponseCode),
		FailureReason: n.FailureCode,
		Amount:        n.Amount,
		RawPayload:    body,
	}, nil
}

func (g *Gateway) accessToken(ctx context.Context) (string, error) {
	g.tokenMu.Lock()
	defer g.tokenMu.Unlock()

	if g.token != "" && time.Now().Before(g.tokenExp.Add(-30*time.Second)) {
		return g.token, nil
	}

	ts := timestamp()
	stringToSign := g.clientID + "|" + ts

	// B2B token: asymmetric signature — SHA256withRSA with merchant's private key.
	sig, err := signRSASHA256(g.privateKey, stringToSign)
	if err != nil {
		return "", fmt.Errorf("doku token: sign: %w", err)
	}

	reqBody, _ := json.Marshal(map[string]string{"grantType": "client_credentials"}) //nolint:errcheck
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+"/authorization/v1/access-token/b2b", bytes.NewReader(reqBody))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CLIENT-KEY", g.clientID)
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-SIGNATURE", sig)

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("doku token: %w", err)
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("doku token: read response: %w", err)
	}

	var tokenResp struct {
		ResponseCode    string `json:"responseCode"`
		ResponseMessage string `json:"responseMessage"`
		AccessToken     string `json:"accessToken"`
		ExpiresIn       any    `json:"expiresIn"`
	}
	if err := json.Unmarshal(rb, &tokenResp); err != nil {
		return "", fmt.Errorf("doku token decode: %w — body: %s", err, rb)
	}
	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("doku token: %s — %s (HTTP %d)", tokenResp.ResponseCode, tokenResp.ResponseMessage, resp.StatusCode)
	}

	g.token = tokenResp.AccessToken
	expiresIn := 900
	switch v := tokenResp.ExpiresIn.(type) {
	case float64:
		expiresIn = int(v)
	case string:
		fmt.Sscanf(v, "%d", &expiresIn) //nolint:errcheck
	}
	g.tokenExp = time.Now().Add(time.Duration(expiresIn) * time.Second)
	return g.token, nil
}

func (g *Gateway) post(ctx context.Context, path, externalID string, body any, dst any) error {
	token, err := g.accessToken(ctx)
	if err != nil {
		return err
	}

	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	ts := timestamp()
	bodyHash := strings.ToLower(hex.EncodeToString(sha256Sum(b)))
	// DOKU SNAP BI-SNAP: stringToSign = HTTP_METHOD:ENDPOINT_URL:ACCESS_TOKEN:SHA256_LOWER_HEX(body):TIMESTAMP
	sig := hmacSHA512hex512(g.hmacKey, "POST:"+path+":"+token+":"+bodyHash+":"+ts)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-PARTNER-ID", g.clientID)
	req.Header.Set("X-EXTERNAL-ID", fmt.Sprintf("%s-%d", externalID, time.Now().UnixNano()))
	req.Header.Set("X-TIMESTAMP", ts)
	req.Header.Set("X-SIGNATURE", sig)
	req.Header.Set("CHANNEL-ID", "H2H")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("doku http: %w", err)
	}
	defer resp.Body.Close()

	rb, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	return json.Unmarshal(rb, dst)
}

func (g *Gateway) verifyWebhookSignature(method, path, ts string, body []byte, expected string) bool {
	token, _ := g.accessToken(context.Background()) //nolint:errcheck // best-effort; empty token causes signature mismatch which is handled by the caller
	bodyHash := strings.ToLower(hex.EncodeToString(sha256Sum(body)))
	computed := hmacSHA512(g.secretKey, method+":"+path+":"+token+":"+bodyHash+":"+ts)
	return hmac.Equal([]byte(computed), []byte(expected))
}

// Crypto helpers

// parseRSAPrivateKey parses a PKCS8 or PKCS1 RSA private key from PEM.
func parseRSAPrivateKey(pemStr string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block — check that private_key_pem starts with -----BEGIN")
	}

	// Try PKCS8 first (DOKU recommends converting to PKCS8).
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		rsaKey, ok := key.(*rsa.PrivateKey)
		if !ok {
			return nil, fmt.Errorf("private key is not RSA")
		}
		return rsaKey, nil
	}

	// Fall back to PKCS1.
	rsaKey, err2 := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err2 != nil {
		return nil, fmt.Errorf("parse PKCS8: %v; parse PKCS1: %v", err, err2)
	}
	return rsaKey, nil
}

// signRSASHA256 signs data using SHA256withRSA and returns a Base64-encoded signature.
func signRSASHA256(key *rsa.PrivateKey, data string) (string, error) {
	hash := sha256.Sum256([]byte(data))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// timestamp returns WIB (+07:00) format as shown in DOKU SNAP docs examples.
// B2B token uses the same function — both use +07:00.
func timestamp() string {
	wib := time.FixedZone("WIB", 7*3600)
	return time.Now().In(wib).Format("2006-01-02T15:04:05+07:00")
}

func sha256Sum(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}

// hmacSHA512hex512 returns lowercase hex-encoded HMAC-SHA512 for transactional request signatures.
// DOKU docs: "Algorithm symmetric signature HMAC_SHA512(clientSecret, stringToSign)" — key is Secret Key.
func hmacSHA512hex512(secret, data string) string {
	h := hmac.New(sha512.New, []byte(secret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// hmacSHA512 kept for webhook verification (may use different algorithm).
func hmacSHA512(secret, data string) string {
	h := hmac.New(sha512.New, []byte(secret))
	h.Write([]byte(data))
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

func mapStatus(code string) entity.PaymentStatus {
	switch code {
	case "00", "2002700":
		return entity.PaymentStatusPaid
	case "03":
		return entity.PaymentStatusPending
	case "05":
		return entity.PaymentStatusCancelled
	default:
		return entity.PaymentStatusFailed
	}
}

func mapDisbStatus(code string) entity.DisbursementStatus {
	switch code {
	case "2000000", "00":
		return entity.DisbursementStatusCompleted
	case "2020000":
		return entity.DisbursementStatusProcessing
	default:
		return entity.DisbursementStatusFailed
	}
}

// Response types

type baseResponse struct {
	ResponseCode    string `json:"responseCode"`
	ResponseMessage string `json:"responseMessage"`
}

type vaCreateResponse struct {
	baseResponse
	VirtualAccountData struct {
		VirtualAccountNo string `json:"virtualAccountNo"`
	} `json:"virtualAccountData"`
}

type vaInquiryResponse struct {
	baseResponse
	VirtualAccountData struct {
		VirtualAccountNo string `json:"virtualAccountNo"`
	} `json:"virtualAccountData"`
}

type qrCreateResponse struct {
	baseResponse
	ReferenceNo string `json:"referenceNo"`
	QRContent   string `json:"qrContent"`
}

type disbResponse struct {
	baseResponse
	PartnerReferenceNo string `json:"partnerReferenceNo"`
}

type vaNotification struct {
	ResponseCode string `json:"responseCode"`
	TrxID        string `json:"trxId"`
	Amount       int64  `json:"amount"`
}

type disbNotification struct {
	ResponseCode       string `json:"responseCode"`
	PartnerReferenceNo string `json:"partnerReferenceNo"`
	Amount             int64  `json:"amount"`
	FailureCode        string `json:"failureCode"`
}
