package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"strconv"
)

type Config struct {
	Addr               string
	DatabaseURL        string
	JWTSecret          string
	TokenEncryptionKey []byte
	EventRetentionDays int
	SeedAdminEmail     string
	SeedAdminPassword  string
	TGSidecarURL       string
	TGSidecarKey       string
	PaymentCallbackURL string
	RendererURL        string
}

func Load() (*Config, error) {
	keyHex := os.Getenv("TOKEN_ENCRYPTION_KEY")
	if keyHex == "" {
		return nil, fmt.Errorf("TOKEN_ENCRYPTION_KEY is required (64 hex chars = 32 bytes)")
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("TOKEN_ENCRYPTION_KEY must be 32 bytes encoded as hex")
	}

	retention := 90
	if v := os.Getenv("EVENT_RETENTION_DAYS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("EVENT_RETENTION_DAYS must be a positive integer")
		}
		retention = n
	}

	cfg := &Config{
		Addr:               getenv("ADDR", ":8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		TokenEncryptionKey: key,
		EventRetentionDays: retention,
		SeedAdminEmail:     os.Getenv("SEED_ADMIN_EMAIL"),
		SeedAdminPassword:  os.Getenv("SEED_ADMIN_PASSWORD"),
		TGSidecarURL:       os.Getenv("TGSIDECAR_URL"),
		TGSidecarKey:       os.Getenv("TGSIDECAR_KEY"),
		PaymentCallbackURL: os.Getenv("PAYMENT_CALLBACK_URL"),
		RendererURL:        os.Getenv("RENDERER_URL"),
	}
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" {
		return nil, fmt.Errorf("JWT_SECRET is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
