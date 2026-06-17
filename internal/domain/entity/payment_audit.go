package entity

import "time"

type AuditEventType string

const (
	AuditEventPaymentCreated   AuditEventType = "PAYMENT_CREATED"
	AuditEventStatusChanged    AuditEventType = "STATUS_CHANGED"
	AuditEventWebhookReceived  AuditEventType = "WEBHOOK_RECEIVED"
	AuditEventPaymentCancelled AuditEventType = "PAYMENT_CANCELLED"
	AuditEventPaymentExpired   AuditEventType = "PAYMENT_EXPIRED"
)

// PaymentAudit is an immutable record of every significant event in a payment's lifecycle.
// Records are append-only — never updated or deleted.
//
// Actor format: "system" | "webhook:{provider}" | "merchant:{id}".
// OldStatus is nil for PAYMENT_CREATED (no prior state).
type PaymentAudit struct {
	ID        string
	PaymentID string
	EventType AuditEventType
	OldStatus *PaymentStatus // nil when no previous state (e.g. PAYMENT_CREATED)
	NewStatus PaymentStatus
	Actor     string         // who triggered this event
	Metadata  map[string]any // raw webhook payload, request context, etc.
	CreatedAt time.Time
}
