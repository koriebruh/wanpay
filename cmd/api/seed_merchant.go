package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"

	"wanpey/core/internal/domain/entity"
	"wanpey/core/internal/infrastructure/config"
	"wanpey/core/internal/infrastructure/database/postgres"
)

var (
	smName         string
	smEmail        string
	smPhone        string
	smWebhookURL   string
	smIsProduction bool
)

var seedMerchantCmd = &cobra.Command{
	Use:   "seed-merchant",
	Short: "Create a sample merchant with API key (dev/demo)",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		db, err := postgres.Connect(cfg.Database)
		if err != nil {
			return fmt.Errorf("open db: %w", err)
		}

		rawKey, hashedKey := devGenerateAPIKey(smIsProduction)
		rawSecret, hashedSecret := devGenerateSecret()

		m := &entity.Merchant{
			Name:              smName,
			Email:             smEmail,
			Phone:             smPhone,
			Status:            entity.MerchantStatusPending,
			APIKey:            hashedKey,
			WebhookURL:        smWebhookURL,
			WebhookSecret:     hashedSecret,
			WebhookSigningKey: rawSecret,
			IsProduction:      smIsProduction,
		}

		repo := postgres.NewMerchantRepo(db)
		if err := repo.Save(context.Background(), m); err != nil {
			return fmt.Errorf("create merchant (email may already exist): %w", err)
		}

		fmt.Printf("\nMerchant created successfully\n")
		fmt.Printf("  ID           : %s\n", m.ID)
		fmt.Printf("  Name         : %s\n", m.Name)
		fmt.Printf("  Email        : %s\n", m.Email)
		fmt.Printf("  Status       : %s\n", m.Status)
		fmt.Printf("  API Key      : %s\n", rawKey)
		fmt.Printf("  Webhook Secret: %s\n", rawSecret)
		fmt.Printf("\nNote: activate merchant via admin API before transacting.\n")
		fmt.Printf("  POST /admin/merchants/%s/activate\n", m.ID)
		return nil
	},
}

func init() {
	seedMerchantCmd.Flags().StringVar(&smName, "name", "Sample Merchant", "merchant display name")
	seedMerchantCmd.Flags().StringVar(&smEmail, "email", "", "merchant email (required)")
	seedMerchantCmd.Flags().StringVar(&smPhone, "phone", "", "merchant phone number")
	seedMerchantCmd.Flags().StringVar(&smWebhookURL, "webhook-url", "", "webhook callback URL")
	seedMerchantCmd.Flags().BoolVar(&smIsProduction, "production", false, "generate live_ key instead of test_ key")
	if err := seedMerchantCmd.MarkFlagRequired("email"); err != nil {
		panic(err)
	}
}

// devGenerateAPIKey returns (rawKey, sha256HexHash).
func devGenerateAPIKey(isProduction bool) (raw, hashed string) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	prefix := "wpay_test_"
	if isProduction {
		prefix = "wpay_live_"
	}
	raw = prefix + hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hashed = hex.EncodeToString(h[:])
	return
}

// devGenerateSecret returns (rawSecret, sha256HexHash).
func devGenerateSecret() (raw, hashed string) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand: " + err.Error())
	}
	raw = hex.EncodeToString(b)
	h := sha256.Sum256([]byte(raw))
	hashed = hex.EncodeToString(h[:])
	return
}

// devHashPassword returns a bcrypt hash of the given password.
func devHashPassword(password string) string {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		panic("bcrypt: " + err.Error())
	}
	return string(h)
}
