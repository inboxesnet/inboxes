package store

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// labelsSubquery is a SQL fragment that aggregates labels for a thread.
const labelsSubquery = `(SELECT COALESCE(array_agg(tl2.label ORDER BY tl2.label), ARRAY[]::text[]) FROM thread_labels tl2 WHERE tl2.thread_id = t.id)`

func (s *PgStore) GetUserAliasAddresses(ctx context.Context, userID string) ([]string, error) {
	rows, err := s.q.Query(ctx,
		`SELECT a.address FROM aliases a
		 JOIN alias_users au ON au.alias_id = a.id
		 WHERE au.user_id = $1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var addrs []string
	for rows.Next() {
		var addr string
		if rows.Scan(&addr) == nil {
			addrs = append(addrs, addr)
		}
	}
	return addrs, rows.Err()
}

func (s *PgStore) BatchFetchLabels(ctx context.Context, threadIDs []string) (map[string][]string, error) {
	labelMap := make(map[string][]string, len(threadIDs))
	rows, err := s.q.Query(ctx,
		`SELECT thread_id, COALESCE(array_agg(label ORDER BY label), ARRAY[]::text[])
		 FROM thread_labels WHERE thread_id = ANY($1::uuid[]) GROUP BY thread_id`, threadIDs)
	if err != nil {
		return labelMap, err
	}
	defer rows.Close()
	for rows.Next() {
		var tid string
		var labels []string
		if rows.Scan(&tid, &labels) == nil {
			labelMap[tid] = labels
		}
	}
	return labelMap, rows.Err()
}

func (s *PgStore) ListThreads(ctx context.Context, orgID, label, domainID string, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
	offset := (page - 1) * limit

	var countQuery, query string
	var args []interface{}
	argIdx := 1

	switch label {
	case "archive":
		countQuery = `SELECT COUNT(*) FROM threads t
			WHERE t.org_id = $1 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label = 'inbox')
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex2 WHERE tex2.thread_id = t.id AND tex2.label IN ('trash','spam'))`
		query = `SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
			t.last_message_at, t.message_count, t.unread_count, t.snippet, t.last_sender, t.original_to, t.created_at,
			t.trash_expires_at
			FROM threads t
			WHERE t.org_id = $1 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label = 'inbox')
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex2 WHERE tex2.thread_id = t.id AND tex2.label IN ('trash','spam'))`
		args = append(args, orgID)
		argIdx = 2

	case "trash", "spam":
		countQuery = `SELECT COUNT(*) FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL`
		query = `SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
			t.last_message_at, t.message_count, t.unread_count, t.snippet, t.last_sender, t.original_to, t.created_at,
			t.trash_expires_at
			FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL`
		args = append(args, orgID, label)
		argIdx = 3

	default:
		countQuery = `SELECT COUNT(*) FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))`
		query = `SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
			t.last_message_at, t.message_count, t.unread_count, t.snippet, t.last_sender, t.original_to, t.created_at,
			t.trash_expires_at
			FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))`
		args = append(args, orgID, label)
		argIdx = 3
	}

	if domainID != "" {
		domainFilter := " AND t.domain_id = $" + strconv.Itoa(argIdx)
		countQuery += domainFilter
		query += domainFilter
		args = append(args, domainID)
		argIdx++
	}

	// Alias visibility: non-admins only see threads related to their aliases
	if role != "admin" {
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		labels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			labels[i] = "alias:" + addr
		}
		vis := ` AND EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = t.id AND al.label = ANY($` + strconv.Itoa(argIdx) + `::text[]))`
		countQuery += vis
		query += vis
		args = append(args, labels)
		argIdx++
	}

	// Count total
	var total int
	warnIfErr(s.q.QueryRow(ctx, countQuery, args...).Scan(&total), "threads: count query failed")

	// Add ORDER BY + pagination
	query += " ORDER BY t.last_message_at DESC LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, offset)

	rows, err := s.q.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("threads: query failed: %w", err)
	}
	defer rows.Close()

	var threads []map[string]any
	var threadIDs []string
	for rows.Next() {
		var id, oID, userID, dID, subject, snippet, lastSender string
		var originalTo *string
		var trashExpiresAt *time.Time
		var participants json.RawMessage
		var lastMessageAt, createdAt time.Time
		var messageCount, unreadCount int

		err := rows.Scan(&id, &oID, &userID, &dID, &subject, &participants,
			&lastMessageAt, &messageCount, &unreadCount, &snippet, &lastSender, &originalTo, &createdAt, &trashExpiresAt)
		if err != nil {
			continue
		}
		t := map[string]any{
			"id":                 id,
			"domain_id":          dID,
			"subject":            subject,
			"participant_emails": participants,
			"last_message_at":    lastMessageAt,
			"message_count":      messageCount,
			"unread_count":       unreadCount,
			"snippet":            snippet,
			"last_sender":        lastSender,
			"created_at":         createdAt,
		}
		if originalTo != nil {
			t["original_to"] = *originalTo
		}
		if trashExpiresAt != nil {
			t["trash_expires_at"] = *trashExpiresAt
		}
		threads = append(threads, t)
		threadIDs = append(threadIDs, id)
	}
	if threads == nil {
		threads = []map[string]any{}
	}

	// Batch-fetch labels for all threads in one query
	if len(threadIDs) > 0 {
		labelMap, _ := s.BatchFetchLabels(ctx, threadIDs)
		for _, t := range threads {
			tid := t["id"].(string)
			if lbls, ok := labelMap[tid]; ok {
				t["labels"] = lbls
			} else {
				t["labels"] = []string{}
			}
		}
	}

	return threads, total, nil
}

func (s *PgStore) GetThread(ctx context.Context, threadID, orgID string) (map[string]any, error) {
	var id, oID, userID, domainID, subject, snippet, lastSender string
	var participants json.RawMessage
	var trashExpiresAt *time.Time
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var labels []string

	err := s.q.QueryRow(ctx,
		`SELECT t.id, t.org_id, t.user_id, t.domain_id, t.subject, t.participant_emails,
		 t.last_message_at, t.message_count, t.unread_count, t.snippet, t.last_sender, t.created_at,
		 t.trash_expires_at,
		 `+labelsSubquery+` as labels
		 FROM threads t WHERE t.id = $1 AND t.org_id = $2`,
		threadID, orgID,
	).Scan(&id, &oID, &userID, &domainID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &snippet, &lastSender, &createdAt, &trashExpiresAt, &labels)
	if err != nil {
		return nil, err
	}
	if labels == nil {
		labels = []string{}
	}

	t := map[string]any{
		"id":                 id,
		"domain_id":          domainID,
		"subject":            subject,
		"participant_emails": participants,
		"last_message_at":    lastMessageAt,
		"message_count":      messageCount,
		"unread_count":       unreadCount,
		"labels":             labels,
		"snippet":            snippet,
		"last_sender":        lastSender,
		"created_at":         createdAt,
	}
	if trashExpiresAt != nil {
		t["trash_expires_at"] = *trashExpiresAt
	}
	return t, nil
}

func (s *PgStore) GetThreadEmails(ctx context.Context, threadID, orgID string) ([]map[string]any, error) {
	emailRows, err := s.q.Query(ctx,
		`SELECT id, direction, from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses, subject,
		 body_html, body_plain, status, attachments, message_id, in_reply_to, references_header,
		 delivered_via_alias, sent_as_alias, is_read, created_at
		 FROM emails WHERE thread_id = $1 AND org_id = $2 ORDER BY created_at ASC`, threadID, orgID,
	)
	if err != nil {
		return nil, err
	}
	defer emailRows.Close()

	var emails []map[string]any
	for emailRows.Next() {
		var eID, dir, from, eSubject, eStatus string
		var eTo, eCC, eBCC, eReplyTo json.RawMessage
		var bodyHTML, bodyPlain, messageID, inReplyTo *string
		var deliveredViaAlias, sentAsAlias *string
		var attachments, refsHeader json.RawMessage
		var isRead bool
		var eCreatedAt time.Time

		emailRows.Scan(&eID, &dir, &from, &eTo, &eCC, &eBCC, &eReplyTo, &eSubject,
			&bodyHTML, &bodyPlain, &eStatus, &attachments, &messageID, &inReplyTo, &refsHeader,
			&deliveredViaAlias, &sentAsAlias, &isRead, &eCreatedAt)

		email := map[string]any{
			"id":                 eID,
			"direction":          dir,
			"from_address":       from,
			"to_addresses":       eTo,
			"cc_addresses":       eCC,
			"bcc_addresses":      eBCC,
			"reply_to_addresses": eReplyTo,
			"subject":            eSubject,
			"status":             eStatus,
			"attachments":        attachments,
			"is_read":            isRead,
			"created_at":         eCreatedAt,
		}
		if bodyHTML != nil {
			email["body_html"] = *bodyHTML
		}
		if bodyPlain != nil {
			email["body_plain"] = *bodyPlain
		}
		if messageID != nil {
			email["message_id"] = *messageID
		}
		if inReplyTo != nil {
			email["in_reply_to"] = *inReplyTo
		}
		if refsHeader != nil {
			email["references"] = refsHeader
		}
		if deliveredViaAlias != nil {
			email["delivered_via_alias"] = *deliveredViaAlias
		}
		if sentAsAlias != nil {
			email["sent_as_alias"] = *sentAsAlias
		}
		emails = append(emails, email)
	}
	if emails == nil {
		emails = []map[string]any{}
	}
	return emails, nil
}

func (s *PgStore) CheckThreadVisibility(ctx context.Context, threadID string, aliasLabels []string) (bool, error) {
	var visible bool
	err := s.q.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = $1 AND al.label = ANY($2::text[]))`,
		threadID, aliasLabels,
	).Scan(&visible)
	return visible, err
}

func (s *PgStore) GetThreadDomainID(ctx context.Context, threadID, orgID string) (string, error) {
	var domainID string
	err := s.q.QueryRow(ctx,
		"SELECT domain_id FROM threads WHERE id = $1 AND org_id = $2",
		threadID, orgID,
	).Scan(&domainID)
	return domainID, err
}

func (s *PgStore) FetchThreadSummary(ctx context.Context, threadID, orgID string) (map[string]any, error) {
	var id, dID, subject, snippet, lastSender string
	var originalTo *string
	var participants json.RawMessage
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var labels []string

	err := s.q.QueryRow(ctx,
		`SELECT t.id, t.domain_id, t.subject, t.participant_emails,
		 t.last_message_at, t.message_count, t.unread_count, t.snippet, t.last_sender, t.original_to, t.created_at,
		 `+labelsSubquery+` as labels
		 FROM threads t WHERE t.id = $1 AND t.org_id = $2`,
		threadID, orgID,
	).Scan(&id, &dID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &snippet, &lastSender, &originalTo, &createdAt, &labels)
	if err != nil {
		return nil, err
	}
	if labels == nil {
		labels = []string{}
	}
	t := map[string]any{
		"id":                 id,
		"domain_id":          dID,
		"subject":            subject,
		"participant_emails": participants,
		"last_message_at":    lastMessageAt,
		"message_count":      messageCount,
		"unread_count":       unreadCount,
		"labels":             labels,
		"snippet":            snippet,
		"last_sender":        lastSender,
		"created_at":         createdAt,
	}
	if originalTo != nil {
		t["original_to"] = *originalTo
	}
	return t, nil
}

func (s *PgStore) UpdateThreadUnread(ctx context.Context, threadID, orgID string, unreadCount int) (int64, error) {
	tag, err := s.q.Exec(ctx,
		"UPDATE threads SET unread_count = $3, updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, orgID, unreadCount,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) MarkAllEmailsRead(ctx context.Context, threadID, orgID string) error {
	_, err := s.q.Exec(ctx,
		"UPDATE emails SET is_read = true WHERE thread_id = $1 AND org_id = $2",
		threadID, orgID,
	)
	return err
}

func (s *PgStore) MarkLatestEmailUnread(ctx context.Context, threadID, orgID string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE emails SET is_read = false WHERE id = (
		  SELECT id FROM emails WHERE thread_id = $1 AND org_id = $2 ORDER BY created_at DESC LIMIT 1
		)`, threadID, orgID)
	return err
}

func (s *PgStore) SoftDeleteThread(ctx context.Context, threadID, orgID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		"UPDATE threads SET deleted_at = now(), updated_at = now() WHERE id = $1 AND org_id = $2",
		threadID, orgID,
	)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) SetTrashExpiry(ctx context.Context, threadIDs []string, orgID string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE threads SET trash_expires_at = now() + interval '30 days', updated_at = now()
		 WHERE id = ANY($1::uuid[]) AND org_id = $2`, threadIDs, orgID)
	return err
}

func (s *PgStore) BulkUpdateUnread(ctx context.Context, threadIDs []string, orgID string, unreadCount int) (int64, error) {
	tag, err := s.q.Exec(ctx,
		"UPDATE threads SET unread_count = $3, updated_at = now() WHERE id = ANY($1::uuid[]) AND org_id = $2",
		threadIDs, orgID, unreadCount)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) FilterTrashThreadIDs(ctx context.Context, threadIDs []string) ([]string, error) {
	rows, err := s.q.Query(ctx,
		`SELECT DISTINCT thread_id FROM thread_labels
		 WHERE thread_id = ANY($1::uuid[]) AND label = 'trash'`,
		threadIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var trashIDs []string
	for rows.Next() {
		var tid string
		if rows.Scan(&tid) == nil {
			trashIDs = append(trashIDs, tid)
		}
	}
	return trashIDs, rows.Err()
}

func (s *PgStore) BulkSoftDelete(ctx context.Context, threadIDs []string, orgID string) (int64, error) {
	tag, err := s.q.Exec(ctx,
		`UPDATE threads SET deleted_at = now(), updated_at = now()
		 WHERE id = ANY($1::uuid[]) AND org_id = $2`,
		threadIDs, orgID)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s *PgStore) ResolveFilteredThreadIDs(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string) ([]string, error) {
	var query string
	var args []interface{}
	argIdx := 1

	switch label {
	case "archive":
		query = `SELECT t.id FROM threads t
			WHERE t.org_id = $1 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label = 'inbox')
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex2 WHERE tex2.thread_id = t.id AND tex2.label IN ('trash','spam'))`
		args = append(args, orgID)
		argIdx = 2
	case "trash", "spam":
		query = `SELECT t.id FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL`
		args = append(args, orgID, label)
		argIdx = 3
	default:
		query = `SELECT t.id FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id
			WHERE tl.org_id = $1 AND tl.label = $2 AND t.deleted_at IS NULL
			AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))`
		args = append(args, orgID, label)
		argIdx = 3
	}

	if domainID != "" {
		query += " AND t.domain_id = $" + strconv.Itoa(argIdx)
		args = append(args, domainID)
		argIdx++
	}

	if role != "admin" {
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		labels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			labels[i] = "alias:" + addr
		}
		query += ` AND EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = t.id AND al.label = ANY($` + strconv.Itoa(argIdx) + `::text[]))`
		args = append(args, labels)
	}

	query += " LIMIT 10001"

	rows, err := s.q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) > 10000 {
		return nil, fmt.Errorf("too many threads selected; please narrow your filter")
	}
	return ids, nil
}

func (s *PgStore) CreateThread(ctx context.Context, orgID, userID, domainID, subject string, participantsJSON []byte, snippet, lastSender string) (string, error) {
	var threadID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, snippet, last_sender)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		orgID, userID, domainID, subject, participantsJSON, snippet, lastSender,
	).Scan(&threadID)
	return threadID, err
}

// ---- Label operations ----

func (s *PgStore) AddLabel(ctx context.Context, threadID, orgID, label string) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO thread_labels (thread_id, org_id, label) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
		threadID, orgID, label)
	return err
}

func (s *PgStore) RemoveLabel(ctx context.Context, threadID, label string) error {
	_, err := s.q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id = $1 AND label = $2`,
		threadID, label)
	return err
}

func (s *PgStore) RemoveAllLabels(ctx context.Context, threadID string) error {
	_, err := s.q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id = $1`,
		threadID)
	return err
}

func (s *PgStore) HasLabel(ctx context.Context, threadID, label string) bool {
	var exists bool
	if err := s.q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM thread_labels WHERE thread_id = $1 AND label = $2)`,
		threadID, label).Scan(&exists); err != nil {
		slog.Warn("HasLabel: query failed", "thread_id", threadID, "label", label, "error", err)
		return false
	}
	return exists
}

func (s *PgStore) GetLabels(ctx context.Context, threadID string) []string {
	rows, err := s.q.Query(ctx,
		`SELECT label FROM thread_labels WHERE thread_id = $1 ORDER BY label`, threadID)
	if err != nil {
		return nil
	}
	defer rows.Close()
	var labels []string
	for rows.Next() {
		var l string
		if rows.Scan(&l) == nil {
			labels = append(labels, l)
		}
	}
	if labels == nil {
		labels = []string{}
	}
	return labels
}

func (s *PgStore) BulkAddLabel(ctx context.Context, threadIDs []string, orgID, label string) error {
	_, err := s.q.Exec(ctx,
		`INSERT INTO thread_labels (thread_id, org_id, label)
		 SELECT unnest($1::uuid[]), $2, $3
		 ON CONFLICT DO NOTHING`,
		threadIDs, orgID, label)
	return err
}

func (s *PgStore) BulkRemoveLabel(ctx context.Context, threadIDs []string, label string) error {
	_, err := s.q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id = ANY($1::uuid[]) AND label = $2`,
		threadIDs, label)
	return err
}
