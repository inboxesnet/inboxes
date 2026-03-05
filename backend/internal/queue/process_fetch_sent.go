package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
)

// sentEmailData mirrors the poller payload for sent emails.
type sentEmailData struct {
	EmailID   string   `json:"email_id"`
	From      string   `json:"from"`
	To        []string `json:"to"`
	CC        []string `json:"cc"`
	BCC       []string `json:"bcc"`
	ReplyTo   []string `json:"reply_to"`
	Subject   string   `json:"subject"`
	LastEvent string   `json:"last_event"`
	CreatedAt string   `json:"created_at"`
}

// resendAttachment matches a single attachment from the Resend attachments endpoint.
type resendAttachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	Size        int    `json:"size"`
}

// resendAttachmentList wraps the Resend GET /emails/{id}/attachments response.
type resendAttachmentList struct {
	Data []resendAttachment `json:"data"`
}

func (w *EmailWorker) processFetchSent(ctx context.Context, jobID, orgID, userID string) error {
	var resendEmailID string
	var webhookDataRaw []byte
	err := w.store.Q().QueryRow(ctx,
		`SELECT resend_email_id, webhook_data FROM email_jobs WHERE id = $1`,
		jobID,
	).Scan(&resendEmailID, &webhookDataRaw)
	if err != nil {
		return fmt.Errorf("load fetch_sent job: %w", err)
	}

	var emailData sentEmailData
	if err := json.Unmarshal(webhookDataRaw, &emailData); err != nil {
		return fmt.Errorf("parse sent email data: %w", err)
	}

	// Idempotency: skip if we already have this email
	var existingID string
	if err := w.store.Q().QueryRow(ctx,
		"SELECT id FROM emails WHERE resend_email_id = $1", resendEmailID,
	).Scan(&existingID); err == nil {
		slog.Info("email worker: duplicate sent email, skipping", "resend_email_id", resendEmailID)
		return nil
	}

	// Determine domain from the From address
	fromClean := service.ExtractEmail(emailData.From)
	parts := strings.Split(fromClean, "@")
	if len(parts) != 2 {
		return fmt.Errorf("invalid from address: %s", emailData.From)
	}

	var domainID string
	if err := w.store.Q().QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND domain = $2 AND status = 'active' AND hidden = false",
		orgID, parts[1],
	).Scan(&domainID); err != nil {
		w.bus.Publish(ctx, event.Event{
			EventType: event.DomainNotFound,
			OrgID:     orgID,
			Payload:   map[string]interface{}{"domain": parts[1]},
		})
		return &nonRetryableError{fmt.Errorf("no matching domain for from=%s", emailData.From)}
	}

	// Fetch full email from Resend API (list endpoint doesn't include body)
	var bodyHTML, bodyPlain string
	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		return err
	}
	fullData, fullErr := w.resendSvc.Fetch(ctx, orgID, "GET", "/emails/"+resendEmailID, nil)
	if fullErr == nil {
		var full struct {
			From    string   `json:"from"`
			HTML    string   `json:"html"`
			Text    string   `json:"text"`
			ReplyTo []string `json:"reply_to"`
		}
		if json.Unmarshal(fullData, &full) == nil {
			bodyHTML = full.HTML
			bodyPlain = full.Text
			if len(full.ReplyTo) > 0 {
				emailData.ReplyTo = full.ReplyTo
			}
			// Enrich alias display name if the full From has a better name
			displayName := extractDisplayName(full.From)
			fromAddr := service.ExtractEmail(full.From)
			if displayName != strings.Split(fromAddr, "@")[0] {
				w.store.Q().Exec(ctx,
					`UPDATE aliases SET name = $1 WHERE org_id = $2 AND address = $3
					 AND name = split_part(address, '@', 1)`,
					displayName, orgID, fromAddr)
			}
		}
	} else {
		slog.Warn("email worker: failed to fetch full sent email", "error", fullErr, "resend_email_id", resendEmailID)
	}

	// Fetch attachments
	var attachmentsJSON []byte
	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		return err
	}
	attData, attErr := w.resendSvc.Fetch(ctx, orgID, "GET", "/emails/"+resendEmailID+"/attachments", nil)
	if attErr == nil {
		var attList resendAttachmentList
		if json.Unmarshal(attData, &attList) == nil && len(attList.Data) > 0 {
			if marshaled, mErr := json.Marshal(attList.Data); mErr == nil {
				attachmentsJSON = marshaled
			}
		}
	}
	if attachmentsJSON == nil {
		attachmentsJSON = []byte("[]")
	}

	// Marshal address fields
	toJSON, err := json.Marshal(emailData.To)
	if err != nil {
		return fmt.Errorf("marshal to: %w", err)
	}
	ccJSON, err := json.Marshal(emailData.CC)
	if err != nil {
		return fmt.Errorf("marshal cc: %w", err)
	}
	bccJSON, err := json.Marshal(emailData.BCC)
	if err != nil {
		return fmt.Errorf("marshal bcc: %w", err)
	}
	replyToJSON, err := json.Marshal(emailData.ReplyTo)
	if err != nil {
		return fmt.Errorf("marshal reply_to: %w", err)
	}

	snippet := util.TruncateRunes(bodyPlain, 200)
	status := mapSentStatus(emailData.LastEvent)
	createdAt := parseSentTime(emailData.CreatedAt)

	// Transaction: find/create thread + insert email + update stats + labels
	dbCtx, dbCancel := util.DBCtx(ctx)
	defer dbCancel()

	tx, err := w.store.Pool().Begin(dbCtx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(dbCtx)

	// Find or create thread (same 3-step matching as inbound)
	var threadID string
	found := false
	cleanSubject := util.CleanSubjectLine(emailData.Subject)

	// Step 1: Match by subject + counterparty (outbound: counterparty is first recipient)
	if cleanSubject != "" && len(emailData.To) > 0 {
		matchAddr := service.ExtractEmail(emailData.To[0])
		counterpartyJSON, marshalErr := json.Marshal([]string{matchAddr})
		if marshalErr != nil {
			return fmt.Errorf("marshal counterparty: %w", marshalErr)
		}
		err := tx.QueryRow(dbCtx,
			`SELECT id FROM threads WHERE org_id = $1 AND domain_id = $2 AND subject = $3
			 AND participant_emails @> $4::jsonb AND deleted_at IS NULL
			 AND last_message_at > now() - interval '90 days'
			 ORDER BY last_message_at DESC LIMIT 1`,
			orgID, domainID, cleanSubject, counterpartyJSON,
		).Scan(&threadID)
		if err == nil {
			found = true
		}
	}

	// Step 2: Create new thread
	if !found {
		participants, marshalErr := json.Marshal(append([]string{emailData.From}, emailData.To...))
		if marshalErr != nil {
			return fmt.Errorf("marshal participants: %w", marshalErr)
		}
		var originalTo *string
		if len(emailData.To) > 0 {
			addr := service.ExtractEmail(emailData.To[0])
			originalTo = &addr
		}
		if err := tx.QueryRow(dbCtx,
			`INSERT INTO threads (org_id, user_id, domain_id, subject, participant_emails, original_to, snippet, last_sender, last_message_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, '1970-01-01T00:00:00Z') RETURNING id`,
			orgID, userID, domainID, cleanSubject, participants, originalTo, snippet, emailData.From,
		).Scan(&threadID); err != nil {
			return fmt.Errorf("create thread: %w", err)
		}
		addLabelQ(dbCtx, tx, threadID, orgID, "sent")
	}

	// Insert email
	var emailID string
	if err := tx.QueryRow(dbCtx,
		`INSERT INTO emails (thread_id, user_id, org_id, domain_id, resend_email_id, direction,
		 from_address, to_addresses, cc_addresses, bcc_addresses, reply_to_addresses,
		 subject, body_html, body_plain, status, last_event, attachments, created_at)
		 VALUES ($1, $2, $3, $4, $5, 'outbound', $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
		 RETURNING id`,
		threadID, userID, orgID, domainID, resendEmailID,
		fromClean, toJSON, ccJSON, bccJSON, replyToJSON,
		emailData.Subject, bodyHTML, bodyPlain, status, emailData.LastEvent, attachmentsJSON, createdAt,
	).Scan(&emailID); err != nil {
		return fmt.Errorf("insert email: %w", err)
	}

	// Update thread stats
	if _, err := tx.Exec(dbCtx,
		`UPDATE threads SET message_count = message_count + 1,
		 last_message_at = GREATEST(last_message_at, $1),
		 snippet = CASE WHEN $1 >= last_message_at THEN $3 ELSE snippet END,
		 last_sender = CASE WHEN $1 >= last_message_at THEN $4 ELSE last_sender END,
		 updated_at = now()
		 WHERE id = $2`,
		createdAt, threadID, snippet, emailData.From,
	); err != nil {
		slog.Error("email worker: failed to update thread stats (sent)", "error", err)
	}

	// Ensure sent label
	addLabelQ(dbCtx, tx, threadID, orgID, "sent")

	// Auto-create alias for From address if it doesn't exist
	tx.Exec(dbCtx,
		`INSERT INTO aliases (org_id, domain_id, address, name)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (org_id, address) DO UPDATE SET deleted_at = NULL WHERE aliases.deleted_at IS NOT NULL`,
		orgID, domainID, fromClean, strings.Split(fromClean, "@")[0],
	)

	// Stamp alias label if from_address matches an alias
	var aliasAddr string
	if tx.QueryRow(dbCtx,
		`SELECT address FROM aliases WHERE org_id=$1 AND address=$2 AND deleted_at IS NULL`,
		orgID, fromClean,
	).Scan(&aliasAddr) == nil {
		addLabelQ(dbCtx, tx, threadID, orgID, "alias:"+aliasAddr)
	}

	if err := tx.Commit(dbCtx); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	// Populate sent_as_alias
	w.populateSentAsAlias(ctx, emailID, orgID)

	// Merge outbound recipients into thread participant_emails
	w.mergeParticipantEmails(ctx, emailID, threadID)

	slog.Info("email worker: sent email imported", "email_id", emailID, "resend_email_id", resendEmailID, "thread_id", threadID)

	// Publish event for real-time frontend updates
	sentPayload := map[string]interface{}{}
	if thread := w.fetchThreadForEvent(ctx, threadID, orgID); thread != nil {
		sentPayload["thread"] = thread
	}
	w.bus.Publish(ctx, event.Event{
		EventType: event.EmailSent,
		OrgID:     orgID,
		UserID:    userID,
		DomainID:  domainID,
		ThreadID:  threadID,
		Payload:   sentPayload,
	})

	return nil
}

// extractDisplayName pulls the name from "CX Agency <hello@cx.agency>" → "CX Agency".
func extractDisplayName(raw string) string {
	if idx := strings.Index(raw, "<"); idx > 0 {
		name := strings.TrimSpace(raw[:idx])
		if name != "" {
			return name
		}
	}
	return strings.Split(service.ExtractEmail(raw), "@")[0]
}

// mapSentStatus maps Resend's last_event to our email status enum.
func mapSentStatus(lastEvent string) string {
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

// parseSentTime parses Resend's timestamp formats for sent emails.
func parseSentTime(s string) time.Time {
	layouts := []string{
		"2006-01-02 15:04:05.999999+00",
		"2006-01-02 15:04:05.999999+00:00",
		"2006-01-02 15:04:05+00",
		time.RFC3339,
		time.RFC3339Nano,
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	slog.Warn("parseSentTime: failed to parse", "value", s)
	return time.Now()
}
