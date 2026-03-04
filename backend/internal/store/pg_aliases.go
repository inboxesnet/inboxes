package store

import (
	"context"
	"fmt"
	"log/slog"
)

func (s *PgStore) ListAliases(ctx context.Context, orgID, domainID string) ([]map[string]any, error) {
	query := `SELECT a.id, a.address, a.name, a.domain_id, a.created_at
	          FROM aliases a WHERE a.org_id = $1 AND a.deleted_at IS NULL`
	args := []any{orgID}

	if domainID != "" {
		query += " AND a.domain_id = $2"
		args = append(args, domainID)
	}
	query += " ORDER BY a.address"

	rows, err := s.q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	aliases, err := scanMaps(rows)
	if err != nil {
		return nil, err
	}

	// Fetch alias_users with user info
	if len(aliases) > 0 {
		aliasIDs := make([]string, len(aliases))
		for i, a := range aliases {
			aliasIDs[i] = a["id"].(string)
		}

		userRows, err := s.q.Query(ctx,
			`SELECT au.alias_id, au.user_id, au.can_send_as, au.is_default, u.name, u.email
			 FROM alias_users au
			 JOIN users u ON u.id = au.user_id
			 WHERE au.alias_id = ANY($1)
			 ORDER BY u.name`, aliasIDs)
		if err == nil {
			defer userRows.Close()
			aliasUsers := map[string][]map[string]any{}
			for userRows.Next() {
				var aliasID, userID, userName, userEmail string
				var canSendAs, isDefault bool
				if userRows.Scan(&aliasID, &userID, &canSendAs, &isDefault, &userName, &userEmail) == nil {
					aliasUsers[aliasID] = append(aliasUsers[aliasID], map[string]any{
						"user_id": userID, "can_send_as": canSendAs,
						"is_default": isDefault, "name": userName, "email": userEmail,
					})
				}
			}
			for _, a := range aliases {
				id := a["id"].(string)
				if users, ok := aliasUsers[id]; ok {
					a["users"] = users
				} else {
					a["users"] = []map[string]any{}
				}
			}
		}
	}

	return aliases, nil
}

func (s *PgStore) CreateAlias(ctx context.Context, orgID, domainID, address, name string) (string, error) {
	var aliasID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO aliases (org_id, domain_id, address, name)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (org_id, address) DO UPDATE
		   SET deleted_at = NULL, name = EXCLUDED.name
		   WHERE aliases.deleted_at IS NOT NULL
		 RETURNING id`,
		orgID, domainID, address, name,
	).Scan(&aliasID)
	if err != nil {
		return "", err
	}

	// Update discovered_addresses if this address was previously discovered
	if _, err := s.q.Exec(ctx,
		`UPDATE discovered_addresses SET type = 'group', alias_id = $1 WHERE domain_id = $2 AND address = $3`,
		aliasID, domainID, address); err != nil {
		slog.Error("aliases: failed to update discovered address", "error", err)
	}

	return aliasID, nil
}

func (s *PgStore) UpdateAlias(ctx context.Context, aliasID, orgID, name string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE aliases SET name = $1 WHERE id = $2 AND org_id = $3`,
		name, aliasID, orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) DeleteAlias(ctx context.Context, aliasID, orgID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE aliases SET deleted_at = now() WHERE id = $1 AND org_id = $2 AND deleted_at IS NULL`,
		aliasID, orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) AddAliasUser(ctx context.Context, aliasID, orgID, userID string, canSendAs bool) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO alias_users (alias_id, user_id, can_send_as)
		 VALUES ($1, $2, $3) ON CONFLICT (alias_id, user_id) DO UPDATE SET can_send_as = $3`,
		aliasID, userID, canSendAs)
	return err
}

func (s *PgStore) RemoveAliasUser(ctx context.Context, aliasID, userID string) error {
	if _, err := s.q.Exec(ctx,
		`DELETE FROM alias_users WHERE alias_id = $1 AND user_id = $2`,
		aliasID, userID); err != nil {
		slog.Error("aliases: failed to remove alias user", "error", err)
	}
	return nil
}

func (s *PgStore) SetDefaultAlias(ctx context.Context, aliasID, userID, orgID string) error {
	// Get the domain_id for this alias and verify org ownership
	var domainID string
	err := s.q.QueryRow(ctx,
		`SELECT a.domain_id FROM aliases a WHERE a.id = $1 AND a.org_id = $2`,
		aliasID, orgID).Scan(&domainID)
	if err != nil {
		return err
	}

	// Clear is_default for all of this user's aliases on this domain
	if _, err := s.q.Exec(ctx,
		`UPDATE alias_users SET is_default = false
		 WHERE user_id = $1 AND alias_id IN (
		   SELECT id FROM aliases WHERE domain_id = $2 AND org_id = $3
		 )`,
		userID, domainID, orgID); err != nil {
		slog.Error("aliases: failed to clear default alias", "error", err)
	}

	// Set this alias as default (only if user already has access)
	tag, err := s.q.Exec(ctx,
		`UPDATE alias_users SET is_default = true WHERE alias_id = $1 AND user_id = $2`,
		aliasID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("user does not have access to this alias")
	}
	return nil
}

func (s *PgStore) ListDiscoveredAddresses(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT da.id, da.domain_id, da.address, da.local_part, da.type, da.email_count
		 FROM discovered_addresses da
		 JOIN domains d ON d.id = da.domain_id
		 WHERE d.org_id = $1 AND da.type = 'unclaimed'
		 ORDER BY da.email_count DESC`,
		orgID)
	if err != nil {
		return nil, err
	}
	return scanMaps(rows)
}

func (s *PgStore) CheckAliasOrg(ctx context.Context, aliasID, orgID string) (int, error) {
	var count int
	err := s.q.QueryRow(ctx,
		`SELECT COUNT(*) FROM aliases WHERE id = $1 AND org_id = $2`,
		aliasID, orgID).Scan(&count)
	return count, err
}

func (s *PgStore) CheckUserOrg(ctx context.Context, userID, orgID string) (bool, error) {
	var targetOrgID string
	err := s.q.QueryRow(ctx,
		`SELECT org_id FROM users WHERE id = $1`, userID,
	).Scan(&targetOrgID)
	if err != nil {
		return false, err
	}
	return targetOrgID == orgID, nil
}
