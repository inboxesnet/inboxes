package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/util"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// warnIfErr logs a warning if err is non-nil. Use for non-critical lookups that have a fallback.
func warnIfErr(err error, msg string, args ...any) {
	if err != nil {
		slog.Warn(msg, append(args, "error", err)...)
	}
}

// Waiter abstracts rate limiting so the queue package can inject its RateLimiter
// without a circular import.
type Waiter interface {
	Wait(ctx context.Context) error
	WaitForOrg(ctx context.Context, orgID string) error
}

type SyncService struct {
	pool      *pgxpool.Pool
	resendSvc *ResendService
	eventBus  *event.Bus
	limiter   Waiter
}

// querier abstracts *pgxpool.Pool and pgx.Tx so sync helpers work in both contexts.
type querier interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
	Exec(ctx context.Context, sql string, args ...interface{}) (pgconn.CommandTag, error)
}

func NewSyncService(pool *pgxpool.Pool, resendSvc *ResendService, eventBus *event.Bus, limiter Waiter) *SyncService {
	return &SyncService{pool: pool, resendSvc: resendSvc, eventBus: eventBus, limiter: limiter}
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
	MessageID string   `json:"message_id"`
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

// SyncJobConfig allows resuming a sync from saved cursor positions.
type SyncJobConfig struct {
	JobID           string // sync_jobs row ID — when set, progress is persisted to Postgres
	SentCursor      string // resume sent pagination from this cursor
	ReceivedCursor  string // resume received pagination from this cursor
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
	return s.syncEmailsInternal(ctx, orgID, adminUserID, domains, nil, SyncJobConfig{})
}

// SyncEmailsWithProgress imports emails and sends progress updates to the channel.
func (s *SyncService) SyncEmailsWithProgress(ctx context.Context, orgID, adminUserID string, domains map[string]string, progress chan<- SyncProgress) (*SyncResult, error) {
	return s.syncEmailsInternal(ctx, orgID, adminUserID, domains, progress, SyncJobConfig{})
}

// SyncEmailsWithJob imports emails, persists progress to the sync_jobs table,
// and supports resuming from saved cursor positions.
func (s *SyncService) SyncEmailsWithJob(ctx context.Context, orgID, adminUserID string, domains map[string]string, cfg SyncJobConfig, progress chan<- SyncProgress) (*SyncResult, error) {
	return s.syncEmailsInternal(ctx, orgID, adminUserID, domains, progress, cfg)
}

func (s *SyncService) rateWait(ctx context.Context, orgID string) {
	if s.limiter != nil {
		s.limiter.WaitForOrg(ctx, orgID)
	}
}

func (s *SyncService) syncEmailsInternal(ctx context.Context, orgID, adminUserID string, domains map[string]string, progress chan<- SyncProgress, cfg SyncJobConfig) (*SyncResult, error) {
	result := &SyncResult{}

	emit := func(p SyncProgress) {
		if progress != nil {
			select {
			case progress <- p:
			case <-ctx.Done():
			}
		}
	}

	// updateJob persists progress to the sync_jobs row when a JobID is set.
	updateJob := func(phase string, imported, total int) {
		if cfg.JobID == "" {
			return
		}
		if _, err := s.pool.Exec(ctx,
			`UPDATE sync_jobs SET phase=$1, imported=$2, total=$3,
			 sent_count=$4, received_count=$5, sent_cursor=$6, received_cursor=$7,
			 heartbeat_at=now(), updated_at=now() WHERE id=$8`,
			phase, imported, total,
			result.SentCount, result.ReceivedCount, cfg.SentCursor, cfg.ReceivedCursor,
			cfg.JobID,
		); err != nil {
			slog.Error("sync: failed to update job progress", "error", err)
		}
	}

	// ── Phase 1: Scan — fetch all sent + received to know the total ──

	emit(SyncProgress{Phase: "scanning", Message: "Scanning sent emails..."})
	updateJob("scanning", 0, 0)

	var allEmails []resendEmail
	cursor := cfg.SentCursor
	for {
		path := "/emails?limit=100"
		if cursor != "" {
			path += "&after=" + cursor
		}
		s.rateWait(ctx, orgID)
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
		cfg.SentCursor = cursor
	}

	emit(SyncProgress{Phase: "scanning", Message: fmt.Sprintf("Found %d sent. Scanning received emails...", len(allEmails))})

	var allReceived []resendReceivedEmail
	cursor = cfg.ReceivedCursor
	for {
		path := "/emails/receiving?limit=100"
		if cursor != "" {
			path += "&after=" + cursor
		}
		s.rateWait(ctx, orgID)
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
		cfg.ReceivedCursor = cursor
	}

	// ── Phase 1.5: Pre-create aliases from scan data ──
	// This lets the user configure aliases while import continues in background.

	emit(SyncProgress{Phase: "aliases", Message: "Creating aliases from scanned emails..."})
	updateJob("aliases", 0, 0)

	// Collect unique addresses on own domains with display names.
	// Key: address, Value: {domainID, displayName}
	type addrInfo struct {
		domainID    string
		displayName string
	}
	ownAddrs := make(map[string]addrInfo)

	// Helper to register an own-domain address with an optional display name.
	registerOwn := func(raw string) {
		addr := strings.ToLower(strings.TrimSpace(ExtractEmail(raw)))
		did := matchDomain(raw, domains)
		if did == "" {
			return
		}
		if _, ok := ownAddrs[addr]; !ok {
			ownAddrs[addr] = addrInfo{domainID: did, displayName: extractDisplayName(raw)}
		} else if existing := ownAddrs[addr]; existing.displayName == strings.Split(addr, "@")[0] {
			// Upgrade from default local-part to a real display name if available
			name := extractDisplayName(raw)
			if name != strings.Split(addr, "@")[0] {
				ownAddrs[addr] = addrInfo{domainID: did, displayName: name}
			}
		}
	}

	// Scan sent emails — From is own address; To/CC/BCC may also be own addresses
	for _, email := range allEmails {
		registerOwn(email.From)
		for _, a := range email.To {
			registerOwn(a)
		}
		for _, a := range email.CC {
			registerOwn(a)
		}
		for _, a := range email.BCC {
			registerOwn(a)
		}
	}
	// Scan received emails — To/CC/BCC are own addresses
	for _, email := range allReceived {
		for _, a := range email.To {
			registerOwn(a)
		}
		for _, a := range email.CC {
			registerOwn(a)
		}
		for _, a := range email.BCC {
			registerOwn(a)
		}
	}

	// Upsert discovered_addresses + aliases for each own-domain address
	for addr, info := range ownAddrs {
		localPart := strings.Split(addr, "@")[0]

		// Upsert discovered_addresses
		if _, err := s.pool.Exec(ctx,
			`INSERT INTO discovered_addresses (domain_id, address, local_part, email_count)
			 VALUES ($1, $2, $3, 0)
			 ON CONFLICT (domain_id, address) DO NOTHING`,
			info.domainID, addr, localPart,
		); err != nil {
			slog.Error("sync: phase1.5 upsert discovered_addresses", "error", err, "address", addr)
		}

		// Create alias — use display name; update name only if still the default local-part
		var aliasID string
		err := s.pool.QueryRow(ctx,
			`INSERT INTO aliases (org_id, domain_id, address, name)
			 VALUES ($1, $2, $3, $4)
			 ON CONFLICT (org_id, address) DO UPDATE SET name = EXCLUDED.name
			 WHERE aliases.name = split_part(aliases.address, '@', 1)
			 RETURNING id`,
			orgID, info.domainID, addr, info.displayName,
		).Scan(&aliasID)
		if err != nil {
			// Alias may already exist with a non-default name — fetch its ID
			warnIfErr(s.pool.QueryRow(ctx,
				`SELECT id FROM aliases WHERE org_id = $1 AND address = $2`,
				orgID, addr,
			).Scan(&aliasID), "sync: alias lookup failed", "address", addr)
		}
		if aliasID != "" {
			// Assign to admin
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO alias_users (alias_id, user_id, can_send_as, is_default)
				 VALUES ($1, $2, true, false)
				 ON CONFLICT DO NOTHING`,
				aliasID, adminUserID,
			); err != nil {
				slog.Error("sync: phase1.5 auto-assign alias user", "error", err)
			}
			// Link discovered_addresses to alias
			if _, err := s.pool.Exec(ctx,
				`UPDATE discovered_addresses SET type = 'group', alias_id = $1
				 WHERE domain_id = $2 AND address = $3`,
				aliasID, info.domainID, addr,
			); err != nil {
				slog.Error("sync: phase1.5 update discovered_addresses", "error", err)
			}
		}
	}

	emit(SyncProgress{Phase: "aliases_ready", Message: "Addresses discovered — starting import..."})
	updateJob("aliases_ready", 0, 0)

	// ── Phase 2: Import — one unified progress bar ──

	total := len(allEmails) + len(allReceived)
	imported := 0
	discoveredAddresses := make(map[string]map[string]int)

	emit(SyncProgress{Phase: "importing", Imported: 0, Total: total,
		Message: fmt.Sprintf("Importing %d emails (%d sent + %d received)", total, len(allEmails), len(allReceived))})
	updateJob("importing", 0, total)

	// Import received emails first so the inbox populates live immediately
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
		s.rateWait(ctx, orgID)
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

		fromClean := ExtractEmail(email.From)
		trackOwnAddresses(discoveredAddresses, email.To, domains)
		trackOwnAddresses(discoveredAddresses, email.CC, domains)

		toJSON, marshalErr := json.Marshal(email.To)
		if marshalErr != nil {
			slog.Error("sync: marshal to addresses", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		ccJSON, marshalErr := json.Marshal(email.CC)
		if marshalErr != nil {
			slog.Error("sync: marshal cc addresses", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		bccJSON, marshalErr := json.Marshal(email.BCC)
		if marshalErr != nil {
			slog.Error("sync: marshal bcc addresses", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		replyToJSON, marshalErr := json.Marshal(replyTo)
		if marshalErr != nil {
			slog.Error("sync: marshal reply-to", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		headersJSON, marshalErr := json.Marshal(headers)
		if marshalErr != nil {
			slog.Error("sync: marshal headers", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		attachmentsJSON, marshalErr := json.Marshal(attachments)
		if marshalErr != nil {
			slog.Error("sync: marshal attachments", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		// Resend returns lowercase header keys. Read them through a
		// lowercase copy so that a change of case cannot hide them.
		lowerHeaders := make(map[string]string, len(headers))
		for k, v := range headers {
			lowerHeaders[strings.ToLower(k)] = v
		}
		inReplyTo := util.NormalizeMsgID(lowerHeaders["in-reply-to"])
		referencesHeader := util.ParseMsgIDs(lowerHeaders["references"])
		messageID := util.NormalizeMsgID(email.MessageID)
		if messageID == "" {
			messageID = util.NormalizeMsgID(lowerHeaders["message-id"])
		}
		refsJSON, marshalErr := json.Marshal(referencesHeader)
		if marshalErr != nil {
			slog.Error("sync: marshal references", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		snippet := util.TruncateRunes(text, 200)

		createdAt := parseTime(email.CreatedAt)

		// Per-email transaction: findOrCreateThread + labels + INSERT email + updateThreadStats
		var recvThreadID string
		txErr := func() error {
			dbCtx, dbCancel := util.DBCtx(ctx)
			defer dbCancel()

			tx, err := s.pool.Begin(dbCtx)
			if err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
			defer tx.Rollback(dbCtx)

			if err := LockThreading(dbCtx, tx, orgID); err != nil {
				return fmt.Errorf("threading lock: %w", err)
			}
			participants := util.NormalizeAddresses([]string{email.From}, email.To, email.CC)
			threadID, err := s.findOrCreateThread(dbCtx, tx, ThreadLookup{
				OrgID:        orgID,
				DomainID:     domainID,
				MessageID:    messageID,
				InReplyTo:    inReplyTo,
				References:   referencesHeader,
				Subject:      email.Subject,
				Counterparty: email.From,
			}, adminUserID, participants, email.To, snippet, "inbound", email.From)
			if err != nil {
				return fmt.Errorf("find/create thread: %w", err)
			}
			recvThreadID = threadID
			if err := MergeParticipants(dbCtx, tx, threadID, participants); err != nil {
				slog.Error("sync: failed to merge participants", "error", err)
			}

			// Add inbox label for received emails (idempotent)
			if _, err := tx.Exec(dbCtx,
				`INSERT INTO thread_labels (thread_id, org_id, label) VALUES ($1, $2, 'inbox') ON CONFLICT DO NOTHING`,
				threadID, orgID); err != nil {
				slog.Error("sync: failed to insert inbox label", "error", err)
			}
			if _, err := tx.Exec(dbCtx,
				`UPDATE threads SET unread_count = unread_count + 1, updated_at = now() WHERE id = $1`,
				threadID); err != nil {
				slog.Error("sync: failed to update unread count", "error", err)
			}

			_, err = tx.Exec(dbCtx,
				`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, message_id,
				 direction, from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
				 subject, body_html, body_plain, status, headers, in_reply_to, references_header,
				 attachments, created_at)
				 VALUES ($1, $2, $3, $4, $5, $6, 'inbound', $7, $8, $9, $10, $11, $12, $13, $14, 'received',
				 $15, $16, $17, $18, $19)`,
				threadID, adminUserID, orgID, domainID, email.ID, nullIfEmpty(messageID),
				fromClean, toJSON, ccJSON, bccJSON, replyToJSON,
				email.Subject, html, text, headersJSON, inReplyTo, refsJSON,
				attachmentsJSON, createdAt,
			)
			if err != nil {
				return fmt.Errorf("insert email: %w", err)
			}

			s.updateThreadStats(dbCtx, tx, threadID, createdAt, snippet, email.From)

			// Stamp alias labels for all to/cc addresses matching an alias (inbound = delivered_via_alias)
			for _, addr := range append(email.To, email.CC...) {
				cleanAddr := strings.ToLower(strings.TrimSpace(ExtractEmail(addr)))
				var matchedAlias string
				if tx.QueryRow(dbCtx,
					`SELECT address FROM aliases WHERE org_id=$1 AND address=$2`,
					orgID, cleanAddr,
				).Scan(&matchedAlias) == nil {
					if _, err := tx.Exec(dbCtx,
						`INSERT INTO thread_labels (thread_id, org_id, label) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
						threadID, orgID, "alias:"+matchedAlias); err != nil {
						slog.Error("sync: failed to insert alias label", "error", err)
					}
				}
			}

			return tx.Commit(dbCtx)
		}()
		if txErr != nil {
			slog.Error("sync: received email tx failed", "error", txErr, "email_id", email.ID)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		// Fire targeted event so frontend updates inbox + thread detail in real time
		eventPayload := map[string]interface{}{
			"from":    email.From,
			"subject": email.Subject,
		}
		if thread := s.fetchThreadForEvent(ctx, recvThreadID, orgID); thread != nil {
			eventPayload["thread"] = thread
		}
		if _, err := s.eventBus.Publish(ctx, event.Event{
			EventType: event.EmailReceived,
			OrgID:     orgID,
			DomainID:  domainID,
			ThreadID:  recvThreadID,
			Payload:   eventPayload,
		}); err != nil {
			slog.Error("sync: event publish failed", "error", err, "event", event.EmailReceived, "thread_id", recvThreadID)
		}

		result.ReceivedCount++
		imported++
		emit(SyncProgress{Phase: "importing", Imported: imported, Total: total,
			Message: fmt.Sprintf("Importing %d of %d emails", imported, total)})
		if imported%20 == 0 {
			updateJob("importing", imported, total)
		}
	}

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

		toJSON, marshalErr := json.Marshal(email.To)
		if marshalErr != nil {
			slog.Error("sync: marshal sent to addresses", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		ccJSON, marshalErr := json.Marshal(email.CC)
		if marshalErr != nil {
			slog.Error("sync: marshal sent cc addresses", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		bccJSON, marshalErr := json.Marshal(email.BCC)
		if marshalErr != nil {
			slog.Error("sync: marshal sent bcc addresses", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}
		replyToJSON, marshalErr := json.Marshal(email.ReplyTo)
		if marshalErr != nil {
			slog.Error("sync: marshal sent reply-to", "error", marshalErr)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		var existingID string
		err := s.pool.QueryRow(ctx,
			"SELECT id FROM emails WHERE resend_email_id = $1", email.ID,
		).Scan(&existingID)
		if err == nil {
			// Backfill: old outbound rows have no Message-ID. Replies to
			// them cannot match by header until the id is stored.
			if msgID := util.NormalizeMsgID(email.MessageID); msgID != "" {
				if _, err := s.pool.Exec(ctx,
					`UPDATE emails SET message_id = $1, updated_at = now()
					 WHERE id = $2 AND org_id = $3 AND (message_id IS NULL OR message_id = '')`,
					msgID, existingID, orgID,
				); err != nil {
					slog.Warn("sync: backfill message_id failed", "email_id", email.ID, "error", err)
				}
			}
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		// Fetch full email detail (list endpoint doesn't include body)
		s.rateWait(ctx, orgID)
		fullData, fullErr := s.resendSvc.Fetch(ctx, orgID, "GET", "/emails/"+email.ID, nil)
		if fullErr == nil {
			var full resendEmail
			if json.Unmarshal(fullData, &full) == nil {
				email.HTML = full.HTML
				email.Text = full.Text
				if email.MessageID == "" {
					email.MessageID = full.MessageID
				}
				// Enrich alias name if the full From field has a better display name
				displayName := extractDisplayName(full.From)
				fromAddr := ExtractEmail(full.From)
				if displayName != strings.Split(fromAddr, "@")[0] {
					if _, err := s.pool.Exec(ctx,
						`UPDATE aliases SET name = $1 WHERE org_id = $2 AND address = $3
						 AND name = split_part(address, '@', 1)`,
						displayName, orgID, fromAddr); err != nil {
						slog.Warn("sync: enrich alias name failed", "address", fromAddr, "error", err)
					}
				}
			}
		}

		// Fetch attachments for this sent email
		var attachmentsJSON []byte
		s.rateWait(ctx, orgID)
		attData, attErr := s.resendSvc.Fetch(ctx, orgID, "GET", "/emails/"+email.ID+"/attachments", nil)
		if attErr == nil {
			var attList resendAttachmentList
			if json.Unmarshal(attData, &attList) == nil && len(attList.Data) > 0 {
				if marshaled, mErr := json.Marshal(attList.Data); mErr == nil {
					attachmentsJSON = marshaled
				} else {
					slog.Warn("sync: marshal sent attachments failed", "error", mErr)
				}
			}
		}
		if attachmentsJSON == nil {
			attachmentsJSON = []byte("[]")
		}

		snippet := util.TruncateRunes(email.Text, 200)
		status := mapResendStatus(email.LastEvent)
		createdAt := parseTime(email.CreatedAt)

		// Per-email transaction: findOrCreateThread + INSERT email + updateThreadStats + labels
		var sentThreadID string
		txErr := func() error {
			dbCtx, dbCancel := util.DBCtx(ctx)
			defer dbCancel()

			tx, err := s.pool.Begin(dbCtx)
			if err != nil {
				return fmt.Errorf("begin tx: %w", err)
			}
			defer tx.Rollback(dbCtx)

			if err := LockThreading(dbCtx, tx, orgID); err != nil {
				return fmt.Errorf("threading lock: %w", err)
			}
			messageID := util.NormalizeMsgID(email.MessageID)
			counterparty := ""
			if len(email.To) > 0 {
				counterparty = email.To[0]
			}
			participants := util.NormalizeAddresses([]string{email.From}, email.To, email.CC)
			threadID, err := s.findOrCreateThread(dbCtx, tx, ThreadLookup{
				OrgID:        orgID,
				DomainID:     domainID,
				MessageID:    messageID,
				Subject:      email.Subject,
				Counterparty: counterparty,
			}, adminUserID, participants, email.To, snippet, "outbound", email.From)
			if err != nil {
				return fmt.Errorf("find/create thread: %w", err)
			}
			sentThreadID = threadID
			if err := MergeParticipants(dbCtx, tx, threadID, participants); err != nil {
				slog.Error("sync: failed to merge participants", "error", err)
			}

			_, err = tx.Exec(dbCtx,
				`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, message_id, direction,
				 from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
				 subject, body_html, body_plain, status, last_event, attachments, created_at)
				 VALUES ($1, $2, $3, $4, $5, $18, 'outbound', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)`,
				threadID, adminUserID, orgID, domainID, email.ID,
				ExtractEmail(email.From), toJSON, ccJSON, bccJSON, replyToJSON,
				email.Subject, email.HTML, email.Text, status, email.LastEvent, attachmentsJSON, createdAt,
				nullIfEmpty(messageID),
			)
			if err != nil {
				return fmt.Errorf("insert email: %w", err)
			}

			s.updateThreadStats(dbCtx, tx, threadID, createdAt, snippet, email.From)

			// Stamp alias label if from_address matches an alias (outbound = sent_as_alias)
			fromClean := strings.ToLower(strings.TrimSpace(ExtractEmail(email.From)))
			var aliasAddr string
			if tx.QueryRow(dbCtx,
				`SELECT address FROM aliases WHERE org_id=$1 AND address=$2`,
				orgID, fromClean,
			).Scan(&aliasAddr) == nil {
				if _, err := tx.Exec(dbCtx,
					`INSERT INTO thread_labels (thread_id, org_id, label) VALUES ($1, $2, $3) ON CONFLICT DO NOTHING`,
					threadID, orgID, "alias:"+aliasAddr); err != nil {
					slog.Error("sync: failed to insert alias label", "error", err)
				}
			}

			return tx.Commit(dbCtx)
		}()
		if txErr != nil {
			slog.Error("sync: sent email tx failed", "error", txErr, "email_id", email.ID)
			imported++
			emit(SyncProgress{Phase: "importing", Imported: imported, Total: total})
			continue
		}

		// Fire targeted event so frontend updates sent folder + thread detail in real time
		sentPayload := map[string]interface{}{}
		if thread := s.fetchThreadForEvent(ctx, sentThreadID, orgID); thread != nil {
			sentPayload["thread"] = thread
		}
		if _, err := s.eventBus.Publish(ctx, event.Event{
			EventType: event.EmailSent,
			OrgID:     orgID,
			DomainID:  domainID,
			ThreadID:  sentThreadID,
			Payload:   sentPayload,
		}); err != nil {
			slog.Error("sync: event publish failed", "error", err, "event", event.EmailSent, "thread_id", sentThreadID)
		}

		result.SentCount++
		imported++
		emit(SyncProgress{Phase: "importing", Imported: imported, Total: total,
			Message: fmt.Sprintf("Importing %d of %d emails", imported, total)})
		if imported%20 == 0 {
			updateJob("importing", imported, total)
		}
	}

	// Phase: addresses
	emit(SyncProgress{Phase: "addresses", Message: "Discovering addresses..."})
	updateJob("addresses", imported, total)

	for domainID, addresses := range discoveredAddresses {
		for addr, count := range addresses {
			localPart := strings.Split(addr, "@")[0]
			// Update email_count on already-created discovered_addresses rows
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

			// Backfill alias labels on threads (catches any threads where per-email check missed)
			if _, err := s.pool.Exec(ctx,
				`INSERT INTO thread_labels (thread_id, org_id, label)
				 SELECT DISTINCT e.thread_id, e.org_id, $2
				 FROM emails e WHERE e.org_id = $1::uuid
				 AND (e.from_address = $3 OR e.to_addresses @> $4::jsonb OR e.cc_addresses @> $4::jsonb)
				 ON CONFLICT DO NOTHING`,
				orgID, "alias:"+addr, addr, fmt.Sprintf(`["%s"]`, addr),
			); err != nil {
				slog.Error("sync: failed to backfill alias labels", "error", err)
			}
		}
	}

	// Count threads
	warnIfErr(s.pool.QueryRow(ctx,
		"SELECT COUNT(DISTINCT id) FROM threads WHERE org_id = $1", orgID,
	).Scan(&result.ThreadCount), "sync: thread count query failed", "org_id", orgID)

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

	// Persist final counts to the job row
	if cfg.JobID != "" {
		if _, err := s.pool.Exec(ctx,
			`UPDATE sync_jobs SET phase='done', imported=$1, total=$2,
			 sent_count=$3, received_count=$4, thread_count=$5, address_count=$6,
			 heartbeat_at=now(), updated_at=now() WHERE id=$7`,
			imported, total, result.SentCount, result.ReceivedCount,
			result.ThreadCount, result.AddressCount, cfg.JobID,
		); err != nil {
			slog.Error("sync: failed to update final job status", "error", err)
		}
	}

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

// ThreadLookup holds the data that selects the thread of one email.
// The ids must be in normal form (util.NormalizeMsgID).
type ThreadLookup struct {
	OrgID      string
	DomainID   string
	MessageID  string   // id of the email itself, "" if unknown
	InReplyTo  string   // "" if the email has no In-Reply-To
	References []string // empty if the email has no References
	Subject    string   // raw subject
	// Counterparty is the other party: the sender of inbound mail, the
	// first recipient of outbound mail.
	Counterparty string
}

// LockThreading serialises thread find-or-create for one org until the
// transaction ends. Without it, two workers can each create a thread for
// two emails of one conversation.
func LockThreading(ctx context.Context, q querier, orgID string) error {
	_, err := q.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended('threading:' || $1::text, 0))`, orgID)
	return err
}

// FindThread returns the live thread that the email belongs to. All steps
// ignore deleted threads, so a reply can never join a thread that no list
// shows. The webhook path, the poller path and the sync import all use it.
func FindThread(ctx context.Context, q querier, in ThreadLookup) (string, bool) {
	var threadID string

	// Step 1: In-Reply-To.
	if in.InReplyTo != "" {
		if err := q.QueryRow(ctx,
			`SELECT e.thread_id FROM emails e
			 JOIN threads t ON t.id = e.thread_id
			 WHERE e.message_id = $1 AND e.org_id = $2 AND t.deleted_at IS NULL
			 ORDER BY e.created_at DESC LIMIT 1`,
			in.InReplyTo, in.OrgID,
		).Scan(&threadID); err == nil {
			return threadID, true
		}
	}

	// Step 2: References.
	if len(in.References) > 0 {
		if err := q.QueryRow(ctx,
			`SELECT e.thread_id FROM emails e
			 JOIN threads t ON t.id = e.thread_id
			 WHERE e.message_id = ANY($1::text[]) AND e.org_id = $2 AND t.deleted_at IS NULL
			 ORDER BY e.created_at DESC LIMIT 1`,
			in.References, in.OrgID,
		).Scan(&threadID); err == nil {
			return threadID, true
		}
	}

	// Step 3: a reply to this email is already stored (the reply arrived
	// before its parent). Join the thread of the reply.
	if in.MessageID != "" {
		if err := q.QueryRow(ctx,
			`SELECT e.thread_id FROM emails e
			 JOIN threads t ON t.id = e.thread_id
			 WHERE e.in_reply_to = $1 AND e.in_reply_to <> '' AND e.org_id = $2 AND t.deleted_at IS NULL
			 ORDER BY e.created_at ASC LIMIT 1`,
			in.MessageID, in.OrgID,
		).Scan(&threadID); err == nil {
			return threadID, true
		}
	}

	// Step 4: subject + counterparty, in the same domain, in the last 90
	// days. Only a reply can use this step. A new email with a common
	// subject ("Invoice") must start its own thread.
	cleanSubject := util.CleanSubjectLine(in.Subject)
	counterparty := util.NormalizeAddress(in.Counterparty)
	isReply := in.InReplyTo != "" || len(in.References) > 0 || util.HasReplyPrefix(in.Subject)
	if isReply && cleanSubject != "" && counterparty != "" {
		counterpartyJSON, err := json.Marshal([]string{counterparty})
		if err != nil {
			return "", false
		}
		if err := q.QueryRow(ctx,
			`SELECT id FROM threads WHERE org_id = $1 AND domain_id = $2 AND subject = $3
			 AND participant_emails @> $4::jsonb AND deleted_at IS NULL
			 AND last_message_at > now() - interval '90 days'
			 ORDER BY last_message_at DESC LIMIT 1`,
			in.OrgID, in.DomainID, cleanSubject, counterpartyJSON,
		).Scan(&threadID); err == nil {
			return threadID, true
		}
	}

	return "", false
}

// MergeParticipants adds bare lowercase addresses to the participants of a
// thread. Pass the output of util.NormalizeAddresses.
func MergeParticipants(ctx context.Context, q querier, threadID string, addrs []string) error {
	if len(addrs) == 0 {
		return nil
	}
	addrsJSON, err := json.Marshal(addrs)
	if err != nil {
		return err
	}
	_, err = q.Exec(ctx,
		`UPDATE threads SET participant_emails = (
		   SELECT COALESCE(jsonb_agg(DISTINCT val), '[]'::jsonb) FROM (
		     SELECT jsonb_array_elements(participant_emails) AS val
		     UNION
		     SELECT jsonb_array_elements($2::jsonb) AS val
		   ) sub
		 )
		 WHERE id = $1`, threadID, string(addrsJSON),
	)
	return err
}

func nullIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func (s *SyncService) findOrCreateThread(ctx context.Context, q querier, in ThreadLookup, userID string, participants, to []string, snippet, direction, lastSender string) (string, error) {
	if threadID, ok := FindThread(ctx, q, in); ok {
		return threadID, nil
	}

	participantsJSON, err := json.Marshal(participants)
	if err != nil {
		return "", fmt.Errorf("marshal participants: %w", err)
	}
	var originalTo *string
	if len(to) > 0 {
		addr := ExtractEmail(to[0])
		originalTo = &addr
	}
	var threadID string
	err = q.QueryRow(ctx,
		`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, original_to, snippet, last_sender, last_message_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '1970-01-01T00:00:00Z')
		 RETURNING id`,
		in.OrgID, userID, in.DomainID, util.CleanSubjectLine(in.Subject), participantsJSON, originalTo, snippet, lastSender,
	).Scan(&threadID)
	if err == nil && direction == "outbound" {
		// Add sent label only for outbound threads
		if _, err := q.Exec(ctx,
			`INSERT INTO thread_labels (thread_id, org_id, label) VALUES ($1, $2, 'sent') ON CONFLICT DO NOTHING`,
			threadID, in.OrgID); err != nil {
			slog.Error("sync: failed to insert sent label", "error", err)
		}
	}
	return threadID, err
}

func (s *SyncService) updateThreadStats(ctx context.Context, q querier, threadID string, messageAt time.Time, snippet, lastSender string) {
	if _, err := q.Exec(ctx,
		`UPDATE threads SET message_count = message_count + 1,
		 last_message_at = GREATEST(last_message_at, $1),
		 snippet = CASE WHEN $1 >= last_message_at THEN $3 ELSE snippet END,
		 last_sender = CASE WHEN $1 >= last_message_at THEN $4 ELSE last_sender END,
		 updated_at = now()
		 WHERE id = $2`,
		messageAt, threadID, snippet, lastSender,
	); err != nil {
		slog.Error("sync: failed to update thread stats", "error", err)
	}
}

// labelsSubquery aggregates labels for a thread (same as handler.labelsSubquery).
const labelsSubquery = `(SELECT COALESCE(array_agg(tl2.label ORDER BY tl2.label), ARRAY[]::text[]) FROM thread_labels tl2 WHERE tl2.thread_id = t.id)`

// fetchThreadForEvent returns a thread summary map suitable for WS event payloads.
func (s *SyncService) fetchThreadForEvent(ctx context.Context, threadID, orgID string) map[string]interface{} {
	var id, dID, subject, snippet, lastSender string
	var originalTo *string
	var trashExpiresAt, snoozedUntil *time.Time
	var participants json.RawMessage
	var lastMessageAt, createdAt time.Time
	var messageCount, unreadCount int
	var labels []string

	err := s.pool.QueryRow(ctx,
		`SELECT t.id, t.domain_id, t.subject, t.participant_emails,
		 t.last_message_at, t.message_count, t.unread_count, t.snippet, t.last_sender, t.original_to, t.created_at,
		 t.trash_expires_at, t.snoozed_until,
		 `+labelsSubquery+` as labels
		 FROM threads t WHERE t.id = $1 AND t.org_id = $2`,
		threadID, orgID,
	).Scan(&id, &dID, &subject, &participants,
		&lastMessageAt, &messageCount, &unreadCount, &snippet, &lastSender, &originalTo, &createdAt,
		&trashExpiresAt, &snoozedUntil, &labels)
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
		"last_sender":        lastSender,
		"created_at":         createdAt,
	}
	if originalTo != nil {
		t["original_to"] = *originalTo
	}
	// The frontend replaces its list row with this summary, so it must
	// carry the same optional fields as the thread list.
	if trashExpiresAt != nil {
		t["trash_expires_at"] = *trashExpiresAt
	}
	if snoozedUntil != nil {
		t["snoozed_until"] = *snoozedUntil
	}
	return t
}

// extractDisplayName pulls the name from "CX Agency <hello@cx.agency>" → "CX Agency".
// Falls back to local part if no name present.
func extractDisplayName(raw string) string {
	if idx := strings.Index(raw, "<"); idx > 0 {
		name := strings.TrimSpace(raw[:idx])
		if name != "" {
			return name
		}
	}
	return strings.Split(ExtractEmail(raw), "@")[0]
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
	return domains[strings.ToLower(parts[1])]
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


// mapResendStatus maps Resend's last_event string to our email_status enum.
func mapResendStatus(lastEvent string) string {
	switch lastEvent {
	case "delivered":
		return "delivered"
	case "bounced":
		return "bounced"
	case "complained":
		return "complained"
	case "failed":
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
