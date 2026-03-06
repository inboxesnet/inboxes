package service

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

type EncryptionService struct {
	key [32]byte
}

func NewEncryptionService(base64Key string) (*EncryptionService, error) {
	decoded, err := base64.StdEncoding.DecodeString(base64Key)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 key: %w", err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("invalid key length: expected 32 bytes, got %d", len(decoded))
	}
	svc := &EncryptionService{}
	copy(svc.key[:], decoded)
	return svc, nil
}

func (s *EncryptionService) Encrypt(plaintext string) (ciphertext, iv, tag string, err error) {
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", "", "", fmt.Errorf("failed to generate nonce: %w", err)
	}
	sealed := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	tagSize := gcm.Overhead()
	ct := sealed[:len(sealed)-tagSize]
	authTag := sealed[len(sealed)-tagSize:]

	ciphertext = base64.StdEncoding.EncodeToString(ct)
	iv = base64.StdEncoding.EncodeToString(nonce)
	tag = base64.StdEncoding.EncodeToString(authTag)
	return ciphertext, iv, tag, nil
}

func (s *EncryptionService) Decrypt(ciphertext, iv, tag string) (string, error) {
	ct, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("failed to decode ciphertext: %w", err)
	}
	nonce, err := base64.StdEncoding.DecodeString(iv)
	if err != nil {
		return "", fmt.Errorf("failed to decode IV: %w", err)
	}
	authTag, err := base64.StdEncoding.DecodeString(tag)
	if err != nil {
		return "", fmt.Errorf("failed to decode tag: %w", err)
	}
	block, err := aes.NewCipher(s.key[:])
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}
	if len(nonce) != gcm.NonceSize() {
		return "", errors.New("invalid IV length")
	}
	if len(authTag) != gcm.Overhead() {
		return "", errors.New("invalid tag length")
	}
	sealed := append(ct, authTag...)
	plaintext, err := gcm.Open(nil, nonce, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt: %w", err)
	}
	return string(plaintext), nil
}
