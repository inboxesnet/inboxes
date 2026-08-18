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

// BounceExpiryDays is how long a bounce block stays active. After this many
// days the address can receive mail again without manual intervention.
const BounceExpiryDays = 30

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
		`SELECT address FROM email_bounces
		 WHERE org_id = $1 AND lower(address) = ANY($2)
		   AND created_at > now() - make_interval(days => $3)`,
		orgID, normalized, BounceExpiryDays,
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
		orgID, strings.ToLower(strings.TrimSpace(domainName)),
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

func (s *PgStore) UpdateThreadStats(ctx context.Context, threadID, orgID, snippet, lastSender string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1, last_message_at = now(), snippet = $2, last_sender = $3, updated_at = now()
		 WHERE id = $1 AND org_id = $4`, threadID, snippet, lastSender, orgID,
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

// searchFilters holds the structured parts of a search query.
type searchFilters struct {
	Text          string
	From          string
	To            string
	HasAttachment bool
	Before        string
	After         string
}

// parseSearchQuery splits a raw query into structured filters and free text.
// Supported operators: from:, to:, has:attachment, before:YYYY-MM-DD,
// after:YYYY-MM-DD.
func parseSearchQuery(raw string) searchFilters {
	var f searchFilters
	var textParts []string
	for _, token := range strings.Fields(raw) {
		lower := strings.ToLower(token)
		switch {
		case strings.HasPrefix(lower, "from:") && len(token) > 5:
			f.From = token[5:]
		case strings.HasPrefix(lower, "to:") && len(token) > 3:
			f.To = token[3:]
		case lower == "has:attachment":
			f.HasAttachment = true
		case strings.HasPrefix(lower, "before:") && len(token) > 7:
			f.Before = token[7:]
		case strings.HasPrefix(lower, "after:") && len(token) > 6:
			f.After = token[6:]
		default:
			textParts = append(textParts, token)
		}
	}
	f.Text = strings.Join(textParts, " ")
	return f
}

// isSearchDate reports whether s looks like YYYY-MM-DD.
func isSearchDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func (s *PgStore) SearchEmails(ctx context.Context, orgID, query string, domainID, label, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
	if page < 1 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	f := parseSearchQuery(query)

	base := ` FROM threads t
		JOIN emails e ON e.thread_id = t.id
		WHERE e.org_id = $1 AND t.deleted_at IS NULL`
	args := []interface{}{orgID}
	argIdx := 2

	addArg := func(cond string, val interface{}) {
		base += strings.ReplaceAll(cond, "$N", "$"+strconv.Itoa(argIdx))
		args = append(args, val)
		argIdx++
	}

	if f.Text != "" {
		addArg(" AND e.search_vector @@ plainto_tsquery('english', $N)", f.Text)
	}
	if f.From != "" {
		addArg(" AND e.from_address ILIKE $N", "%"+escapeLIKE(f.From)+"%")
	}
	if f.To != "" {
		addArg(" AND (e.to_addresses::text ILIKE $N OR e.cc_addresses::text ILIKE $N)", "%"+escapeLIKE(f.To)+"%")
	}
	if f.HasAttachment {
		base += " AND COALESCE(jsonb_array_length(e.attachment_ids), 0) > 0"
	}
	if f.Before != "" && isSearchDate(f.Before) {
		addArg(" AND e.created_at < $N::date", f.Before)
	}
	if f.After != "" && isSearchDate(f.After) {
		addArg(" AND e.created_at >= $N::date", f.After)
	}
	if domainID != "" {
		addArg(" AND e.domain_id = $N", domainID)
	}

	// Optional folder scope. "archive" is label-absence, the rest are labels.
	switch label {
	case "":
		// all mail — no scope filter
	case "archive":
		base += ` AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('inbox','trash','spam'))`
	default:
		addArg(` AND EXISTS (SELECT 1 FROM thread_labels tl WHERE tl.thread_id = t.id AND tl.label = $N)`, label)
		if label != "trash" && label != "spam" {
			base += ` AND NOT EXISTS (SELECT 1 FROM thread_labels tex WHERE tex.thread_id = t.id AND tex.label IN ('trash','spam'))`
		}
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
		addArg(` AND EXISTS (SELECT 1 FROM thread_labels al WHERE al.thread_id = t.id AND al.label = ANY($N::text[]))`, labels)
	}

	var total int
	if err := s.q.QueryRow(ctx, "SELECT COUNT(DISTINCT t.id)"+base, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	sqlQuery := `SELECT DISTINCT ON (t.id)
		t.id, t.domain_id, t.subject, t.participant_emails,
		t.last_message_at, t.message_count, t.unread_count,
		t.snippet, t.last_sender, t.original_to, t.created_at` + base
	sqlQuery = "SELECT * FROM (" + sqlQuery + ") sub ORDER BY last_message_at DESC" +
		" LIMIT $" + strconv.Itoa(argIdx) + " OFFSET $" + strconv.Itoa(argIdx+1)
	args = append(args, limit, (page-1)*limit)

	rows, err := s.q.Query(ctx, sqlQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var results []map[string]any
	var threadIDs []string
	for rows.Next() {
		var id, dID, subject, snippet, lastSender string
		var originalTo *string
		var participants json.RawMessage
		var lastMessageAt, createdAt time.Time
		var messageCount, unreadCount int

		rows.Scan(&id, &dID, &subject, &participants,
			&lastMessageAt, &messageCount, &unreadCount,
			&snippet, &lastSender, &originalTo, &createdAt)

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

	return results, total, nil
}

func (s *PgStore) ListAdminJobs(ctx context.Context, orgID string) ([]map[string]any, error) {
	rows, err := s.q.Query(ctx,
		`SELECT id, job_type, status, email_id, thread_id, error_message, retry_count AS attempts, created_at, updated_at
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
	// Only live jobs block a re-send. A failed or cancelled job must not
	// lock the draft with a 409 forever.
	var exists bool
	err := s.q.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM email_jobs WHERE draft_id = $1 AND job_type = 'send'
		  AND status IN ('pending', 'running'))`,
		draftID).Scan(&exists)
	return exists, err
}
