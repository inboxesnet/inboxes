package store

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

func (s *PgStore) ListDrafts(ctx context.Context, userID, orgID, domainID string) ([]map[string]any, error) {
	query := `SELECT id, domain_id, thread_id, kind, subject, from_address,
		to_addresses, cc_addresses, bcc_addresses, body_html, body_plain,
		created_at, updated_at, COALESCE(attachment_ids, '[]')
		FROM drafts WHERE user_id = $1 AND org_id = $2`
	args := []any{userID, orgID}

	if domainID != "" {
		query += " AND domain_id = $3"
		args = append(args, domainID)
	}
	query += " ORDER BY updated_at DESC"

	rows, err := s.q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var drafts []map[string]any
	for rows.Next() {
		var id, domID, kind, subject, fromAddr, bodyHTML, bodyPlain string
		var threadID *string
		var toAddr, ccAddr, bccAddr, attIDs json.RawMessage
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &domID, &threadID, &kind, &subject, &fromAddr,
			&toAddr, &ccAddr, &bccAddr, &bodyHTML, &bodyPlain,
			&createdAt, &updatedAt, &attIDs)
		if err != nil {
			continue
		}

		draft := map[string]any{
			"id":             id,
			"domain_id":      domID,
			"thread_id":      threadID,
			"kind":           kind,
			"subject":        subject,
			"from_address":   fromAddr,
			"to_addresses":   json.RawMessage(toAddr),
			"cc_addresses":   json.RawMessage(ccAddr),
			"bcc_addresses":  json.RawMessage(bccAddr),
			"body_html":      bodyHTML,
			"body_plain":     bodyPlain,
			"attachment_ids": json.RawMessage(attIDs),
			"created_at":     createdAt,
			"updated_at":     updatedAt,
		}
		drafts = append(drafts, draft)
	}
	if drafts == nil {
		drafts = []map[string]any{}
	}
	return drafts, nil
}

func (s *PgStore) CreateDraft(ctx context.Context, orgID, userID, domainID string, threadID *string, kind, subject, fromAddress string, toJSON, ccJSON, bccJSON, attJSON []byte) (string, error) {
	var id string
	err := s.q.QueryRow(ctx,
		`INSERT INTO drafts (org_id, user_id, domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain, attachment_ids)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, '', '', $11)
		 RETURNING id`,
		orgID, userID, domainID, threadID, kind, subject, fromAddress,
		toJSON, ccJSON, bccJSON, attJSON,
	).Scan(&id)
	return id, err
}

func (s *PgStore) UpdateDraft(ctx context.Context, draftID, userID string, sets []string, args []any) (int64, error) {
	query := "UPDATE drafts SET " + strings.Join(sets, ", ") + " WHERE id = $1 AND user_id = $2"

	tag, err := s.q.Exec(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) DeleteDraft(ctx context.Context, draftID, userID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		"DELETE FROM drafts WHERE id = $1 AND user_id = $2",
		draftID, userID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) GetDraft(ctx context.Context, draftID, userID string) (domainID string, threadID *string, kind, subject, fromAddr, bodyHTML, bodyPlain string, toAddr, ccAddr, bccAddr, attIDsRaw json.RawMessage, err error) {
	err = s.q.QueryRow(ctx,
		`SELECT domain_id, thread_id, kind, subject, from_address,
		 to_addresses, cc_addresses, bcc_addresses, body_html, body_plain,
		 COALESCE(attachment_ids, '[]')
		 FROM drafts WHERE id = $1 AND user_id = $2`,
		draftID, userID,
	).Scan(&domainID, &threadID, &kind, &subject, &fromAddr,
		&toAddr, &ccAddr, &bccAddr, &bodyHTML, &bodyPlain, &attIDsRaw)
	return
}
