package store

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *PgStore) ListUsers(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, email, name, role, status, created_at FROM users WHERE org_id = $1 ORDER BY created_at`,
		orgID)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) InsertInvitedUser(ctx context.Context, orgID, email, name, role, token string, expiresAt time.Time) (string, error) {
	var userID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO users (org_id, email, name, role, status, invite_token, invite_expires_at)
		 VALUES ($1, $2, $3, $4, 'invited', $5, $6)
		 ON CONFLICT (email) DO UPDATE SET
		   status = CASE WHEN users.status = 'placeholder' THEN 'invited' ELSE users.status END,
		   invite_token = $5,
		   invite_expires_at = $6,
		   updated_at = now()
		 RETURNING id`,
		orgID, email, name, role, token, expiresAt,
	).Scan(&userID)
	return userID, err
}

func (s *PgStore) GetOrgName(ctx context.Context, orgID string) (string, error) {
	var name string
	err := s.q.QueryRow(ctx, `SELECT name FROM orgs WHERE id = $1`, orgID).Scan(&name)
	return name, err
}

func (s *PgStore) GetUserName(ctx context.Context, userID string) (string, error) {
	var name string
	err := s.q.QueryRow(ctx, `SELECT name FROM users WHERE id = $1`, userID).Scan(&name)
	return name, err
}

func (s *PgStore) ReinviteUser(ctx context.Context, userID, orgID, token string, expiresAt time.Time) (string, error) {
	var email string
	err := s.q.QueryRow(ctx,
		`UPDATE users SET invite_token = $1, invite_expires_at = $2, updated_at = now()
		 WHERE id = $3 AND org_id = $4 AND status IN ('invited', 'placeholder')
		 RETURNING email`,
		token, expiresAt, userID, orgID).Scan(&email)
	return email, err
}

func (s *PgStore) GetUserRole(ctx context.Context, userID, orgID string) (string, error) {
	var role string
	err := s.q.QueryRow(ctx,
		`SELECT role FROM users WHERE id = $1 AND org_id = $2`, userID, orgID,
	).Scan(&role)
	return role, err
}

func (s *PgStore) CountActiveAdmins(ctx context.Context, orgID string) (int, error) {
	var count int
	err := s.q.QueryRow(ctx,
		`SELECT COUNT(*) FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active'`,
		orgID,
	).Scan(&count)
	return count, err
}

func (s *PgStore) DisableUser(ctx context.Context, userID, orgID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE users SET status = 'disabled', invite_token = NULL, invite_expires_at = NULL, updated_at = now() WHERE id = $1 AND org_id = $2`,
		userID, orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) DeleteAliasUsers(ctx context.Context, userID string) error {
	_, err := s.q.Exec(ctx, `DELETE FROM alias_users WHERE user_id = $1`, userID)
	return err
}

func (s *PgStore) ReassignAndDisable(ctx context.Context, orgID, adminID, sourceID, targetID string) (map[string]any, error) {
	// Validate target user exists, is active, and belongs to the same org.
	var targetStatus string
	err := s.pool.QueryRow(ctx,
		`SELECT status FROM users WHERE id = $1 AND org_id = $2`,
		targetID, orgID,
	).Scan(&targetStatus)
	if err != nil {
		return nil, fmt.Errorf("target user not found")
	}
	if targetStatus != "active" {
		return nil, fmt.Errorf("target user is not active")
	}

	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to start transaction")
	}
	defer tx.Rollback(ctx)

	// Transfer threads
	threadTag, err := tx.Exec(ctx,
		`UPDATE threads SET user_id = $1, updated_at = now() WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer threads: %w", err)
	}
	threadsMoved := threadTag.RowsAffected()

	// Copy alias_users to target (skip conflicts where target already has access)
	aliasTag, err := tx.Exec(ctx,
		`INSERT INTO alias_users (alias_id, user_id, can_send_as, is_default)
		 SELECT alias_id, $1, can_send_as, false
		 FROM alias_users WHERE user_id = $2
		 ON CONFLICT (alias_id, user_id) DO NOTHING`,
		targetID, sourceID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer aliases: %w", err)
	}
	aliasesMoved := aliasTag.RowsAffected()

	// Delete source's alias_users
	if _, err := tx.Exec(ctx, `DELETE FROM alias_users WHERE user_id = $1`, sourceID); err != nil {
		slog.Error("user: failed to delete source alias_users", "source_user_id", sourceID, "error", err)
	}

	// Transfer drafts
	draftTag, err := tx.Exec(ctx,
		`UPDATE drafts SET user_id = $1, updated_at = now() WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to transfer drafts: %w", err)
	}
	draftsMoved := draftTag.RowsAffected()

	// Reassign sync_jobs
	if _, err := tx.Exec(ctx,
		`UPDATE sync_jobs SET user_id = $1 WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID); err != nil {
		return nil, fmt.Errorf("failed to reassign sync_jobs: %w", err)
	}

	// Reassign email_jobs
	if _, err := tx.Exec(ctx,
		`UPDATE email_jobs SET user_id = $1 WHERE user_id = $2 AND org_id = $3`,
		targetID, sourceID, orgID); err != nil {
		return nil, fmt.Errorf("failed to reassign email_jobs: %w", err)
	}

	// Reassign discovered_addresses
	if _, err := tx.Exec(ctx,
		`UPDATE discovered_addresses SET user_id = $1 WHERE user_id = $2`,
		targetID, sourceID); err != nil {
		return nil, fmt.Errorf("failed to reassign discovered_addresses: %w", err)
	}

	// Disable user
	tag, err := tx.Exec(ctx,
		`UPDATE users SET status = 'disabled', updated_at = now() WHERE id = $1 AND org_id = $2`,
		sourceID, orgID)
	if err != nil || tag.RowsAffected() == 0 {
		return nil, fmt.Errorf("user not found or already disabled")
	}

	// Insert audit row (best-effort)
	if _, err := tx.Exec(ctx,
		`INSERT INTO user_reassignments (org_id, source_user_id, target_user_id, reassigned_by, threads_moved, aliases_moved, drafts_moved)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		orgID, sourceID, targetID, adminID, threadsMoved, aliasesMoved, draftsMoved); err != nil {
		slog.Error("user: failed to insert audit row", "source", sourceID, "target", targetID, "error", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("failed to commit: %w", err)
	}

	slog.Info("user: disabled with reassignment",
		"source", sourceID, "target", targetID,
		"threads", threadsMoved, "aliases", aliasesMoved, "drafts", draftsMoved)

	return map[string]any{
		"status":        "disabled",
		"threads_moved": threadsMoved,
		"aliases_moved": aliasesMoved,
		"drafts_moved":  draftsMoved,
	}, nil
}

func (s *PgStore) GetMe(ctx context.Context, userID string) (map[string]any, error) {
	var id, email, name, role, status string
	var createdAt time.Time
	var isOwner, hasWebhook bool
	err := s.q.QueryRow(ctx,
		`SELECT u.id, u.email, u.name, u.role, u.status, u.created_at, u.is_owner,
		        (o.resend_webhook_id IS NOT NULL AND o.resend_webhook_id != '') AS has_webhook
		 FROM users u
		 JOIN orgs o ON o.id = u.org_id
		 WHERE u.id = $1`,
		userID).Scan(&id, &email, &name, &role, &status, &createdAt, &isOwner, &hasWebhook)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"id": id, "email": email, "name": name, "role": role,
		"status": status, "created_at": createdAt, "is_owner": isOwner,
		"has_webhook": hasWebhook,
	}, nil
}

func (s *PgStore) UpdateUserName(ctx context.Context, userID, name string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE users SET name = $1, updated_at = now() WHERE id = $2`,
		name, userID)
	return err
}

func (s *PgStore) GetPasswordHash(ctx context.Context, userID string) (string, error) {
	var hash string
	err := s.q.QueryRow(ctx,
		`SELECT password_hash FROM users WHERE id = $1`, userID).Scan(&hash)
	return hash, err
}

func (s *PgStore) UpdatePassword(ctx context.Context, userID, hash string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE users SET password_hash = $1, updated_at = now() WHERE id = $2`,
		hash, userID)
	return err
}

func (s *PgStore) GetPreferences(ctx context.Context, userID string) ([]byte, error) {
	var prefs []byte
	err := s.q.QueryRow(ctx,
		`SELECT COALESCE(notification_preferences, '{}') FROM users WHERE id = $1`,
		userID).Scan(&prefs)
	return prefs, err
}

func (s *PgStore) UpdatePreferences(ctx context.Context, userID string, prefs map[string]any) error {
	_, err := s.q.Exec(ctx,
		`UPDATE users SET notification_preferences = $1, updated_at = now() WHERE id = $2`,
		prefs, userID)
	return err
}

func (s *PgStore) ListMyAliases(ctx context.Context, userID, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT a.id, a.address, a.name, a.domain_id, au.can_send_as, au.is_default
		 FROM aliases a
		 JOIN alias_users au ON au.alias_id = a.id
		 WHERE au.user_id = $1 AND a.org_id = $2 AND a.deleted_at IS NULL
		 ORDER BY au.is_default DESC, a.address`,
		userID, orgID)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) ChangeRole(ctx context.Context, userID, orgID, role string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE users SET role = $1, updated_at = now() WHERE id = $2 AND org_id = $3`,
		role, userID, orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) GetUserOwnerAndRole(ctx context.Context, userID, orgID string) (bool, string, error) {
	var isOwner bool
	var currentRole string
	err := s.q.QueryRow(ctx,
		`SELECT is_owner, role FROM users WHERE id = $1 AND org_id = $2`,
		userID, orgID).Scan(&isOwner, &currentRole)
	return isOwner, currentRole, err
}

func (s *PgStore) EnableUser(ctx context.Context, userID, orgID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE users SET status = 'active', updated_at = now() WHERE id = $1 AND org_id = $2 AND status = 'disabled'`,
		userID, orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}
