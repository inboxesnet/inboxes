package service

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"testing"
)

func validBase64Key(t *testing.T) string {
	t.Helper()
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestEncryption_RoundTrip(t *testing.T) {
	t.Parallel()
	svc, err := NewEncryptionService(validBase64Key(t))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	plaintext := "hello-world-api-key"
	ct, iv, tag, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	got, err := svc.Decrypt(ct, iv, tag)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if got != plaintext {
		t.Errorf("RoundTrip: got %q, want %q", got, plaintext)
	}
}

func TestEncryption_DifferentPlaintexts(t *testing.T) {
	t.Parallel()
	svc, err := NewEncryptionService(validBase64Key(t))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	plaintexts := []string{"", "hello 世界 🌍", strings.Repeat("a", 10000)}
	for _, pt := range plaintexts {
		ct, iv, tag, err := svc.Encrypt(pt)
		if err != nil {
			t.Fatalf("Encrypt(%q): %v", pt, err)
		}
		got, err := svc.Decrypt(ct, iv, tag)
		if err != nil {
			t.Fatalf("Decrypt(%q): %v", pt, err)
		}
		if got != pt {
			t.Errorf("RoundTrip(%q): got %q", pt, got)
		}
	}
}

func TestEncryption_DifferentCiphertexts(t *testing.T) {
	t.Parallel()
	svc, err := NewEncryptionService(validBase64Key(t))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	plaintext := "same-plaintext"
	ct1, iv1, tag1, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt1: %v", err)
	}
	ct2, iv2, tag2, err := svc.Encrypt(plaintext)
	if err != nil {
		t.Fatalf("Encrypt2: %v", err)
	}
	if ct1 == ct2 && iv1 == iv2 && tag1 == tag2 {
		t.Errorf("DifferentCiphertexts: two encryptions of same plaintext produced identical output")
	}
}

func TestEncryption_InvalidKeyLength(t *testing.T) {
	t.Parallel()
	shortKey := make([]byte, 16)
	_, _ = rand.Read(shortKey)
	b64 := base64.StdEncoding.EncodeToString(shortKey)
	_, err := NewEncryptionService(b64)
	if err == nil {
		t.Fatal("NewEncryptionService(16-byte key): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "invalid key length") {
		t.Errorf("NewEncryptionService(16-byte key): error = %q, want containing 'invalid key length'", err.Error())
	}
}

func TestEncryption_InvalidBase64Key(t *testing.T) {
	t.Parallel()
	_, err := NewEncryptionService("not-valid-base64!!!")
	if err == nil {
		t.Fatal("NewEncryptionService(garbage): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("NewEncryptionService(garbage): error = %q, want containing 'decode'", err.Error())
	}
}

func TestEncryption_TamperedCiphertext(t *testing.T) {
	t.Parallel()
	svc, err := NewEncryptionService(validBase64Key(t))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	ct, iv, tag, err := svc.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	// Flip a character in the ciphertext
	tampered := flipChar(ct)
	_, err = svc.Decrypt(tampered, iv, tag)
	if err == nil {
		t.Error("Decrypt(tampered ciphertext): expected error, got nil")
	}
}

func TestEncryption_TamperedTag(t *testing.T) {
	t.Parallel()
	svc, err := NewEncryptionService(validBase64Key(t))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	ct, iv, tag, err := svc.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	tampered := flipChar(tag)
	_, err = svc.Decrypt(ct, iv, tampered)
	if err == nil {
		t.Error("Decrypt(tampered tag): expected error, got nil")
	}
}

func TestEncryption_TamperedIV(t *testing.T) {
	t.Parallel()
	svc, err := NewEncryptionService(validBase64Key(t))
	if err != nil {
		t.Fatalf("NewEncryptionService: %v", err)
	}
	ct, iv, tag, err := svc.Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	tampered := flipChar(iv)
	_, err = svc.Decrypt(ct, tampered, tag)
	if err == nil {
		t.Error("Decrypt(tampered IV): expected error, got nil")
	}
}

// flipChar decodes base64, flips a byte, re-encodes.
func flipChar(b64 string) string {
	data, _ := base64.StdEncoding.DecodeString(b64)
	if len(data) > 0 {
		data[0] ^= 0xFF
	}
	return base64.StdEncoding.EncodeToString(data)
}
