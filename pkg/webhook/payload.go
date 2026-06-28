package webhook

import "time"

// Payload is the envelope sent to merchant webhook URLs.
// DeliveryID is the outbox row ID — merchants should use it for idempotency.
type Payload struct {
	EventType  string    `json:"event_type"`
	DeliveryID string    `json:"delivery_id"`
	CreatedAt  time.Time `json:"created_at"`
	Data       any       `json:"data"`
}

// PaymentData is Data when EventType is "payment.*".
type PaymentData struct {
	PaymentID     string    `json:"payment_id"`
	ExternalID    string    `json:"external_id"`
	MerchantID    string    `json:"merchant_id"`
	Status        string    `json:"status"`
	Method        string    `json:"method"`
	Provider      string    `json:"provider"`
	Amount        int64     `json:"amount"`
	FeeAmount     int64     `json:"fee_amount"`
	NetAmount     int64     `json:"net_amount"`
	Currency      string    `json:"currency"`
	CustomerName  string    `json:"customer_name"`
	CustomerEmail string    `json:"customer_email"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// DisbursementData is Data when EventType is "disbursement.*".
type DisbursementData struct {
	DisbursementID string    `json:"disbursement_id"`
	MerchantID     string    `json:"merchant_id"`
	Status         string    `json:"status"`
	Provider       string    `json:"provider"`
	BankCode       string    `json:"bank_code"`
	AccountNumber  string    `json:"account_number"`
	AccountName    string    `json:"account_name"`
	Amount         int64     `json:"amount"`
	FeeAmount      int64     `json:"fee_amount"`
	Currency       string    `json:"currency"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	FailedAt       *time.Time `json:"failed_at,omitempty"`
	FailureReason  string    `json:"failure_reason,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
