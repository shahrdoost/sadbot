package crypto

import (
	"bytes"
	"testing"
)

func TestEncryptDecrypt_Roundtrip(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	token := "123456789:AAHdqTcvCH1vGWJxfSeofSAs0K5PALDsaw"

	enc, err := EncryptToken(key, token)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if bytes.Contains(enc, []byte(token)) {
		t.Fatal("ciphertext must not contain plaintext")
	}
	if len(enc) <= 12 { // nonce prefix
		t.Fatalf("ciphertext too short: %d", len(enc))
	}

	dec, err := DecryptToken(key, enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != token {
		t.Fatalf("roundtrip mismatch: %q != %q", dec, token)
	}
}

func TestEncryptToken_NonceRandomized(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	a, err := EncryptToken(key, "same-input")
	if err != nil {
		t.Fatal(err)
	}
	b, err := EncryptToken(key, "same-input")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two encryptions of same plaintext must differ (random nonce)")
	}
}

func TestDecryptToken_WrongKeyFails(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	other := bytes.Repeat([]byte{0x02}, 32)
	enc, err := EncryptToken(key, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecryptToken(other, enc); err == nil {
		t.Fatal("decrypt with wrong key must fail")
	}
}

func TestDecryptToken_TamperedCiphertextFails(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	enc, err := EncryptToken(key, "secret")
	if err != nil {
		t.Fatal(err)
	}
	enc[len(enc)-1] ^= 0xFF
	if _, err := DecryptToken(key, enc); err == nil {
		t.Fatal("tampered ciphertext must fail authentication")
	}
}

func TestDecryptToken_TooShort(t *testing.T) {
	key := bytes.Repeat([]byte{0x01}, 32)
	if _, err := DecryptToken(key, []byte{1, 2, 3}); err == nil {
		t.Fatal("short ciphertext must fail")
	}
}

func TestEncryptToken_BadKeySize(t *testing.T) {
	if _, err := EncryptToken([]byte("short"), "x"); err == nil {
		t.Fatal("non-32-byte key must fail")
	}
}
