package store

import (
	"context"
	"fmt"
	"strings"
)

func (s *PgStore) SetupCountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.q.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *PgStore) CreateAdminSetup(ctx context.Context, orgName, email, name, passwordHash, systemResendKey, systemFromAddress, systemFromName string, encSvc interface{ Encrypt(string) (string, string, string, error) }) (string, string, error) {
	// Create org
	var orgID string
	if err := s.q.QueryRow(ctx,
		"INSERT INTO orgs (name) VALUES ($1) RETURNING id", orgName,
	).Scan(&orgID); err != nil {
		return "", "", fmt.Errorf("create org: %w", err)
	}

	// Create admin user with is_owner = true
	var userID string
	if err := s.q.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, password_hash, role, status, email_verified, is_owner)
		 VALUES ($1, $2, $3, $4, 'admin', 'active', true, true)
		 RETURNING id`,
		orgID, email, name, passwordHash,
	).Scan(&userID); err != nil {
		if strings.Contains(err.Error(), "unique") {
			return "", "", fmt.Errorf("email already registered")
		}
		return "", "", fmt.Errorf("create user: %w", err)
	}

	// Store system Resend key if provided
	if systemResendKey != "" && encSvc != nil {
		encrypted, iv, tag, err := encSvc.Encrypt(systemResendKey)
		if err != nil {
			return "", "", fmt.Errorf("encrypt system key: %w", err)
		}
		if _, err := s.q.Exec(ctx,
			`INSERT INTO system_settings (key, value, iv, tag, encrypted)
			 VALUES ('resend_system_api_key', $1, $2, $3, true)`,
			encrypted, iv, tag,
		); err != nil {
			return "", "", fmt.Errorf("store system key: %w", err)
		}
	}

	// Store from address if provided (plain text, not encrypted)
	if systemFromAddress != "" {
		if _, err := s.q.Exec(ctx,
			`INSERT INTO system_settings (key, value, encrypted) VALUES ('system_from_address', $1, false)`,
			systemFromAddress,
		); err != nil {
			return "", "", fmt.Errorf("store from address: %w", err)
		}
	}

	// Store from name if provided (plain text, not encrypted)
	if systemFromName != "" {
		if _, err := s.q.Exec(ctx,
			`INSERT INTO system_settings (key, value, encrypted) VALUES ('system_from_name', $1, false)`,
			systemFromName,
		); err != nil {
			return "", "", fmt.Errorf("store from name: %w", err)
		}
	}

	return orgID, userID, nil
}

func (s *PgStore) UpsertSystemSetting(ctx context.Context, key, value string) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO system_settings (key, value, encrypted) VALUES ($1, $2, false)
		 ON CONFLICT (key) DO UPDATE SET value = $2, updated_at = now()`,
		key, value,
	)
	return err
}

func (s *PgStore) GetUserEmail(ctx context.Context, userID string) (string, error) {
	var email string
	err := s.q.QueryRow(ctx, "SELECT email FROM users WHERE id = $1", userID).Scan(&email)
	return email, err
}
