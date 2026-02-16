package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SyncService struct {
	pool      *pgxpool.Pool
	resendSvc *ResendService
	eventBus  *event.Bus
}

func NewSyncService(pool *pgxpool.Pool, resendSvc *ResendService, eventBus *event.Bus) *SyncService {
	return &SyncService{pool: pool, resendSvc: resendSvc, eventBus: eventBus}
}

type resendEmail struct {
	ID        string   `json:"id"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	CC        []string `json:"cc"`
	BCC       []string `json:"bcc"`
	ReplyTo   []string `json:"reply_to"`
	Subject   string   `json:"subject"`
	HTML      string   `json:"html"`
	Text      string   `json:"text"`
	LastEvent string   `json:"last_event"`
	CreatedAt string   `json:"created_at"`
}

type resendEmailList struct {
	Data    []resendEmail `json:"data"`
	HasMore bool          `json:"has_more"`
}

type resendAttachment struct {
	ID                 string `json:"id"`
	Filename           string `json:"filename"`
	ContentType        string `json:"content_type"`
	ContentID          string `json:"content_id"`
	ContentDisposition string `json:"content_disposition"`
	DownloadURL        string `json:"download_url"`
	ExpiresAt          string `json:"expires_at"`
	Size               int    `json:"size"`
}

type resendAttachmentList struct {
	Data []resendAttachment `json:"data"`
}

type resendReceivedEmail struct {
	ID          string             `json:"id"`
	From        string             `json:"from"`
	To          []string           `json:"to"`
	CC          []string           `json:"cc"`
	BCC         []string           `json:"bcc"`
	ReplyTo     []string           `json:"reply_to"`
	Subject     string             `json:"subject"`
	HTML        string             `json:"html"`
	Text        string             `json:"text"`
	MessageID   string             `json:"message_id"`
	Headers     map[string]string  `json:"headers"`
	Attachments []resendAttachment `json:"attachments"`
	CreatedAt   string             `json:"created_at"`
}

type resendReceivedList struct {
	Data    []resendReceivedEmail `json:"data"`
	HasMore bool                  `json:"has_more"`
}

type SyncResult struct {
	SentCount     int `json:"sent_count"`
	ReceivedCount int `json:"received_count"`
	ThreadCount   int `json:"thread_count"`
	AddressCount  int `json:"address_count"`
}

// SyncProgress is sent over the progress channel during streaming sync.
type SyncProgress struct {
	Phase    string `json:"phase"`    // "fetching", "importing", "addresses", "done"
	Imported int    `json:"imported"` // emails imported so far
	Total    int    `json:"total"`    // total emails to import (0 if unknown)
	Message  string `json:"message"`  // human-readable status

	// Populated only on the "done" event:
	SentCount     int `json:"sent_count,omitempty"`
	ReceivedCount int `json:"received_count,omitempty"`
	ThreadCount   int `json:"thread_count,omitempty"`
	AddressCount  int `json:"address_count,omitempty"`
}

// SyncEmails imports emails without progress reporting (legacy blocking endpoint).
func (s *SyncService) SyncEmails(ctx context.Context, orgID, adminUserID string, domains map[string]string) (*SyncResult, error) {
	return s.syncEmailsInternal(ctx, orgID, adminUserID, domains, nil)
}

// SyncEmailsWithProgress imports emails and sends progress updates to the channel.
// The caller should close the channel when done reading, but SyncEmailsWithProgress
// will send a final "done" event and stop writing.
func (s *SyncService) SyncEmailsWithProgress(ctx context.Context, orgID, adminUserID string, domains map[string]string, progress chan<- SyncProgress) (*SyncResult, error) {
	return s.syncEmailsInternal(ctx, orgID, adminUserID, domains, progress)
}

func (s *SyncService) syncEmailsInternal(ctx context.Context, orgID, adminUserID string, domains map[string]string, progress chan<- SyncProgress) (*SyncResult, error) {
	result := &SyncResult{}

	emit := func(p SyncProgress) {
		if progress != nil {
			select {
			case progress <- p:
			case <-ctx.Done():
			}
		}
	}

	// ── Phase 1: Scan — fetch all sent + received to know the total ──

	emit(SyncProgress{Phase: "scanning", Message: "Scanning sent emails..."})

	var allEmails []resendEmail
	cursor := ""
	for {
		path := "/emails?limit=100"
		if cursor != "" {
			path += "&after=" + cursor
		}
		data, err := s.resendSvc.Fetch(ctx, orgID, "GET", path, nil)
		if err != nil {
			return nil, fmt.Errorf("fetch emails: %w", err)
		}
		var page resendEmailList
		if err := json.Unmarshal(data, &page); err != nil {
			return nil, fmt.Errorf("parse emails: %w", err)
		}
		if len(page.Data) == 0 {
			break
		}
		allEmails = append(allEmails, page.Data...)
		emit(SyncProgress{Phase: "scanning", Message: fmt.Sprintf("Found %d sent emails...", len(allEmails))})
		if len(page.Data) < 100 {
			break
		}
		cursor = page.Data[len(page.Data)-1].ID
		time.Sleep(600 * time.Millisecond)
	}

	emit(SyncProgress{Phase: "scanning", Message: fmt.Sprintf("Found %d sent. Scanning received emails...", len(allEmails))})

	var allReceived []resendReceivedEmail
	cursor = ""
	for {
		path := "/emails/receiving?limit=100"
		if cursor != "" {
			path += "&after=" + cursor
		}
		data, err := s.resendSvc.Fetch(ctx, orgID, "GET", path, nil)
		if err != nil {
			slog.Error("sync: fetch received emails", "error", err)
			break
		}
		var page resendReceivedList
		if err := json.Unmarshal(data, &page); err != nil {
			slog.Error("sync: parse received emails", "error", err)
			break
		}
		if len(page.Data) == 0 {
			break
		}
		allReceived = append(allReceived, page.Data...)
		emit(SyncProgress{Phase: "scanning", Message: fmt.Sprintf("Found %d sent + %d received emails...", len(allEmails), len(allReceived))})
		if !page.HasMore || len(page.Data) < 100 {
			break
		}
		cursor = page.Data[len(page.Data)-1].ID
		time.Sleep(600 * time.Millisecond)
	}

	// ── Phase 2: Import — one unified progress bar ──

	total := len(allEmails) + len(allReceived)
	imported := 0
	discoveredAddresses := make(map[string]map[string]int)

	emit(SyncProgress{Phase: "importing", Imported: 0, Total: total,
		Message: fmt.Sprintf("Importing %d emails (%d sent + %d received)", total, len(allEmails), len(allReceived))})

	// Import sent emails
	for _, email := range allEmails {
		if ctx.Err() != nil {
			break
		}

		domainID := matchDomain(email.From, domains)
		if domainID == "" {
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		trackAddress(discoveredAddresses, ExtractEmail(email.From), domainID)
		trackOwnAddresses(discoveredAddresses, email.To, domains)
		trackOwnAddresses(discoveredAddresses, email.CC, domains)

		toJSON, _ := json.Marshal(email.To)
		ccJSON, _ := json.Marshal(email.CC)
		bccJSON, _ := json.Marshal(email.BCC)
		replyToJSON, _ := json.Marshal(email.ReplyTo)

		var existingID string
		err := s.pool.QueryRow(ctx,
			"SELECT id FROM emails WHERE resend_email_id = $1", email.ID,
		).Scan(&existingID)
		if err == nil {
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		// Fetch full email detail (list endpoint doesn't include body)
		fullData, fullErr := s.resendSvc.Fetch(ctx, orgID, "GET", "/emails/"+email.ID, nil)
		if fullErr == nil {
			var full resendEmail
			if json.Unmarshal(fullData, &full) == nil {
				email.HTML = full.HTML
				email.Text = full.Text
			}
		}
		time.Sleep(600 * time.Millisecond)

		// Fetch attachments for this sent email
		var attachmentsJSON []byte
		attData, attErr := s.resendSvc.Fetch(ctx, orgID, "GET", "/emails/"+email.ID+"/attachments", nil)
		if attErr == nil {
			var attList resendAttachmentList
			if json.Unmarshal(attData, &attList) == nil && len(attList.Data) > 0 {
				attachmentsJSON, _ = json.Marshal(attList.Data)
			}
		}
		if attachmentsJSON == nil {
			attachmentsJSON = []byte("[]")
		}
		time.Sleep(600 * time.Millisecond)

		snippet := email.Text
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}

		threadID, err := s.findOrCreateThread(ctx, orgID, adminUserID, domainID, email.Subject, email.From, email.To, snippet, "", "outbound")
		if err != nil {
			slog.Error("sync: create thread", "error", err, "subject", email.Subject)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		status := mapResendStatus(email.LastEvent)
		createdAt := parseTime(email.CreatedAt)
		_, err = s.pool.Exec(ctx,
			`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, direction,
			 from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
			 subject, body_html, body_plain, status, last_event, attachments, created_at)
			 VALUES ($1, $2, $3, $4, $5, 'outbound', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
			threadID, adminUserID, orgID, domainID, email.ID,
			ExtractEmail(email.From), toJSON, ccJSON, bccJSON, replyToJSON,
			email.Subject, email.HTML, email.Text, status, email.LastEvent, attachmentsJSON, createdAt,
		)
		if err != nil {
			slog.Error("sync: insert sent email", "error", err)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		s.updateThreadStats(ctx, threadID, createdAt, snippet)
		result.SentCount++
		imported++
		emit(SyncProgress{Phase: "importing", Imported: imported, Total: total,
			Message: fmt.Sprintf("Importing %d of %d emails", imported, total)})
	}

	// Import received emails
	for _, email := range allReceived {
		if ctx.Err() != nil {
			break
		}

		domainID := ""
		for _, to := range email.To {
			domainID = matchDomain(to, domains)
			if domainID != "" {
				break
			}
		}
		if domainID == "" {
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		var existingID string
		err := s.pool.QueryRow(ctx,
			"SELECT id FROM emails WHERE resend_email_id = $1", email.ID,
		).Scan(&existingID)
		if err == nil {
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		// Fetch full email body + headers + attachments (list endpoint doesn't include them)
		var html, text string
		var headers map[string]string
		var replyTo []string
		var attachments []resendAttachment
		bodyData, err := s.resendSvc.Fetch(ctx, orgID, "GET", "/emails/receiving/"+email.ID, nil)
		if err == nil {
			var full resendReceivedEmail
			if json.Unmarshal(bodyData, &full) == nil {
				html = full.HTML
				text = full.Text
				headers = full.Headers
				replyTo = full.ReplyTo
				attachments = full.Attachments
			}
		}
		time.Sleep(600 * time.Millisecond)

		fromClean := ExtractEmail(email.From)
		trackOwnAddresses(discoveredAddresses, email.To, domains)
		trackOwnAddresses(discoveredAddresses, email.CC, domains)

		toJSON, _ := json.Marshal(email.To)
		ccJSON, _ := json.Marshal(email.CC)
		bccJSON, _ := json.Marshal(email.BCC)
		replyToJSON, _ := json.Marshal(replyTo)
		headersJSON, _ := json.Marshal(headers)
		attachmentsJSON, _ := json.Marshal(attachments)

		var inReplyTo string
		var referencesHeader []string
		if headers != nil {
			inReplyTo = headers["In-Reply-To"]
			if refs, ok := headers["References"]; ok {
				referencesHeader = strings.Fields(refs)
			}
		}
		refsJSON, _ := json.Marshal(referencesHeader)

		snippet := text
		if len(snippet) > 200 {
			snippet = snippet[:200]
		}

		threadID, err := s.findOrCreateThread(ctx, orgID, adminUserID, domainID, email.Subject, email.From, email.To, snippet, inReplyTo, "inbound")
		if err != nil {
			slog.Error("sync: create thread for received", "error", err, "subject", email.Subject)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		s.pool.Exec(ctx,
			`UPDATE threads SET folder = 'inbox', unread_count = unread_count + 1, updated_at = now()
			 WHERE id = $1 AND folder = 'sent'`, threadID,
		)

		createdAt := parseTime(email.CreatedAt)
		_, err = s.pool.Exec(ctx,
			`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, message_id,
			 direction, from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
			 subject, body_html, body_plain, status, headers, in_reply_to, references_header,
			 attachments, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, 'inbound', $7, $8, $9, $10, $11, $12, $13, $14, 'received',
			 $15, $16, $17, $18, $19)`,
			threadID, adminUserID, orgID, domainID, email.ID, email.MessageID,
			fromClean, toJSON, ccJSON, bccJSON, replyToJSON,
			email.Subject, html, text, headersJSON, inReplyTo, refsJSON,
			attachmentsJSON, createdAt,
		)
		if err != nil {
			slog.Error("sync: insert received email", "error", err)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		s.updateThreadStats(ctx, threadID, createdAt, snippet)
		result.ReceivedCount++
		imported++
		emit(SyncProgress{Phase: "importing", Imported: imported, Total: total,
			Message: fmt.Sprintf("Importing %d of %d emails", imported, total)})
	}

	// Phase: addresses
	emit(SyncProgress{Phase: "addresses", Message: "Discovering addresses..."})

	for domainID, addresses := range discoveredAddresses {
		for addr, count := range addresses {
			localPart := strings.Split(addr, "@")[0]
			_, err := s.pool.Exec(ctx,
				`INSERT INTO discovered_addresses (domain_id, address, local_part, email_count)
				 VALUES ($1, $2, $3, $4)
				 ON CONFLICT (domain_id, address) DO UPDATE SET email_count = discovered_addresses.email_count + $4`,
				domainID, addr, localPart, count,
			)
			if err != nil {
				slog.Error("sync: upsert address", "error", err, "address", addr)
			}
			result.AddressCount++
		}
	}

	// Count threads
	s.pool.QueryRow(ctx,
		"SELECT COUNT(DISTINCT id) FROM threads WHERE org_id = $1", orgID,
	).Scan(&result.ThreadCount)

	// Phase: done
	emit(SyncProgress{
		Phase:         "done",
		Imported:      imported,
		Total:         total,
		Message:       fmt.Sprintf("Imported %d sent + %d received into %d threads, discovered %d addresses", result.SentCount, result.ReceivedCount, result.ThreadCount, result.AddressCount),
		SentCount:     result.SentCount,
		ReceivedCount: result.ReceivedCount,
		ThreadCount:   result.ThreadCount,
		AddressCount:  result.AddressCount,
	})

	// Notify connected clients so the inbox updates without a manual refresh
	if s.eventBus != nil && (result.SentCount > 0 || result.ReceivedCount > 0) {
		s.eventBus.Publish(ctx, event.Event{
			EventType: event.SyncCompleted,
			OrgID:     orgID,
			UserID:    adminUserID,
			Payload: map[string]interface{}{
				"sent_count":     result.SentCount,
				"received_count": result.ReceivedCount,
				"thread_count":   result.ThreadCount,
			},
		})
	}

	return result, nil
}

func (s *SyncService) findOrCreateThread(ctx context.Context, orgID, userID, domainID, subject, from string, to []string, snippet string, inReplyTo string, direction string) (string, error) {
	var threadID string
	cleanSubject := cleanSubjectLine(subject)

	// Step 1: Match by In-Reply-To header
	if inReplyTo != "" {
		err := s.pool.QueryRow(ctx,
			`SELECT thread_id FROM emails WHERE message_id = $1 AND org_id = $2 LIMIT 1`,
			inReplyTo, orgID,
		).Scan(&threadID)
		if err == nil {
			return threadID, nil
		}
	}

	// Step 2: Match by subject + counterparty (the other party in the conversation)
	// For outbound: the counterparty is the recipient (TO address)
	// For inbound: the counterparty is the sender (FROM address)
	var matchAddr string
	if direction == "outbound" && len(to) > 0 {
		matchAddr = ExtractEmail(to[0])
	} else {
		matchAddr = ExtractEmail(from)
	}
	counterpartyJSON, _ := json.Marshal([]string{matchAddr})
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM threads WHERE org_id = $1 AND domain_id = $2 AND subject = $3
		 AND participant_emails @> $4::jsonb
		 ORDER BY last_message_at DESC LIMIT 1`,
		orgID, domainID, cleanSubject, counterpartyJSON,
	).Scan(&threadID)
	if err == nil {
		return threadID, nil
	}

	// Step 3: Create new thread
	participants, _ := json.Marshal(append([]string{from}, to...))
	var originalTo *string
	if len(to) > 0 {
		addr := ExtractEmail(to[0])
		originalTo = &addr
	}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, folder, original_to, snippet, last_message_at)
		 VALUES ($1, $2, $3, $4, $5, 'sent', $6, $7, '1970-01-01T00:00:00Z')
		 RETURNING id`,
		orgID, userID, domainID, cleanSubject, participants, originalTo, snippet,
	).Scan(&threadID)
	return threadID, err
}

func (s *SyncService) updateThreadStats(ctx context.Context, threadID string, messageAt time.Time, snippet string) {
	s.pool.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1,
		 last_message_at = GREATEST(last_message_at, $1), snippet = $3, updated_at = now()
		 WHERE id = $2`,
		messageAt, threadID, snippet,
	)
}

// ExtractEmail extracts a bare email address from a "Name <email>" format string.
func ExtractEmail(raw string) string {
	if idx := strings.Index(raw, "<"); idx != -1 {
		end := strings.Index(raw, ">")
		if end > idx {
			return strings.TrimSpace(raw[idx+1 : end])
		}
	}
	return strings.TrimSpace(raw)
}

func matchDomain(raw string, domains map[string]string) string {
	email := ExtractEmail(raw)
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return ""
	}
	return domains[parts[1]]
}

func trackAddress(addresses map[string]map[string]int, email, domainID string) {
	if _, ok := addresses[domainID]; !ok {
		addresses[domainID] = make(map[string]int)
	}
	addresses[domainID][strings.ToLower(email)]++
}

// trackOwnAddresses discovers addresses that belong to the user's domains
func trackOwnAddresses(discovered map[string]map[string]int, addrs []string, domains map[string]string) {
	for _, raw := range addrs {
		clean := ExtractEmail(raw)
		if did := matchDomain(raw, domains); did != "" {
			trackAddress(discovered, clean, did)
		}
	}
}

func cleanSubjectLine(subject string) string {
	s := subject
	for {
		lower := strings.ToLower(s)
		if strings.HasPrefix(lower, "re: ") {
			s = s[4:]
		} else if strings.HasPrefix(lower, "fwd: ") {
			s = s[5:]
		} else {
			break
		}
	}
	// Collapse multiple spaces into one
	parts := strings.Fields(strings.TrimSpace(s))
	return strings.Join(parts, " ")
}

// mapResendStatus maps Resend's last_event string to our email_status enum.
func mapResendStatus(lastEvent string) string {
	switch lastEvent {
	case "delivered":
		return "delivered"
	case "bounced":
		return "bounced"
	case "complained", "failed":
		return "failed"
	default:
		return "sent"
	}
}

func parseTime(s string) time.Time {
	// Resend actual format: "2026-02-13 16:52:55.929182+00" (space, not T; +00, not +00:00)
	// Resend spec claims:  "2023-04-03T22:13:42.674981+00:00" (RFC3339 with microseconds)
	// Handle both plus standard RFC3339 variants
	layouts := []string{
		"2006-01-02 15:04:05.999999+00",     // Resend actual (list endpoints)
		"2006-01-02 15:04:05.999999+00:00",  // Resend actual with full offset
		"2006-01-02 15:04:05+00",            // Resend without fractional seconds
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	slog.Warn("parseTime: failed to parse", "value", s)
	return time.Now()
}
