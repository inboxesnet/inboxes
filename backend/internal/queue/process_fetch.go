package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
)

// webhookEmailData mirrors the webhook payload structure for email.received.
type webhookEmailData struct {
	EmailID   string            `json:"email_id"`
	From      string            `json:"from"`
	To        []string          `json:"to"`
	CC        []string          `json:"cc"`
	BCC       []string          `json:"bcc"`
	ReplyTo   []string          `json:"reply_to"`
	Subject   string            `json:"subject"`
	HTML      string            `json:"html"`
	Text      string            `json:"text"`
	MessageID string            `json:"message_id"`
	Headers   map[string]string `json:"headers"`
	CreatedAt string            `json:"created_at"`
}

func (w *EmailWorker) processFetch(ctx context.Context, jobID, orgID, userID string) error {
	var resendEmailID string
	var webhookDataRaw []byte
	err := w.store.Q().QueryRow(ctx,
		`SELECT resend_email_id, webhook_data FROM email_jobs WHERE id = $1`,
		jobID,
	).Scan(&resendEmailID, &webhookDataRaw)
	if err != nil {
		return fmt.Errorf("load fetch job: %w", err)
	}

	var emailData webhookEmailData
	if err := json.Unmarshal(webhookDataRaw, &emailData); err != nil {
		return fmt.Errorf("parse webhook data: %w", err)
	}

	// Idempotency: skip if we already have this email
	var existingID string
	if err := w.store.Q().QueryRow(ctx,
		"SELECT id FROM emails WHERE resend_email_id = $1", resendEmailID,
	).Scan(&existingID); err == nil {
		slog.Info("email worker: duplicate email, skipping", "resend_email_id", resendEmailID)
		return nil
	}

	// Determine domain from TO/CC/BCC addresses (single batch query)
	var domainID string
	var recipientAddress string

	// Collect unique domain names from all recipients
	allRecipients := make([]string, 0, len(emailData.To)+len(emailData.CC)+len(emailData.BCC))
	allRecipients = append(allRecipients, emailData.To...)
	allRecipients = append(allRecipients, emailData.CC...)
	allRecipients = append(allRecipients, emailData.BCC...)

	uniqueDomains := make(map[string]bool)
	var domainNames []string
	for _, addr := range allRecipients {
		parts := strings.Split(addr, "@")
		if len(parts) != 2 {
			continue
		}
		d := parts[1]
		if !uniqueDomains[d] {
			uniqueDomains[d] = true
			domainNames = append(domainNames, d)
		}
	}

	// Single query for all domain lookups
	domainMap := make(map[string]string) // domain name -> domain ID
	if len(domainNames) > 0 {
		rows, err := w.store.Q().Query(ctx,
			"SELECT id, domain FROM domains WHERE org_id = $1 AND domain = ANY($2) AND status = 'active' AND hidden = false",
			orgID, domainNames,
		)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var dID, dName string
				if rows.Scan(&dID, &dName) == nil {
					domainMap[dName] = dID
				}
			}
			rows.Close()
		}
	}

	// Match in priority order: To > CC > BCC
	for _, addr := range allRecipients {
		parts := strings.Split(addr, "@")
		if len(parts) == 2 {
			if dID, ok := domainMap[parts[1]]; ok {
				domainID = dID
				recipientAddress = addr
				break
			}
		}
	}
	if domainID == "" {
		w.bus.Publish(ctx, event.Event{
			EventType: event.DomainNotFound,
			OrgID:     orgID,
			Payload:   map[string]interface{}{"domains": domainNames},
		})
		return &nonRetryableError{fmt.Errorf("no matching domain for email to=%v cc=%v bcc=%v", emailData.To, emailData.CC, emailData.BCC)}
	}

	// Fetch full email from Resend API
	type resendFullEmail struct {
		HTML        string            `json:"html"`
		Text        string            `json:"text"`
		Headers     map[string]string `json:"headers"`
		ReplyTo     []string          `json:"reply_to"`
		Attachments []struct {
			Filename    string `json:"filename"`
			ContentType string `json:"content_type"`
			Size        int    `json:"size"`
			URL         string `json:"url"`
		} `json:"attachments"`
	}

	var full resendFullEmail
	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		return err
	}
	bodyData, err := w.resendSvc.Fetch(ctx, orgID, "GET", "/emails/receiving/"+resendEmailID, nil)
	if err != nil {
		slog.Error("email worker: fetch full email from Resend", "error", err, "email_id", resendEmailID)
		// Fall back to webhook data
	} else {
		if err := json.Unmarshal(bodyData, &full); err != nil {
			slog.Error("email worker: parse full email response", "error", err)
		}
	}

	// Merge fetched data — prefer API response over webhook payload
	bodyHTML := full.HTML
	bodyPlain := full.Text
	headers := full.Headers
	if headers == nil {
		headers = emailData.Headers
	}

	// Normalize header keys to lowercase for consistent lookups
	if headers != nil {
		normalized := make(map[string]string, len(headers))
		for k, v := range headers {
			normalized[strings.ToLower(k)] = v
		}
		headers = normalized
	}
	replyTo := full.ReplyTo
	if replyTo == nil {
		replyTo = emailData.ReplyTo
	}

	// 3-phase routing
	routedUserID := w.routeEmail(ctx, orgID, domainID, recipientAddress)
	if routedUserID == "" {
		slog.Warn("email worker: no user found for", "address", recipientAddress)
		return fmt.Errorf("no user found for address %s", recipientAddress)
	}

	// Download inbound attachments and store in DB
	var downloadedAttachmentIDs []string
	for _, att := range full.Attachments {
		if att.URL == "" {
			continue
		}
		attID, dlErr := w.downloadAndStoreAttachment(ctx, orgID, routedUserID, att.URL, att.Filename, att.ContentType)
		if dlErr != nil {
			slog.Error("email worker: download attachment failed", "filename", att.Filename, "error", dlErr)
			continue
		}
		downloadedAttachmentIDs = append(downloadedAttachmentIDs, attID)
	}

	// Spam classification
	spamResult := service.ClassifySpam(headers, emailData.From, emailData.Subject, bodyPlain)
	isSpam := spamResult.IsSpam
	if isSpam {
		slog.Info("email worker: email classified as spam", "from", emailData.From, "score", spamResult.Score)
	}

	// Bounce detection
	isBounce := service.IsBounceNotification(emailData.From, headers)
	if isBounce {
		slog.Info("email worker: bounce notification detected", "from", emailData.From)
	}

	// Threading
	cleanSubject := util.CleanSubjectLine(emailData.Subject)
	snippet := util.TruncateRunes(bodyPlain, 200)

	// Extract threading headers
	var inReplyTo string
	if headers != nil {
		inReplyTo = headers["in-reply-to"]
	}

	toJSON, err := json.Marshal(emailData.To)
	if err != nil {
		return fmt.Errorf("marshal to addresses: %w", err)
	}
	ccJSON, err := json.Marshal(emailData.CC)
	if err != nil {
		return fmt.Errorf("marshal cc addresses: %w", err)
	}
	bccJSON, err := json.Marshal(emailData.BCC)
	if err != nil {
		return fmt.Errorf("marshal bcc addresses: %w", err)
	}
	replyToJSON, err := json.Marshal(replyTo)
	if err != nil {
		return fmt.Errorf("marshal reply-to addresses: %w", err)
	}
	headersJSON, err := json.Marshal(headers)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}
	spamReasons, err := json.Marshal(spamResult.Reasons)
	if err != nil {
		return fmt.Errorf("marshal spam reasons: %w", err)
	}

	attachmentsJSON := []byte("[]")
	if len(full.Attachments) > 0 {
		attachmentsJSON, err = json.Marshal(full.Attachments)
		if err != nil {
			return fmt.Errorf("marshal attachments: %w", err)
		}
	}

	attachmentIDsJSON := []byte("[]")
	if len(downloadedAttachmentIDs) > 0 {
		attachmentIDsJSON, err = json.Marshal(downloadedAttachmentIDs)
		if err != nil {
			return fmt.Errorf("marshal attachment IDs: %w", err)
		}
	}

	var refsJSON []byte
	if headers != nil {
		if refs, ok := headers["references"]; ok {
			refsJSON, err = json.Marshal(strings.Fields(refs))
			if err != nil {
				return fmt.Errorf("marshal references: %w", err)
			}
		}
	}
	if refsJSON == nil {
		refsJSON = []byte("[]")
	}

	// Determine delivered_via_alias
	var deliveredViaAlias *string
	var aliasAddr string
	if err := w.store.Q().QueryRow(ctx,
		`SELECT address FROM aliases WHERE org_id=$1 AND address=$2 AND deleted_at IS NULL`,
		orgID, strings.ToLower(strings.TrimSpace(recipientAddress)),
	).Scan(&aliasAddr); err == nil {
		deliveredViaAlias = &aliasAddr
	}

	// BEGIN TRANSACTION
	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	tx, err := w.store.Pool().Begin(dbCtx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(dbCtx)

	// Find or create thread (3-step matching)
	var threadID string
	found := false

	// Step 1: Match by In-Reply-To header
	if inReplyTo != "" {
		err := tx.QueryRow(dbCtx,
			`SELECT thread_id FROM emails WHERE message_id = $1 AND org_id = $2 LIMIT 1`,
			inReplyTo, orgID,
		).Scan(&threadID)
		if err == nil {
			found = true
		}
	}

	// Step 1.5: Match by References header
	if !found && headers != nil {
		if refs, ok := headers["references"]; ok {
			refIDs := strings.Fields(refs)
			if len(refIDs) > 0 {
				err := tx.QueryRow(dbCtx,
					`SELECT thread_id FROM emails WHERE message_id = ANY($1::text[]) AND org_id = $2
					 ORDER BY created_at DESC LIMIT 1`,
					refIDs, orgID,
				).Scan(&threadID)
				if err == nil {
					found = true
				}
			}
		}
	}

	// Step 2: Match by subject + counterparty (within 90-day window)
	// Skip when subject is empty to prevent unrelated no-subject emails from merging
	if !found && cleanSubject != "" {
		fromClean := service.ExtractEmail(emailData.From)
		counterpartyJSON, marshalErr := json.Marshal([]string{fromClean})
		if marshalErr != nil {
			return fmt.Errorf("marshal counterparty: %w", marshalErr)
		}
		err := tx.QueryRow(dbCtx,
			`SELECT id FROM threads WHERE org_id = $1 AND subject = $2
			 AND participant_emails @> $3::jsonb AND last_message_at > now() - interval '90 days'
			 ORDER BY last_message_at DESC LIMIT 1`,
			orgID, cleanSubject, counterpartyJSON,
		).Scan(&threadID)
		if err == nil {
			found = true
		}
	}

	// Step 3: Create new thread
	if !found {
		participants, marshalErr := json.Marshal(append([]string{emailData.From}, emailData.To...))
		if marshalErr != nil {
			return fmt.Errorf("marshal participants: %w", marshalErr)
		}
		if err := tx.QueryRow(dbCtx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, original_to, snippet)
			 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
			orgID, routedUserID, domainID, cleanSubject, participants, recipientAddress, snippet,
		).Scan(&threadID); err != nil {
			return fmt.Errorf("create thread: %w", err)
		}
		// Add appropriate label for new thread
		if isSpam {
			addLabelQ(dbCtx, tx, threadID, orgID, "spam")
		} else if isBounce {
			addLabelQ(dbCtx, tx, threadID, orgID, "trash")
		} else {
			addLabelQ(dbCtx, tx, threadID, orgID, "inbox")
		}
	}

	// INSERT email
	var emailID string
	if err := tx.QueryRow(dbCtx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, message_id,
		 direction, from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
		 subject, body_html, body_plain, status, headers, in_reply_to, references_header,
		 spam_score, spam_reasons, attachments, attachment_ids, delivered_via_alias)
		 VALUES ($1, $2, $3, $4, $5, $6, 'inbound', $7, $8, $9, $10, $11, $12, $13, $14, 'received',
		 $15, $16, $17, $18, $19, $20, $21, $22)
		 RETURNING id`,
		threadID, routedUserID, orgID, domainID, resendEmailID, emailData.MessageID,
		emailData.From, toJSON, ccJSON, bccJSON, replyToJSON,
		emailData.Subject, bodyHTML, bodyPlain,
		headersJSON, inReplyTo, refsJSON, spamResult.Score, spamReasons, attachmentsJSON,
		attachmentIDsJSON, deliveredViaAlias,
	).Scan(&emailID); err != nil {
		return fmt.Errorf("insert email: %w", err)
	}

	// UPDATE thread stats + merge participant_emails
	if _, err := tx.Exec(dbCtx,
		`UPDATE threads SET message_count = message_count + 1, unread_count = unread_count + 1,
		 last_message_at = now(), snippet = $2, updated_at = now(),
		 participant_emails = (
		   SELECT jsonb_agg(DISTINCT val) FROM (
		     SELECT jsonb_array_elements(participant_emails) AS val
		     UNION
		     SELECT jsonb_array_elements($3::jsonb) AS val
		   ) sub
		 )
		 WHERE id = $1`, threadID, snippet, toJSON,
	); err != nil {
		return fmt.Errorf("update thread: %w", err)
	}

	// Add inbox label for existing threads on inbound reply (idempotent).
	// Spam/bounce threads stay out of inbox — don't auto-rescue.
	// Muted threads: skip re-adding inbox label so they don't resurface.
	if found && !isSpam && !isBounce {
		if !hasLabelQ(dbCtx, tx, threadID, "muted") {
			addLabelQ(dbCtx, tx, threadID, orgID, "inbox")
		}
		// Remove trash if thread was trashed — reply un-trashes it
		removeLabelQ(dbCtx, tx, threadID, "trash")
	}

	// Auto-create aliases for own-domain recipient addresses so every address
	// that receives mail gets an alias for label stamping and sidebar visibility.
	for _, addr := range allRecipients {
		cleanAddr := strings.ToLower(strings.TrimSpace(addr))
		addrParts := strings.Split(cleanAddr, "@")
		if len(addrParts) == 2 {
			if dID, ok := domainMap[addrParts[1]]; ok {
				tx.Exec(dbCtx,
					`INSERT INTO aliases (org_id, domain_id, address, name)
					 VALUES ($1, $2, $3, $4)
					 ON CONFLICT (org_id, address) DO UPDATE SET deleted_at = NULL WHERE aliases.deleted_at IS NOT NULL`,
					orgID, dID, cleanAddr, addrParts[0],
				)
			}
		}
	}
	// Re-check deliveredViaAlias now that alias may have been auto-created
	if deliveredViaAlias == nil {
		var autoAliasAddr string
		if tx.QueryRow(dbCtx,
			`SELECT address FROM aliases WHERE org_id=$1 AND address=$2 AND deleted_at IS NULL`,
			orgID, strings.ToLower(strings.TrimSpace(recipientAddress)),
		).Scan(&autoAliasAddr) == nil {
			deliveredViaAlias = &autoAliasAddr
		}
	}

	// Stamp alias labels for visibility filtering
	if deliveredViaAlias != nil {
		addLabelQ(dbCtx, tx, threadID, orgID, "alias:"+*deliveredViaAlias)
	}
	// Batch-check all to/cc/bcc addresses against aliases and stamp each match
	// (BCC included for alias visibility, but NOT added to participant_emails for privacy)
	var cleanAddrs []string
	for _, addr := range allRecipients {
		cleanAddr := strings.ToLower(strings.TrimSpace(addr))
		if deliveredViaAlias != nil && cleanAddr == *deliveredViaAlias {
			continue // already stamped above
		}
		cleanAddrs = append(cleanAddrs, cleanAddr)
	}
	if len(cleanAddrs) > 0 {
		aliasRows, aliasErr := tx.Query(dbCtx,
			`SELECT address FROM aliases WHERE org_id=$1 AND address = ANY($2) AND deleted_at IS NULL`,
			orgID, cleanAddrs,
		)
		if aliasErr == nil {
			defer aliasRows.Close()
			for aliasRows.Next() {
				var matchedAlias string
				if aliasRows.Scan(&matchedAlias) == nil {
					addLabelQ(dbCtx, tx, threadID, orgID, "alias:"+matchedAlias)
				}
			}
			aliasRows.Close()
		}
	}

	// COMMIT
	if err := tx.Commit(dbCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Upsert discovered addresses for From/To/CC
	for _, addr := range append(emailData.To, emailData.CC...) {
		cleanAddr := strings.ToLower(strings.TrimSpace(addr))
		parts := strings.Split(cleanAddr, "@")
		if len(parts) == 2 {
			// Only track addresses on our own domains
			var matchedDomainID string
			if w.store.Q().QueryRow(ctx,
				"SELECT id FROM domains WHERE org_id = $1 AND domain = $2",
				orgID, parts[1],
			).Scan(&matchedDomainID) == nil {
				if _, err := w.store.Q().Exec(ctx,
					`INSERT INTO discovered_addresses (domain_id, address, local_part, email_count)
					 VALUES ($1, $2, $3, 1)
					 ON CONFLICT (domain_id, address) DO UPDATE SET email_count = discovered_addresses.email_count + 1`,
					matchedDomainID, cleanAddr, parts[0],
				); err != nil {
					slog.Warn("email worker: upsert discovered address failed", "address", cleanAddr, "error", err)
				}
			}
		}
	}

	slog.Info("email worker: email processed", "email_id", emailID, "thread_id", threadID, "spam", isSpam, "from", emailData.From)

	// Publish event with full thread for smooth frontend cache updates
	eventPayload := map[string]interface{}{
		"email_id": emailID,
		"from":     emailData.From,
		"subject":  emailData.Subject,
	}
	if thread := w.fetchThreadForEvent(ctx, threadID, orgID); thread != nil {
		eventPayload["thread"] = thread
	}
	w.bus.Publish(ctx, event.Event{
		EventType: event.EmailReceived,
		OrgID:     orgID,
		UserID:    routedUserID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload:   eventPayload,
	})

	return nil
}

// routeEmail determines which user should receive an inbound email.
// 3-phase routing: direct user -> alias -> catch-all admin.
func (w *EmailWorker) routeEmail(ctx context.Context, orgID, domainID, address string) string {
	var userID string

	// Phase 1: Direct user match
	err := w.store.Q().QueryRow(ctx,
		"SELECT id FROM users WHERE org_id = $1 AND email = $2 AND status = 'active'",
		orgID, address,
	).Scan(&userID)
	if err == nil {
		return userID
	}

	// Phase 2: Alias match — only route to active users
	var aliasID string
	err = w.store.Q().QueryRow(ctx,
		"SELECT id FROM aliases WHERE org_id = $1 AND address = $2 AND deleted_at IS NULL",
		orgID, address,
	).Scan(&aliasID)
	if err == nil {
		err = w.store.Q().QueryRow(ctx,
			`SELECT au.user_id FROM alias_users au
			 JOIN users u ON u.id = au.user_id
			 WHERE au.alias_id = $1 AND u.status = 'active'
			 LIMIT 1`,
			aliasID,
		).Scan(&userID)
		if err == nil {
			return userID
		}
	}

	// Phase 3: Catch-all — org admin
	err = w.store.Q().QueryRow(ctx,
		"SELECT id FROM users WHERE org_id = $1 AND role = 'admin' AND status = 'active' LIMIT 1",
		orgID,
	).Scan(&userID)
	if err == nil {
		return userID
	}

	return ""
}

// downloadAndStoreAttachment downloads an attachment from a URL, validates its
// MIME type, and stores it in the attachments table. Returns the attachment ID.
func (w *EmailWorker) downloadAndStoreAttachment(ctx context.Context, orgID, userID, fileURL, filename, contentType string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fileURL, nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, 25<<20)) // 25MB limit
	if err != nil {
		return "", fmt.Errorf("read body: %w", err)
	}

	// MIME-check same blocklist as upload path
	detected := http.DetectContentType(data)
	blockedTypes := map[string]bool{
		"application/x-executable":     true,
		"application/x-msdos-program":  true,
		"application/x-msdownload":     true,
		"application/x-dosexec":        true,
		"application/vnd.microsoft.portable-executable": true,
	}
	if blockedTypes[detected] {
		return "", fmt.Errorf("blocked MIME type: %s", detected)
	}

	if contentType == "" {
		contentType = detected
	}

	id := uuid.New().String()

	_, err = w.store.Q().Exec(ctx,
		`INSERT INTO attachments (id, org_id, user_id, filename, content_type, size, data, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, now())`,
		id, orgID, userID, filename, contentType, len(data), data,
	)
	if err != nil {
		return "", fmt.Errorf("insert attachment: %w", err)
	}

	slog.Info("email worker: stored inbound attachment", "id", id, "filename", filename, "size", len(data))
	return id, nil
}

// labelsSubquery aggregates labels for a thread (same as handler.labelsSubquery).
const labelsSubquery = `(SELECT COALESCE(array_agg(tl2.label ORDER BY tl2.label), ARRAY[]::text[]) FROM thread_labels tl2 WHERE tl2.thread_id = t.id)`

// fetchThreadForEvent returns a thread summary map suitable for WS event payloads.
func (w *EmailWorker) fetchThreadForEvent(ctx context.Context, threadID, orgID string) map[string]interface{} {
	var id, dID, subject, snippet string
	var originalTo *string
	var participants json.RawMessage
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var labels []string

	err := w.store.Q().QueryRow(ctx,
		`SELECT t.id, t.domain_id, t.subject, t.participant_emails,
		 t.last_message_at, t.message_count, t.unread_count, t.snippet, t.original_to, t.created_at,
		 `+labelsSubquery+` as labels
		 FROM threads t WHERE t.id = $1 AND t.org_id = $2`,
		threadID, orgID,
	).Scan(&id, &dID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &snippet, &originalTo, &createdAt, &labels)
	if err != nil {
		return nil
	}
	if labels == nil {
		labels = []string{}
	}
	t := map[string]interface{}{
		"id":                 id,
		"domain_id":          dID,
		"subject":            subject,
		"participant_emails": participants,
		"last_message_at":    lastMessageAt,
		"message_count":      messageCount,
		"unread_count":       unreadCount,
		"labels":             labels,
		"snippet":            snippet,
		"created_at":         createdAt,
	}
	if originalTo != nil {
		t["original_to"] = *originalTo
	}
	return t
}

