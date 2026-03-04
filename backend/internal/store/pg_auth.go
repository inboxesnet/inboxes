package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *PgStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.q.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *PgStore) CreateOrgAndAdmin(ctx context.Context, orgName, email, name, passwordHash string, emailVerified bool, isOwner bool) (string, string, error) {
	// Must run inside a transaction (caller uses WithTx)
	var orgID string
	err := s.q.QueryRow(ctx,
		"INSERT INTO orgs (name) VALUES ($1) RETURNING id", orgName,
	).Scan(&orgID)
	if err != nil {
		return "", "", fmt.Errorf("create org: %w", err)
	}

	var userID string
	err = s.q.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, password_hash, role, status, email_verified, is_owner)
		 VALUES ($1, $2, $3, $4, 'admin', 'active', $5, $6) RETURNING id`,
		orgID, email, name, passwordHash, emailVerified, isOwner,
	).Scan(&userID)
	if err != nil {
		if strings.Contains(err.Error(), "unique") {
			return "", "", fmt.Errorf("email already registered")
		}
		return "", "", fmt.Errorf("create user: %w", err)
	}
	return orgID, userID, nil
}

func (s *PgStore) SetVerificationCode(ctx context.Context, userID, code string, expires time.Time) error {
	_, err := s.q.Exec(ctx,
		"UPDATE users SET verification_code = $1, verification_expires_at = $2 WHERE id = $3",
		code, expires, userID,
	)
	return err
}

func (s *PgStore) GetUserByEmail(ctx context.Context, email string) (id, orgID, name, role, status, passwordHash string, emailVerified bool, err error) {
	err = s.q.QueryRow(ctx,
		`SELECT id, org_id, name, role, status, password_hash, email_verified FROM users WHERE email = $1`,
		email,
	).Scan(&id, &orgID, &name, &role, &status, &passwordHash, &emailVerified)
	return
}

func (s *PgStore) GetOnboardingCompleted(ctx context.Context, orgID string) (bool, error) {
	var completed bool
	err := s.q.QueryRow(ctx, "SELECT onboarding_completed FROM orgs WHERE id = $1", orgID).Scan(&completed)
	return completed, err
}

func (s *PgStore) SetResetToken(ctx context.Context, email, token string, expires time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE users SET reset_token = $1, reset_expires_at = $2 WHERE email = $3 AND status = 'active'`,
		token, expires, email,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) ResetPassword(ctx context.Context, passwordHash, token string) (string, error) {
	var userID string
	err := s.q.QueryRow(ctx,
		`UPDATE users SET password_hash = $1, reset_token = NULL, reset_expires_at = NULL
		 WHERE reset_token = $2 AND reset_expires_at > now()
		 RETURNING id`,
		passwordHash, token,
	).Scan(&userID)
	return userID, err
}

func (s *PgStore) ClaimInvite(ctx context.Context, passwordHash, name, token string) (string, string, string, string, error) {
	var userID, orgID, email, role string
	err := s.q.QueryRow(ctx,
		`UPDATE users SET password_hash = $1, name = CASE WHEN $2 = '' THEN name ELSE $2 END,
		 status = 'active', invite_token = NULL, invite_expires_at = NULL
		 WHERE invite_token = $3 AND invite_expires_at > now() AND status IN ('placeholder', 'invited')
		 RETURNING id, org_id, email, role`,
		passwordHash, name, token,
	).Scan(&userID, &orgID, &email, &role)
	return userID, orgID, email, role, err
}

func (s *PgStore) VerifyEmail(ctx context.Context, email, code string) (string, string, string, string, error) {
	var userID, orgID, name, role string
	err := s.q.QueryRow(ctx,
		`UPDATE users SET email_verified = true, verification_code = NULL, verification_expires_at = NULL
		 WHERE email = $1 AND verification_code = $2 AND verification_expires_at > now()
		 RETURNING id, org_id, name, role`,
		email, code,
	).Scan(&userID, &orgID, &name, &role)
	return userID, orgID, name, role, err
}

func (s *PgStore) ResendVerificationCode(ctx context.Context, email, code string, expires time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE users SET verification_code = $1, verification_expires_at = $2
		 WHERE email = $3 AND email_verified = false AND status = 'active'`,
		code, expires, email,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) ValidateInviteToken(ctx context.Context, token string) (string, string, string, error) {
	var email, name, status string
	err := s.q.QueryRow(ctx,
		`SELECT email, name, status FROM users
		 WHERE invite_token = $1 AND invite_expires_at > now()`,
		token,
	).Scan(&email, &name, &status)
	return email, name, status, err
}
