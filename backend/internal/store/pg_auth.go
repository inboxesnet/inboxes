package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// hashSecretToken returns the hex SHA-256 of a high-entropy token. Reset and
// invite tokens are stored as this hash, never in plaintext, so a database or
// backup leak cannot be replayed to take over an account. The raw token still
// travels in the email link; only the stored copy is hashed.
func hashSecretToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (s *PgStore) CountUsers(ctx context.Context) (int, error) {
	var count int
	err := s.q.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}

func (s *PgStore) createAccount(ctx context.Context, email, name, passwordHash string, emailVerified bool) (string, error) {
	var accountID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO accounts (email, name, password_hash, email_verified)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		email, name, passwordHash, emailVerified,
	).Scan(&accountID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return "", fmt.Errorf("email already registered")
		}
		return "", fmt.Errorf("create account: %w", err)
	}
	return accountID, nil
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

	accountID, err := s.createAccount(ctx, email, name, passwordHash, emailVerified)
	if err != nil {
		return "", "", err
	}

	var userID string
	err = s.q.QueryRow(ctx,
		`INSERT INTO users (account_id, org_id, email, name, password_hash, role, status, email_verified, is_owner)
		 VALUES ($1, $2, $3, $4, $5, 'admin', 'active', $6, $7) RETURNING id`,
		accountID, orgID, email, name, passwordHash, emailVerified, isOwner,
	).Scan(&userID)
	if err != nil {
		return "", "", fmt.Errorf("create user: %w", err)
	}
	return orgID, userID, nil
}

// CreateOrgForAccount adds a new tenant and admin membership to an existing
// verified account. The caller wraps this in a transaction.
func (s *PgStore) CreateOrgForAccount(ctx context.Context, orgName, email string) (string, string, error) {
	var accountID, name, passwordHash string
	if err := s.q.QueryRow(ctx,
		`SELECT a.id, a.name, a.password_hash
		 FROM accounts a
		 WHERE lower(a.email) = lower($1) AND a.email_verified = true
		   AND a.password_hash IS NOT NULL
		   AND EXISTS (SELECT 1 FROM users u WHERE u.account_id = a.id AND u.status = 'active')`,
		email,
	).Scan(&accountID, &name, &passwordHash); err != nil {
		return "", "", fmt.Errorf("account not available for workspace creation: %w", err)
	}

	var orgID string
	if err := s.q.QueryRow(ctx,
		`INSERT INTO orgs (name) VALUES ($1) RETURNING id`, orgName,
	).Scan(&orgID); err != nil {
		return "", "", fmt.Errorf("create org: %w", err)
	}

	var userID string
	if err := s.q.QueryRow(ctx,
		`INSERT INTO users (account_id, org_id, email, name, password_hash, role, status, email_verified, is_owner)
		 VALUES ($1, $2, $3, $4, $5, 'admin', 'active', true, false) RETURNING id`,
		accountID, orgID, email, name, passwordHash,
	).Scan(&userID); err != nil {
		return "", "", fmt.Errorf("create workspace membership: %w", err)
	}
	return orgID, userID, nil
}

// CreateWorkspaceForUser creates another tenant for an already authenticated
// account. The caller wraps the operation in a transaction.
func (s *PgStore) CreateWorkspaceForUser(ctx context.Context, userID, orgName string) (string, string, error) {
	var accountID, email, name, passwordHash string
	if err := s.q.QueryRow(ctx,
		`SELECT account.id, account.email, account.name, account.password_hash
		 FROM users membership
		 JOIN accounts account ON account.id = membership.account_id
		 JOIN orgs current_org ON current_org.id = membership.org_id
		 WHERE membership.id = $1 AND membership.status = 'active'
		   AND current_org.deleted_at IS NULL
		   AND account.email_verified = true AND account.password_hash IS NOT NULL`,
		userID,
	).Scan(&accountID, &email, &name, &passwordHash); err != nil {
		return "", "", fmt.Errorf("account not available for workspace creation: %w", err)
	}

	var orgID string
	if err := s.q.QueryRow(ctx,
		`INSERT INTO orgs (name) VALUES ($1) RETURNING id`, orgName,
	).Scan(&orgID); err != nil {
		return "", "", fmt.Errorf("create org: %w", err)
	}

	var newUserID string
	if err := s.q.QueryRow(ctx,
		`INSERT INTO users (account_id, org_id, email, name, password_hash, role, status, email_verified, is_owner)
		 VALUES ($1, $2, $3, $4, $5, 'admin', 'active', true, false) RETURNING id`,
		accountID, orgID, email, name, passwordHash,
	).Scan(&newUserID); err != nil {
		return "", "", fmt.Errorf("create workspace membership: %w", err)
	}
	return orgID, newUserID, nil
}

func (s *PgStore) SetVerificationCode(ctx context.Context, userID, code string, expires time.Time) error {
	_, err := s.q.Exec(ctx,
		`UPDATE accounts SET verification_code = $1, verification_expires_at = $2, updated_at = now()
		 WHERE id = (SELECT account_id FROM users WHERE id = $3)`,
		code, expires, userID,
	)
	return err
}

func (s *PgStore) GetLoginMemberships(ctx context.Context, email string) ([]LoginMembership, error) {
	rows, err := s.q.Query(ctx,
		`SELECT u.id, u.org_id, u.name, u.role::text, u.status::text,
		        COALESCE(a.password_hash, ''), a.email_verified, o.name, o.onboarding_completed
		 FROM accounts a
	 JOIN users u ON u.account_id = a.id
	 JOIN orgs o ON o.id = u.org_id
	 WHERE lower(a.email) = lower($1) AND o.deleted_at IS NULL
	 ORDER BY u.created_at, u.id`,
		email,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	memberships := make([]LoginMembership, 0)
	for rows.Next() {
		var item LoginMembership
		if err := rows.Scan(&item.UserID, &item.OrgID, &item.Name, &item.Role, &item.Status,
			&item.PasswordHash, &item.EmailVerified, &item.OrgName, &item.OnboardingCompleted); err != nil {
			return nil, err
		}
		memberships = append(memberships, item)
	}
	return memberships, rows.Err()
}

func (s *PgStore) GetOnboardingCompleted(ctx context.Context, orgID string) (bool, error) {
	var completed bool
	err := s.q.QueryRow(ctx, "SELECT onboarding_completed FROM orgs WHERE id = $1", orgID).Scan(&completed)
	return completed, err
}

func (s *PgStore) SetResetToken(ctx context.Context, email, token string, expires time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE accounts SET reset_token = $1, reset_expires_at = $2, updated_at = now()
		 WHERE lower(email) = lower($3) AND EXISTS (
		   SELECT 1 FROM users WHERE users.account_id = accounts.id AND users.status = 'active'
		 )`,
		hashSecretToken(token), expires, email,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) ResetPassword(ctx context.Context, passwordHash, token string) (string, error) {
	userIDs, err := s.ResetPasswordMemberships(ctx, passwordHash, token)
	if err != nil {
		return "", err
	}
	return userIDs[0], nil
}

func (s *PgStore) ResetPasswordMemberships(ctx context.Context, passwordHash, token string) ([]string, error) {
	rows, err := s.q.Query(ctx,
		`WITH reset_account AS (
		   UPDATE accounts SET password_hash = $1, reset_token = NULL, reset_expires_at = NULL, updated_at = now()
		   WHERE reset_token IN ($2, $3) AND reset_expires_at > now()
	   RETURNING id
		 )
		 UPDATE users SET password_hash = $1, updated_at = now()
		 WHERE account_id IN (SELECT id FROM reset_account)
		 RETURNING id`,
		passwordHash, hashSecretToken(token), token,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	userIDs := make([]string, 0)
	for rows.Next() {
		var userID string
		if err := rows.Scan(&userID); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, userID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(userIDs) == 0 {
		return nil, fmt.Errorf("invalid or expired reset token")
	}
	return userIDs, nil
}

func (s *PgStore) ClaimInvite(ctx context.Context, passwordHash, name, token string) (string, string, string, string, error) {
	var userID, orgID, email, role string
	err := s.q.QueryRow(ctx,
		`WITH target AS (
		   SELECT id, account_id FROM users
		   WHERE invite_token IN ($3, $4) AND invite_expires_at > now()
		     AND status IN ('placeholder', 'invited') FOR UPDATE
		 ), account_update AS (
		   UPDATE accounts a
		   SET password_hash = COALESCE(a.password_hash, NULLIF($1, '')),
		       name = CASE WHEN $2 = '' THEN a.name ELSE $2 END,
		       email_verified = true,
		       updated_at = now()
	   FROM target t
		   WHERE a.id = t.account_id
		     AND (a.password_hash IS NOT NULL OR NULLIF($1, '') IS NOT NULL)
	   RETURNING a.id, a.password_hash, a.email
		 ), member_update AS (
		   UPDATE users u SET password_hash = a.password_hash,
		       name = CASE WHEN $2 = '' THEN u.name ELSE $2 END,
		       status = 'active', invite_token = NULL, invite_expires_at = NULL, updated_at = now()
	   FROM target t JOIN account_update a ON a.id = t.account_id
		   WHERE u.id = t.id
	   RETURNING u.id, u.org_id, u.role::text, a.email
		 )
		 SELECT id, org_id, email, role FROM member_update`,
		passwordHash, name, hashSecretToken(token), token,
	).Scan(&userID, &orgID, &email, &role)
	return userID, orgID, email, role, err
}

func (s *PgStore) VerifyEmail(ctx context.Context, email, code string) (string, string, string, string, error) {
	var userID, orgID, name, role string
	err := s.q.QueryRow(ctx,
		`WITH verified AS (
		   UPDATE accounts SET email_verified = true, verification_code = NULL,
		       verification_expires_at = NULL, updated_at = now()
		   WHERE lower(email) = lower($1) AND verification_code = $2 AND verification_expires_at > now()
	   RETURNING id
		 )
		 SELECT u.id, u.org_id, u.name, u.role::text
		 FROM users u JOIN verified v ON v.id = u.account_id
		 WHERE u.status = 'active'
		 ORDER BY u.created_at, u.id LIMIT 1`,
		email, code,
	).Scan(&userID, &orgID, &name, &role)
	return userID, orgID, name, role, err
}

func (s *PgStore) ResendVerificationCode(ctx context.Context, email, code string, expires time.Time) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE accounts SET verification_code = $1, verification_expires_at = $2, updated_at = now()
		 WHERE lower(email) = lower($3) AND email_verified = false AND EXISTS (
		   SELECT 1 FROM users WHERE users.account_id = accounts.id AND users.status = 'active'
		 )`,
		code, expires, email,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) ValidateInviteToken(ctx context.Context, token string) (string, string, string, bool, error) {
	var email, name, status string
	var hasAccount bool
	err := s.q.QueryRow(ctx,
		`SELECT u.email, u.name, u.status::text, (a.password_hash IS NOT NULL)
		 FROM users u LEFT JOIN accounts a ON a.id = u.account_id
		 WHERE u.invite_token IN ($1, $2) AND u.invite_expires_at > now()`,
		hashSecretToken(token), token,
	).Scan(&email, &name, &status, &hasAccount)
	return email, name, status, hasAccount, err
}

func (s *PgStore) ListAccountUserIDs(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id FROM users WHERE account_id = (SELECT account_id FROM users WHERE id = $1) ORDER BY id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	userIDs := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		userIDs = append(userIDs, id)
	}
	return userIDs, rows.Err()
}

func (s *PgStore) ListAccountMemberships(ctx context.Context, userID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT org.id, org.name, member.role::text AS role,
		        (member.id = current.id) AS current, org.onboarding_completed
		FROM users current
		JOIN users member ON member.account_id = current.account_id
		JOIN orgs org ON org.id = member.org_id
		 WHERE current.id = $1 AND member.status = 'active' AND org.deleted_at IS NULL
		 ORDER BY member.created_at, member.id`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanMaps(rows)
}

func (s *PgStore) GetAccountMembership(ctx context.Context, userID, orgID string) (LoginMembership, error) {
	var item LoginMembership
	err := s.q.QueryRow(ctx,
		`SELECT member.id, member.org_id, member.name, member.role::text, member.status::text,
		        COALESCE(account.password_hash, ''), account.email_verified,
		        org.name, org.onboarding_completed
		 FROM users current
	 JOIN accounts account ON account.id = current.account_id
	 JOIN users member ON member.account_id = account.id
	 JOIN orgs org ON org.id = member.org_id
		 WHERE current.id = $1 AND member.org_id = $2
		   AND member.status = 'active' AND org.deleted_at IS NULL`,
		userID, orgID,
	).Scan(&item.UserID, &item.OrgID, &item.Name, &item.Role, &item.Status,
		&item.PasswordHash, &item.EmailVerified, &item.OrgName, &item.OnboardingCompleted)
	return item, err
}
