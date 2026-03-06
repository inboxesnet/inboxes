package handler

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ── verifySvixSignature tests ──

func makeSvixHeaders(t *testing.T, secret string, payload []byte, timestampOffset time.Duration) http.Header {
	t.Helper()
	ts := time.Now().Add(timestampOffset)
	tsStr := strconv.FormatInt(ts.Unix(), 10)
	msgID := "msg_test123"

	keyStr := strings.TrimPrefix(secret, "whsec_")
	key, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		t.Fatalf("decode secret: %v", err)
	}

	signedContent := fmt.Sprintf("%s.%s.%s", msgID, tsStr, string(payload))
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(signedContent))
	sig := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	h := http.Header{}
	h.Set("svix-id", msgID)
	h.Set("svix-timestamp", tsStr)
	h.Set("svix-signature", "v1,"+sig)
	return h
}

func validSecret(t *testing.T) string {
	t.Helper()
	key := make([]byte, 24)
	for i := range key {
		key[i] = byte(i + 1)
	}
	return base64.StdEncoding.EncodeToString(key)
}

func TestVerifySvixSignature_ValidSignature(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{"type":"email.received"}`)
	headers := makeSvixHeaders(t, secret, payload, 0)
	if err := verifySvixSignature(payload, headers, secret); err != nil {
		t.Errorf("verifySvixSignature(valid): got error %v, want nil", err)
	}
}

func TestVerifySvixSignature_InvalidSignature(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{"type":"email.received"}`)
	headers := makeSvixHeaders(t, secret, payload, 0)
	headers.Set("svix-signature", "v1,aW52YWxpZHNpZ25hdHVyZQ==")
	err := verifySvixSignature(payload, headers, secret)
	if err == nil {
		t.Fatal("verifySvixSignature(invalid sig): expected error, got nil")
	}
	if !strings.Contains(err.Error(), "no matching signature found") {
		t.Errorf("verifySvixSignature(invalid sig): error = %q, want 'no matching signature found'", err.Error())
	}
}

func TestVerifySvixSignature_MissingSvixID(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{}`)
	headers := makeSvixHeaders(t, secret, payload, 0)
	headers.Del("svix-id")
	err := verifySvixSignature(payload, headers, secret)
	if err == nil || !strings.Contains(err.Error(), "missing svix headers") {
		t.Errorf("verifySvixSignature(missing id): got %v, want 'missing svix headers'", err)
	}
}

func TestVerifySvixSignature_MissingTimestamp(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{}`)
	headers := makeSvixHeaders(t, secret, payload, 0)
	headers.Del("svix-timestamp")
	err := verifySvixSignature(payload, headers, secret)
	if err == nil || !strings.Contains(err.Error(), "missing svix headers") {
		t.Errorf("verifySvixSignature(missing timestamp): got %v, want 'missing svix headers'", err)
	}
}

func TestVerifySvixSignature_MissingSignature(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{}`)
	headers := makeSvixHeaders(t, secret, payload, 0)
	headers.Del("svix-signature")
	err := verifySvixSignature(payload, headers, secret)
	if err == nil || !strings.Contains(err.Error(), "missing svix headers") {
		t.Errorf("verifySvixSignature(missing signature): got %v, want 'missing svix headers'", err)
	}
}

func TestVerifySvixSignature_TimestampTooOld(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{}`)
	headers := makeSvixHeaders(t, secret, payload, -6*time.Minute)
	err := verifySvixSignature(payload, headers, secret)
	if err == nil || !strings.Contains(err.Error(), "timestamp too old or too new") {
		t.Errorf("verifySvixSignature(too old): got %v, want 'timestamp too old or too new'", err)
	}
}

func TestVerifySvixSignature_TimestampInFuture(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{}`)
	headers := makeSvixHeaders(t, secret, payload, 6*time.Minute)
	err := verifySvixSignature(payload, headers, secret)
	if err == nil || !strings.Contains(err.Error(), "timestamp too old or too new") {
		t.Errorf("verifySvixSignature(future): got %v, want 'timestamp too old or too new'", err)
	}
}

func TestVerifySvixSignature_WhsecPrefix(t *testing.T) {
	t.Parallel()
	rawSecret := validSecret(t)
	prefixedSecret := "whsec_" + rawSecret
	payload := []byte(`{"type":"test"}`)
	headers := makeSvixHeaders(t, prefixedSecret, payload, 0)
	if err := verifySvixSignature(payload, headers, prefixedSecret); err != nil {
		t.Errorf("verifySvixSignature(whsec prefix): got error %v, want nil", err)
	}
}

func TestVerifySvixSignature_MultipleSignatures(t *testing.T) {
	t.Parallel()
	secret := validSecret(t)
	payload := []byte(`{"type":"test"}`)
	headers := makeSvixHeaders(t, secret, payload, 0)
	// Prepend an invalid sig, followed by the valid one
	validSig := headers.Get("svix-signature")
	headers.Set("svix-signature", "v1,aW52YWxpZA== "+validSig)
	if err := verifySvixSignature(payload, headers, secret); err != nil {
		t.Errorf("verifySvixSignature(multiple sigs): got error %v, want nil", err)
	}
}

func TestVerifySvixSignature_InvalidBase64Secret(t *testing.T) {
	t.Parallel()
	payload := []byte(`{}`)
	h := http.Header{}
	h.Set("svix-id", "msg_123")
	h.Set("svix-timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	h.Set("svix-signature", "v1,test")
	err := verifySvixSignature(payload, h, "not-valid-base64!!!")
	if err == nil || !strings.Contains(err.Error(), "invalid secret key") {
		t.Errorf("verifySvixSignature(invalid base64): got %v, want 'invalid secret key'", err)
	}
}
