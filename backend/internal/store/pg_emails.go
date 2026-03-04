package store

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

func (s *PgStore) LoadAttachmentsForResend(ctx context.Context, ids []string, orgID string) ([]map[string]string, error) {
	rows, err := s.q.Query(ctx,
		`SELECT filename, content_type, data FROM attachments WHERE id = ANY($1) AND org_id = $2`,
		ids, orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var attachments []map[string]string
	for rows.Next() {
		var filename, contentType string
		var data []byte
		if rows.Scan(&filename, &contentType, &data) == nil {
			attachments = append(attachments, map[string]string{
				"content":  base64.StdEncoding.EncodeToString(data),
				"filename": filename,
			})
		}
	}
	return attachments, rows.Err()
}

func (s *PgStore) CheckBouncedRecipients(ctx context.Context, orgID string, addresses []string) ([]string, error) {
	if len(addresses) == 0 {
		return nil, nil
	}

	// Normalize addresses
	normalized := make([]string, len(addresses))
	for i, addr := range addresses {
		normalized[i] = strings.ToLower(strings.TrimSpace(addr))
	}

	rows, err := s.q.Query(ctx,
		`SELECT address FROM email_bounces WHERE org_id = $1 AND lower(address) = ANY($2)`,
		orgID, normalized,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var blocked []string
	for rows.Next() {
		var addr string
		if rows.Scan(&addr) == nil {
			blocked = append(blocked, addr)
		}
	}
	return blocked, rows.Err()
}

func (s *PgStore) CanSendAs(ctx context.Context, userID, orgID, fromAddress, role string) (bool, error) {
	if role == "admin" {
		return true, nil
	}

	// Check if sending from own email
	var userEmail string
	err := s.q.QueryRow(ctx,
		"SELECT email FROM users WHERE id = $1 AND org_id = $2 AND status = 'active'",
		userID, orgID,
	).Scan(&userEmail)
	if err == nil && strings.EqualFold(userEmail, fromAddress) {
		return true, nil
	}

	// Check alias_users.can_send_as
	var allowed bool
	if err := s.q.QueryRow(ctx,
		`SELECT EXISTS(
			SELECT 1 FROM alias_users au
			JOIN aliases a ON a.id = au.alias_id
			JOIN domains d ON d.id = a.domain_id
			WHERE au.user_id = $1 AND a.org_id = $2 AND a.address = $3 AND au.can_send_as = true
			AND d.status NOT IN ('disconnected', 'pending', 'deleted')
		)`,
		userID, orgID, fromAddress,
	).Scan(&allowed); err != nil {
		slog.Warn("CanSendAs: alias check failed", "user_id", userID, "address", fromAddress, "error", err)
		return false, nil
	}
	return allowed, nil
}

func (s *PgStore) GetDomainStatus(ctx context.Context, domainID, orgID string) (string, error) {
	var status string
	err := s.q.QueryRow(ctx,
		"SELECT status FROM domains WHERE id = $1 AND org_id = $2",
		domainID, orgID,
	).Scan(&status)
	return status, err
}

func (s *PgStore) ResolveFromDisplay(ctx context.Context, orgID, address string) (string, error) {
	var name string
	err := s.q.QueryRow(ctx,
		"SELECT name FROM aliases WHERE org_id = $1 AND address = $2 AND name != ''",
		orgID, address,
	).Scan(&name)
	if err == nil && name != "" {
		return fmt.Sprintf("%s <%s>", name, address), nil
	}

	err = s.q.QueryRow(ctx,
		"SELECT name FROM users WHERE org_id = $1 AND email = $2 AND name != '' AND status = 'active'",
		orgID, address,
	).Scan(&name)
	if err == nil && name != "" {
		return fmt.Sprintf("%s <%s>", name, address), nil
	}

	return address, nil
}

func (s *PgStore) LookupDomainByName(ctx context.Context, orgID, domainName string) (string, error) {
	var domainID string
	err := s.q.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
		orgID, domainName,
	).Scan(&domainID)
	return domainID, err
}

func (s *PgStore) InsertEmail(ctx context.Context, threadID, userID, orgID, domainID, direction, from string, toJSON, ccJSON, bccJSON []byte, subject, bodyHTML, bodyPlain, status string, inReplyTo string, refsJSON []byte) (string, error) {
	var emailID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, direction,
		 from_address, to_addresses, cc_addresses, bcc_addresses, subject, body_html, body_plain,
		 status, in_reply_to, references_header)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 RETURNING id`,
		threadID, userID, orgID, domainID, direction,
		from, toJSON, ccJSON, bccJSON, subject, bodyHTML, bodyPlain,
		status, inReplyTo, refsJSON,
	).Scan(&emailID)
	return emailID, err
}

func (s *PgStore) UpdateThreadStats(ctx context.Context, threadID, snippet string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1, last_message_at = now(), snippet = $2, updated_at = now()
		 WHERE id = $1`, threadID, snippet,
	)
	return err
}

func (s *PgStore) CreateEmailJob(ctx context.Context, orgID, userID, domainID, jobType, emailID, threadID string, resendPayload []byte, draftID *string) (string, error) {
	var jobID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO email_jobs (org_id, user_id, domain_id, job_type, email_id, thread_id, resend_payload, draft_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING id`,
		orgID, userID, domainID, jobType, emailID, threadID, resendPayload, draftID,
	).Scan(&jobID)
	return jobID, err
}

func (s *PgStore) SearchEmails(ctx context.Context, orgID, query string, domainID, role string, aliasAddrs []string) ([]map[string]any, error) {
	sqlQuery := `SELECT DISTINCT ON (t.id)
		t.id, t.domain_id, t.subject, t.participant_emails,
		t.last_message_at, t.message_count, t.unread_count,
		t.snippet, t.original_to, t.created_at
		FROM threads t
		JOIN emails e ON e.thread_id = t.id
		WHERE e.org_id = $1 AND e.search_vector @@ plainto_tsquery('english', $2)
		AND t.deleted_at IS NULL`
	args := []interface{}{orgID, query}
	argIdx := 3

	if domainID != "" {
		sqlQuery += " AND e.domain_id = $" + strconv.Itoa(argIdx)
		args = append(args, domainID)
		argIdx++
	}

	// Alias visibility filter for non-admins
	if role != "admin" {
		if aliasAddrs == nil {
			aliasAddrs = []string{}
		}
		labels := make([]string, len(aliasAddrs))
		for i, addr := range aliasAddrs {
			labels[i] = "alias:" + addr
		}
		sqlQuery += ` AND EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = t.id AND al.label = ANY($` + strconv.Itoa(argIdx) + `::text[]))`
		args = append(args, labels)
	}

	sqlQuery = "SELECT * FROM (" + sqlQuery + ") sub ORDER BY last_message_at DESC LIMIT 50"

	rows, err := s.q.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]any
	var threadIDs []string
	for rows.Next() {
		var id, dID, subject, snippet string
		var originalTo *string
		var participants json.RawMessage
		var lastMessageAt, createdAt time.Time
		var messageCount, unreadCount int

		rows.Scan(&id, &dID, &subject, &participants,
			&lastMessageAt, &messageCount, &unreadCount,
			&snippet, &originalTo, &createdAt)

		t := map[string]any{
			"id":                 id,
			"domain_id":          dID,
			"subject":            subject,
			"participant_emails": participants,
			"last_message_at":    lastMessageAt,
			"message_count":      messageCount,
			"unread_count":       unreadCount,
			"snippet":            snippet,
			"created_at":         createdAt,
		}
		if originalTo != nil {
			t["original_to"] = *originalTo
		}
		results = append(results, t)
		threadIDs = append(threadIDs, id)
	}
	if results == nil {
		results = []map[string]any{}
	}

	// Batch-fetch labels for all result threads
	if len(threadIDs) > 0 {
		labelMap, _ := s.BatchFetchLabels(ctx, threadIDs)
		for _, t := range results {
			tid := t["id"].(string)
			if lbls, ok := labelMap[tid]; ok {
				t["labels"] = lbls
			} else {
				t["labels"] = []string{}
			}
		}
	}

	return results, nil
}

func (s *PgStore) ListAdminJobs(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, job_type, status, email_id, thread_id, error_message, attempts, created_at, updated_at
		 FROM email_jobs WHERE org_id = $1
		 ORDER BY created_at DESC LIMIT 100`,
		orgID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []map[string]any
	for rows.Next() {
		var id, jobType, status string
		var emailID, threadID, errorMsg *string
		var attempts int
		var createdAt, updatedAt time.Time

		if rows.Scan(&id, &jobType, &status, &emailID, &threadID, &errorMsg, &attempts, &createdAt, &updatedAt) == nil {
			job := map[string]any{
				"id":         id,
				"job_type":   jobType,
				"status":     status,
				"attempts":   attempts,
				"created_at": createdAt,
				"updated_at": updatedAt,
			}
			if emailID != nil {
				job["email_id"] = *emailID
			}
			if threadID != nil {
				job["thread_id"] = *threadID
			}
			if errorMsg != nil {
				job["error_message"] = *errorMsg
			}
			jobs = append(jobs, job)
		}
	}
	if jobs == nil {
		jobs = []map[string]any{}
	}
	return jobs, nil
}

func (s *PgStore) CheckSendJobExists(ctx context.Context, draftID string) (bool, error) {
	var exists bool
	err := s.q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM email_jobs WHERE draft_id = $1 AND job_type = 'send')`,
		draftID).Scan(&exists)
	return exists, err
}
