package config

import (
	"encoding/hex"
	"strings"
	"testing"
)

const validKeyHex = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func baseEnv(t *testing.T) {
	t.Helper()
	t.Setenv("TOKEN_ENCRYPTION_KEY", validKeyHex)
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/sadbot")
	t.Setenv("JWT_SECRET", "test-secret")
	t.Setenv("ADDR", "")
	t.Setenv("EVENT_RETENTION_DAYS", "")
	t.Setenv("SEED_ADMIN_EMAIL", "")
	t.Setenv("SEED_ADMIN_PASSWORD", "")
}

func TestLoad_ValidConfig(t *testing.T) {
	baseEnv(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Addr != ":8080" {
		t.Fatalf("default ADDR = %q, want :8080", cfg.Addr)
	}
	if cfg.EventRetentionDays != 90 {
		t.Fatalf("default retention = %d, want 90", cfg.EventRetentionDays)
	}
	if len(cfg.TokenEncryptionKey) != 32 {
		t.Fatalf("key length = %d, want 32", len(cfg.TokenEncryptionKey))
	}
}

func TestLoad_AddrOverride(t *testing.T) {
	baseEnv(t)
	t.Setenv("ADDR", ":9999")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != ":9999" {
		t.Fatalf("ADDR = %q", cfg.Addr)
	}
}

func TestLoad_MissingEncryptionKey(t *testing.T) {
	baseEnv(t)
	t.Setenv("TOKEN_ENCRYPTION_KEY", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "TOKEN_ENCRYPTION_KEY") {
		t.Fatalf("expected TOKEN_ENCRYPTION_KEY error, got %v", err)
	}
}

func TestLoad_KeyWrongByteLength(t *testing.T) {
	baseEnv(t)
	t.Setenv("TOKEN_ENCRYPTION_KEY", hex.EncodeToString(make([]byte, 16)))
	if _, err := Load(); err == nil {
		t.Fatal("16-byte key must be rejected")
	}
}

func TestLoad_KeyBadHex(t *testing.T) {
	baseEnv(t)
	t.Setenv("TOKEN_ENCRYPTION_KEY", strings.Repeat("z", 64))
	if _, err := Load(); err == nil {
		t.Fatal("non-hex key must be rejected")
	}
}

func TestLoad_MissingDatabaseURL(t *testing.T) {
	baseEnv(t)
	t.Setenv("DATABASE_URL", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("expected DATABASE_URL error, got %v", err)
	}
}

func TestLoad_MissingJWTSecret(t *testing.T) {
	baseEnv(t)
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("expected JWT_SECRET error, got %v", err)
	}
}

func TestLoad_RetentionValidation(t *testing.T) {
	baseEnv(t)
	t.Setenv("EVENT_RETENTION_DAYS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("retention 0 must be rejected")
	}
	t.Setenv("EVENT_RETENTION_DAYS", "-5")
	if _, err := Load(); err == nil {
		t.Fatal("negative retention must be rejected")
	}
	t.Setenv("EVENT_RETENTION_DAYS", "abc")
	if _, err := Load(); err == nil {
		t.Fatal("non-numeric retention must be rejected")
	}
	t.Setenv("EVENT_RETENTION_DAYS", "30")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.EventRetentionDays != 30 {
		t.Fatalf("retention = %d, want 30", cfg.EventRetentionDays)
	}
}
