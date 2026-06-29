package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/database"
	"wanpey/core/internal/infrastructure/database/postgres"
)

// seedDevCmd populates the database with a complete dev dataset:
//   - 3 admins (super_admin, ops, finance)  password: Admin123!
//   - 3 merchants (active×2, pending×1)
//   - bank accounts (primary+secondary for merchant-alpha)
//   - 4 payments (paid, paid, pending, expired)
//   - 1 disbursement (completed)
//   - mutations matching the paid payments + completed disbursement
//   - provider balances
//
// All IDs are deterministic UUIDs — safe to re-run (ON CONFLICT DO NOTHING).
var seedDevCmd = &cobra.Command{
	Use:   "seed-dev",
	Short: "Populate database with full dev sample data (idempotent)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		db, err := postgres.Connect(cfg.Database)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}

		ctx := context.Background()
		if err := runDevSeed(ctx, db); err != nil {
			return fmt.Errorf("seed-dev: %w", err)
		}
		fmt.Println("\nDev seed completed successfully.")
		printDevCredentials()
		return nil
	},
}

func runDevSeed(ctx context.Context, db database.SQLDB) error {
	steps := []struct {
		name string
		fn   func(context.Context, database.SQLDB) error
	}{
		{"admins", seedAdmins},
		{"merchants", seedMerchants},
		{"bank_accounts", seedBankAccounts},
		{"payments", seedPayments},
		{"payment_audits", seedPaymentAudits},
		{"disbursements", seedDisbursements},
		{"mutations", seedMutations},
		{"provider_balances", seedProviderBalances},
	}

	for _, step := range steps {
		if err := step.fn(ctx, db); err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
		fmt.Printf("  ✓ %s\n", step.name)
	}
	return nil
}

func execSQL(ctx context.Context, db database.SQLDB, query string, args ...any) error {
	_, err := db.ExecContext(ctx, query, args...)
	return err
}

func hashStr(s string) string {
	h := sha256.Sum256([]byte(s))
	return fmt.Sprintf("%x", h)
}

// ── Admins ─────────────────────────────────────────────────────────────────────

func seedAdmins(ctx context.Context, db database.SQLDB) error {
	pass := devHashPassword("Admin123!")
	rows := []struct{ id, email, role string }{
		{"a0000000-0000-0000-0000-000000000001", "admin@wanpey.dev", string(entity.AdminRoleSuperAdmin)},
		{"a0000000-0000-0000-0000-000000000002", "ops@wanpey.dev", string(entity.AdminRoleOps)},
		{"a0000000-0000-0000-0000-000000000003", "finance@wanpey.dev", string(entity.AdminRoleFinance)},
	}
	for _, r := range rows {
		if err := execSQL(ctx, db,
			`INSERT INTO admins (id, email, password_hash, role, is_active)
			 VALUES ($1, $2, $3, $4, true)
			 ON CONFLICT DO NOTHING`,
			r.id, r.email, pass, r.role,
		); err != nil {
			return err
		}
	}
	return nil
}

// ── Merchants ──────────────────────────────────────────────────────────────────

func seedMerchants(ctx context.Context, db database.SQLDB) error {
	feeAlpha := `{"va":{"type":"flat","amount":4000},"qris":{"type":"percentage","percentage":0.7},"disbursement":{"type":"flat","amount":6500}}`

	type mrow struct {
		id, name, email, phone, status string
		apiKeyRaw, webhookURL          string
		feeConfig                      string
		dailyLimit                     int64
	}
	merchants := []mrow{
		{
			"b0000000-0000-0000-0000-000000000001",
			"Toko Alpha Dev", "alpha@merchant.dev", "081111111111", "active",
			"wpay_test_alpha000000000000000001",
			"http://localhost:9090/webhook",
			feeAlpha, 10_000_000,
		},
		{
			"b0000000-0000-0000-0000-000000000002",
			"Warung Beta Dev", "beta@merchant.dev", "082222222222", "active",
			"wpay_test_beta0000000000000000002",
			"http://localhost:9090/webhook",
			`{}`, 0,
		},
		{
			"b0000000-0000-0000-0000-000000000003",
			"Startup Gamma Dev", "gamma@merchant.dev", "083333333333", "pending",
			"wpay_test_gamma000000000000000003",
			"",
			`{}`, 0,
		},
	}

	for _, m := range merchants {
		apiKeyHash := hashStr(m.apiKeyRaw)
		signingKey := "devsecret-" + m.id[:8]
		secretHash := hashStr(signingKey)
		if err := execSQL(ctx, db,
			`INSERT INTO merchants (
				id, name, email, phone, status,
				api_key, webhook_url, webhook_secret, webhook_signing_key,
				fee_config, daily_cashout_limit, is_production
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,false)
			ON CONFLICT DO NOTHING`,
			m.id, m.name, m.email, m.phone, m.status,
			apiKeyHash, m.webhookURL, secretHash, signingKey,
			m.feeConfig, m.dailyLimit,
		); err != nil {
			return err
		}
	}
	return nil
}

// ── Bank Accounts ──────────────────────────────────────────────────────────────

func seedBankAccounts(ctx context.Context, db database.SQLDB) error {
	type barow struct {
		id, merchantID, bankCode, accountNumber, accountName string
		isPrimary, isVerified                                bool
	}
	rows := []barow{
		{"c0000000-0000-0000-0000-000000000001", "b0000000-0000-0000-0000-000000000001", "BCA", "1234567890", "Toko Alpha Dev", true, true},
		{"c0000000-0000-0000-0000-000000000002", "b0000000-0000-0000-0000-000000000001", "BNI", "9876543210", "Toko Alpha Dev", false, false},
		{"c0000000-0000-0000-0000-000000000003", "b0000000-0000-0000-0000-000000000002", "MANDIRI", "1122334455", "Warung Beta Dev", true, true},
	}
	for _, r := range rows {
		if err := execSQL(ctx, db,
			`INSERT INTO merchant_bank_accounts (id, merchant_id, bank_code, account_number, account_name, is_primary, is_verified)
			 VALUES ($1,$2,$3,$4,$5,$6,$7)
			 ON CONFLICT DO NOTHING`,
			r.id, r.merchantID, r.bankCode, r.accountNumber, r.accountName, r.isPrimary, r.isVerified,
		); err != nil {
			return err
		}
	}
	return nil
}

// ── Payments ───────────────────────────────────────────────────────────────────

func seedPayments(ctx context.Context, db database.SQLDB) error {
	now := time.Now()
	paid1 := now.Add(-1 * time.Hour)
	paid2 := now.Add(-30 * time.Minute)

	type prow struct {
		id, merchantID, externalID, method, provider, status string
		amount, fee                                          int64
		desc, customerName, customerEmail, customerPhone     string
		vaNumber, bankCode, qrString                         string
		expiryAt                                             time.Time
		paidAt                                               *time.Time
	}
	rows := []prow{
		{
			"d0000000-0000-0000-0000-000000000001",
			"b0000000-0000-0000-0000-000000000001",
			"wpay-seed-va-001", "va", "midtrans", "paid",
			500000, 4000, "Order #SEED-001",
			"Budi Santoso", "budi@example.com", "081234567890",
			"8023123456789", "BCA", "",
			now.Add(24 * time.Hour), &paid1,
		},
		{
			"d0000000-0000-0000-0000-000000000002",
			"b0000000-0000-0000-0000-000000000001",
			"wpay-seed-qris-001", "qris", "xendit", "paid",
			250000, 1750, "Order #SEED-002",
			"Siti Rahayu", "siti@example.com", "082345678901",
			"", "", "00020101021226570011ID.CO.XENDIT.WWW011893600914000000000000021540000000000000204000053037605802ID5910TokoAlpha6007Jakarta6304SEED",
			now.Add(15 * time.Minute), &paid2,
		},
		{
			"d0000000-0000-0000-0000-000000000003",
			"b0000000-0000-0000-0000-000000000001",
			"wpay-seed-va-002", "va", "midtrans", "pending",
			1000000, 0, "Order #SEED-003",
			"Ahmad Fauzi", "ahmad@example.com", "083456789012",
			"7001987654321", "MANDIRI", "",
			now.Add(24 * time.Hour), nil,
		},
		{
			"d0000000-0000-0000-0000-000000000004",
			"b0000000-0000-0000-0000-000000000001",
			"wpay-seed-va-003", "va", "doku", "expired",
			750000, 0, "Order #SEED-004",
			"Dewi Kusuma", "dewi@example.com", "084567890123",
			"3331234567890", "BNI", "",
			now.Add(-2 * time.Hour), nil,
		},
	}

	for _, r := range rows {
		if err := execSQL(ctx, db,
			`INSERT INTO payments (
				id, merchant_id, external_id, method, provider, status,
				amount, fee_amount, currency, description,
				customer_name, customer_email, customer_phone,
				va_number, bank_code, qr_string, qr_image_url,
				expiry_at, paid_at, metadata
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'IDR',$9,$10,$11,$12,$13,$14,$15,'',
				$16,$17,'{}')
			ON CONFLICT DO NOTHING`,
			r.id, r.merchantID, r.externalID, r.method, r.provider, r.status,
			r.amount, r.fee, r.desc,
			r.customerName, r.customerEmail, r.customerPhone,
			r.vaNumber, r.bankCode, r.qrString,
			r.expiryAt, r.paidAt,
		); err != nil {
			return err
		}
	}
	return nil
}

// ── Payment Audits ─────────────────────────────────────────────────────────────

func seedPaymentAudits(ctx context.Context, db database.SQLDB) error {
	type arow struct{ paymentID, event, oldStatus, newStatus, actor string }
	rows := []arow{
		{"d0000000-0000-0000-0000-000000000001", "PAYMENT_CREATED", "", "pending", "merchant:b0000000-0000-0000-0000-000000000001"},
		{"d0000000-0000-0000-0000-000000000001", "WEBHOOK_RECEIVED", "pending", "paid", "webhook:midtrans"},
		{"d0000000-0000-0000-0000-000000000002", "PAYMENT_CREATED", "", "pending", "merchant:b0000000-0000-0000-0000-000000000001"},
		{"d0000000-0000-0000-0000-000000000002", "WEBHOOK_RECEIVED", "pending", "paid", "webhook:xendit"},
		{"d0000000-0000-0000-0000-000000000003", "PAYMENT_CREATED", "", "pending", "merchant:b0000000-0000-0000-0000-000000000001"},
		{"d0000000-0000-0000-0000-000000000004", "PAYMENT_CREATED", "", "pending", "merchant:b0000000-0000-0000-0000-000000000001"},
		{"d0000000-0000-0000-0000-000000000004", "PAYMENT_EXPIRED", "pending", "expired", "system"},
	}
	for _, r := range rows {
		var oldStatus *string
		if r.oldStatus != "" {
			oldStatus = &r.oldStatus
		}
		if err := execSQL(ctx, db,
			`INSERT INTO payment_audits (payment_id, event_type, old_status, new_status, actor)
			 VALUES ($1,$2,$3,$4,$5)
			 ON CONFLICT DO NOTHING`,
			r.paymentID, r.event, oldStatus, r.newStatus, r.actor,
		); err != nil {
			return err
		}
	}
	return nil
}

// ── Disbursements ──────────────────────────────────────────────────────────────

func seedDisbursements(ctx context.Context, db database.SQLDB) error {
	completedAt := time.Now().Add(-30 * time.Minute)
	return execSQL(ctx, db,
		`INSERT INTO disbursements (
			id, merchant_id, external_id,
			provider, status, amount, fee_amount, currency,
			bank_code, account_number, account_name,
			description, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,'IDR',$8,$9,$10,$11,$12)
		ON CONFLICT DO NOTHING`,
		"f0000000-0000-0000-0000-000000000001",
		"b0000000-0000-0000-0000-000000000001",
		"wpay-seed-disb-001",
		"xendit", "completed",
		200000, 6500,
		"BCA", "1234567890", "Toko Alpha Dev",
		"Withdrawal #SEED-001",
		completedAt,
	)
}

// ── Mutations ──────────────────────────────────────────────────────────────────

func seedMutations(ctx context.Context, db database.SQLDB) error {
	type mrow struct {
		id, refID, refType, merchantID, mtype string
		amount, fee                           int64
		desc                                  string
	}
	rows := []mrow{
		{
			"e0000000-0000-0000-0000-000000000001",
			"d0000000-0000-0000-0000-000000000001", "payment",
			"b0000000-0000-0000-0000-000000000001",
			"cash_in", 500000, 4000, "Payment Order #SEED-001",
		},
		{
			"e0000000-0000-0000-0000-000000000002",
			"d0000000-0000-0000-0000-000000000002", "payment",
			"b0000000-0000-0000-0000-000000000001",
			"cash_in", 250000, 1750, "Payment Order #SEED-002",
		},
		{
			"e0000000-0000-0000-0000-000000000003",
			"f0000000-0000-0000-0000-000000000001", "disbursement",
			"b0000000-0000-0000-0000-000000000001",
			"cash_out", 200000, 0, "Withdrawal #SEED-001",
		},
	}
	for _, r := range rows {
		if err := execSQL(ctx, db,
			`INSERT INTO mutations (id, reference_id, reference_type, merchant_id, type, amount, fee_amount, currency, description)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,'IDR',$8)
			 ON CONFLICT DO NOTHING`,
			r.id, r.refID, r.refType, r.merchantID, r.mtype, r.amount, r.fee, r.desc,
		); err != nil {
			return err
		}
	}
	return nil
}

// ── Provider Balances ──────────────────────────────────────────────────────────

func seedProviderBalances(ctx context.Context, db database.SQLDB) error {
	rows := []struct {
		provider string
		balance  int64
	}{
		{"midtrans", 50_000_000},
		{"xendit", 75_000_000},
		{"doku", 25_000_000},
		{"ipaymu", 10_000_000},
	}
	for _, r := range rows {
		if err := execSQL(ctx, db,
			`INSERT INTO provider_balances (provider, balance_idr, note)
			 VALUES ($1,$2,'dev seed — not real')
			 ON CONFLICT (provider) DO UPDATE SET balance_idr=$2, note='dev seed — not real', updated_at=NOW()`,
			r.provider, r.balance,
		); err != nil {
			return err
		}
	}
	return nil
}

// ── Credentials summary ────────────────────────────────────────────────────────

func printDevCredentials() {
	fmt.Println("\n── Dev Credentials ─────────────────────────────────────────────")
	fmt.Println("Admins  (password: Admin123!)")
	fmt.Println("  super_admin : admin@wanpey.dev")
	fmt.Println("  ops         : ops@wanpey.dev")
	fmt.Println("  finance     : finance@wanpey.dev")
	fmt.Println("\nMerchants")
	fmt.Println("  alpha (active)  API Key : wpay_test_alpha000000000000000001")
	fmt.Println("  beta  (active)  API Key : wpay_test_beta0000000000000000002")
	fmt.Println("  gamma (pending)         : needs admin approval")
	fmt.Println("\nMerchant Alpha balance  : IDR 543,500")
	fmt.Println("  cash_in  500000 - fee 4000  (SEED-001 VA paid)")
	fmt.Println("  cash_in  250000 - fee 1750  (SEED-002 QRIS paid)")
	fmt.Println("  cash_out 200000             (Withdrawal SEED-001)")
	fmt.Println("  net = (500000-4000) + (250000-1750) - 200000 = 544,250")
	fmt.Println("────────────────────────────────────────────────────────────────")
}
