package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
	"github.com/inboxes/backend/internal/util"
)

type sendItem struct {
	jobID      string
	emailID    string
	threadID   string
	payload    json.RawMessage
	draftID    *string
	userID     string
	retryCount int
	maxRetries int
}

func (w *EmailWorker) processSend(ctx context.Context, jobID, orgID, userID string) error {
	var emailID, threadID string
	var resendPayloadRaw []byte
	var draftID *string
	var retryCount, maxRetries int
	err := w.store.Q().QueryRow(ctx,
		`SELECT email_id, thread_id, resend_payload, draft_id, retry_count, max_retries
		 FROM email_jobs WHERE id = $1`,
		jobID,
	).Scan(&emailID, &threadID, &resendPayloadRaw, &draftID, &retryCount, &maxRetries)
	if err != nil {
		return fmt.Errorf("load send job: %w", err)
	}

	// Check domain status before sending
	var domainStatus string
	if err := w.store.Q().QueryRow(ctx,
		`SELECT d.status FROM domains d
		 JOIN emails e ON e.domain_id = d.id
		 WHERE e.id = $1`,
		emailID,
	).Scan(&domainStatus); err == nil {
		if domainStatus == "disconnected" || domainStatus == "pending" || domainStatus == "deleted" {
			// Not retryable — the domain state will not fix itself mid-job.
			return &nonRetryableError{fmt.Errorf("domain is %s, cannot send email", domainStatus)}
		}
	}

	apiKey, err := w.resendSvc.GetOrgAPIKey(ctx, orgID)
	if err != nil {
		return fmt.Errorf("get org api key: %w", err)
	}

	items := []sendItem{{
		jobID:      jobID,
		emailID:    emailID,
		threadID:   threadID,
		payload:    resendPayloadRaw,
		draftID:    draftID,
		userID:     userID,
		retryCount: retryCount,
		maxRetries: maxRetries,
	}}

	// The Resend batch endpoint does not accept attachments. An email with
	// attachments always goes out alone.
	if !payloadHasAttachments(resendPayloadRaw) {
		items = append(items, w.collectBatch(ctx, orgID, jobID)...)
	}

	if len(items) == 1 {
		return w.sendSingle(ctx, apiKey, items[0], orgID)
	}

	return w.sendBatch(ctx, apiKey, items, orgID)
}

// payloadHasAttachments reports whether a stored Resend payload has a
// non-empty attachments field.
func payloadHasAttachments(raw []byte) bool {
	var p struct {
		Attachments []json.RawMessage `json:"attachments"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return false
	}
	return len(p.Attachments) > 0
}

// collectBatch claims up to 99 more send jobs of the same org that are ready
// to go now. A job is ready only when it is in the Redis work queue: a
// scheduled job, a job in its undo window, and a job in retry backoff are
// not in that queue, so they stay untouched. The LREM is the test and the
// take in one step. The conditional UPDATE then claims the row, so two
// workers can never send the same job.
func (w *EmailWorker) collectBatch(ctx context.Context, orgID, leadJobID string) []sendItem {
	rows, err := w.store.Q().Query(ctx,
		`SELECT id FROM email_jobs
		 WHERE org_id = $1 AND status = 'pending' AND job_type = 'send' AND id != $2
		   AND run_after IS NULL AND retry_count = 0
		   AND NOT (resend_payload ? 'attachments')
		 ORDER BY created_at ASC
		 LIMIT 99`,
		orgID, leadJobID,
	)
	if err != nil {
		slog.Warn("email worker: batch candidate query failed", "org_id", orgID, "error", err)
		return nil
	}
	var candidates []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err == nil {
			candidates = append(candidates, id)
		}
	}
	rows.Close()

	var items []sendItem
	for _, id := range candidates {
		removed, err := w.rdb.LRem(ctx, emailJobsQueue, 1, id).Result()
		if err != nil || removed == 0 {
			continue
		}
		item := sendItem{jobID: id}
		err = w.store.Q().QueryRow(ctx,
			`UPDATE email_jobs SET status='running', heartbeat_at=now(), updated_at=now()
			 WHERE id=$1 AND status='pending' AND run_after IS NULL
			 RETURNING email_id, thread_id, resend_payload, draft_id, user_id, retry_count, max_retries`,
			id,
		).Scan(&item.emailID, &item.threadID, &item.payload, &item.draftID, &item.userID, &item.retryCount, &item.maxRetries)
		if err != nil {
			// No row: another worker or a cancel won. Any other error leaves
			// the job pending; the orphan sweep puts it back on the queue.
			if !errors.Is(err, pgx.ErrNoRows) {
				slog.Error("email worker: failed to claim batch item", "job_id", id, "error", err)
			}
			continue
		}
		items = append(items, item)
	}
	return items
}

// sendResponse is one accepted email in a Resend send response.
type sendResponse struct {
	ID        string `json:"id"`
	MessageID string `json:"message_id"`
}

func (w *EmailWorker) sendSingle(ctx context.Context, apiKey string, item sendItem, orgID string) error {
	var payload interface{}
	if err := json.Unmarshal(item.payload, &payload); err != nil {
		return &nonRetryableError{fmt.Errorf("unmarshal payload: %w", err)}
	}

	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		return err
	}

	data, err := service.DoRequestCtx(ctx, apiKey, "POST", service.ResendBaseURL()+"/emails", payload)
	if err != nil {
		return err
	}

	var resp sendResponse
	if err := json.Unmarshal(data, &resp); err != nil || resp.ID == "" {
		// Resend accepted the email. A retry would send it twice.
		return &nonRetryableError{fmt.Errorf("parse resend response: %v", err)}
	}

	w.finishSent(ctx, apiKey, item, orgID, resp, true)
	slog.Info("email worker: sent email", "email_id", item.emailID, "resend_email_id", resp.ID)
	return nil
}

// finishSent records a successful send: Resend id, Message-ID, labels,
// participants, draft cleanup, and the email.sent event. With lookupMessageID
// it asks Resend for the Message-ID when the send response did not give it.
func (w *EmailWorker) finishSent(ctx context.Context, apiKey string, item sendItem, orgID string, resp sendResponse, lookupMessageID bool) {
	messageID := util.NormalizeMsgID(resp.MessageID)
	if _, err := w.store.Q().Exec(ctx,
		`UPDATE emails SET resend_email_id=$1, status='sent',
		        message_id = COALESCE(NULLIF($3, ''), message_id), updated_at=now()
		 WHERE id=$2`,
		resp.ID, item.emailID, messageID,
	); err != nil {
		slog.Error("email worker: failed to update email status", "email_id", item.emailID, "error", err)
	}

	aliasAddr := w.populateSentAsAlias(ctx, item.emailID, orgID)

	// Ensure thread has sent label
	addLabelQ(ctx, w.store.Q(), item.threadID, orgID, "sent")

	// Stamp alias label for visibility filtering
	if aliasAddr != "" {
		addLabelQ(ctx, w.store.Q(), item.threadID, orgID, "alias:"+aliasAddr)
	}

	// Merge outbound recipients into thread participant_emails
	w.mergeParticipantEmails(ctx, item.emailID, item.threadID)

	if item.draftID != nil && *item.draftID != "" {
		if _, err := w.store.Q().Exec(ctx, "DELETE FROM drafts WHERE id = $1", *item.draftID); err != nil {
			slog.Error("email worker: failed to delete draft", "draft_id", *item.draftID, "error", err)
		}
	}

	if messageID == "" && lookupMessageID {
		w.storeMessageID(ctx, apiKey, orgID, resp.ID, item.emailID)
	}

	var domainID string
	warnIfErr(w.store.Q().QueryRow(ctx, `SELECT domain_id FROM emails WHERE id=$1`, item.emailID).Scan(&domainID),
		"email worker: domain_id lookup for event failed", "email_id", item.emailID)

	w.bus.Publish(ctx, event.Event{
		EventType: event.EmailSent,
		OrgID:     orgID,
		UserID:    item.userID,
		DomainID:  domainID,
		ThreadID:  item.threadID,
		Payload: map[string]interface{}{
			"email_id": item.emailID,
		},
	})
}

// storeMessageID reads the Message-ID of a sent email from Resend and stores
// it. A reply can then match its thread before the email.sent webhook
// arrives. It stores the normal form "<id>" that thread lookups use. A
// failure only logs: the webhook stores the id later.
func (w *EmailWorker) storeMessageID(ctx context.Context, apiKey, orgID, resendID, emailID string) {
	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		return
	}
	data, err := service.DoRequestCtx(ctx, apiKey, "GET", service.ResendBaseURL()+"/emails/"+resendID, nil)
	if err != nil {
		slog.Warn("email worker: message id lookup failed", "email_id", emailID, "error", err)
		return
	}
	var got sendResponse
	if err := json.Unmarshal(data, &got); err != nil {
		return
	}
	messageID := util.NormalizeMsgID(got.MessageID)
	if messageID == "" {
		return
	}
	if _, err := w.store.Q().Exec(ctx,
		`UPDATE emails SET message_id = $1 WHERE id = $2 AND (message_id IS NULL OR message_id = '')`,
		messageID, emailID,
	); err != nil {
		slog.Error("email worker: failed to store message id", "email_id", emailID, "error", err)
	}
}

// maxBatchMessageIDLookups limits the Message-ID lookups after one batch.
// Each lookup waits for the org rate limit, and the worker is busy meanwhile.
const maxBatchMessageIDLookups = 5

// parseBatchResponse reads a Resend batch response. Resend documents the
// form {"data":[{"id":...}]}; a bare array is also accepted.
func parseBatchResponse(data []byte) ([]sendResponse, error) {
	var wrapped struct {
		Data []sendResponse `json:"data"`
	}
	if err := json.Unmarshal(data, &wrapped); err == nil && wrapped.Data != nil {
		return wrapped.Data, nil
	}
	var bare []sendResponse
	if err := json.Unmarshal(data, &bare); err != nil {
		return nil, err
	}
	return bare, nil
}

func (w *EmailWorker) sendBatch(ctx context.Context, apiKey string, items []sendItem, orgID string) error {
	leadJobID := items[0].jobID
	// The lead item belongs to processJob: its error is returned so that
	// processJob does the retry accounting. Every other item goes through
	// failJob here.
	var leadErr error
	fail := func(item sendItem, err error) {
		if item.jobID == leadJobID {
			leadErr = err
			return
		}
		w.failJob(ctx, item.jobID, item.retryCount, item.maxRetries, err)
	}

	// Keep payloads and items at the same index. A payload that does not
	// parse fails its own job only.
	var payloads []interface{}
	var sendable []sendItem
	for _, item := range items {
		var p interface{}
		if err := json.Unmarshal(item.payload, &p); err != nil {
			fail(item, &nonRetryableError{fmt.Errorf("unmarshal payload: %w", err)})
			continue
		}
		payloads = append(payloads, p)
		sendable = append(sendable, item)
	}
	if len(sendable) == 0 {
		return leadErr
	}

	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		for _, item := range sendable {
			fail(item, err)
		}
		return leadErr
	}

	data, err := service.DoRequestCtx(ctx, apiKey, "POST", service.ResendBaseURL()+"/emails/batch", payloads)
	if err != nil {
		slog.Error("email worker: batch send failed", "error", err, "count", len(sendable))
		for _, item := range sendable {
			fail(item, err)
		}
		return leadErr
	}

	resps, err := parseBatchResponse(data)
	if err != nil {
		// Resend accepted the batch. A retry would send every email twice.
		parseErr := &nonRetryableError{fmt.Errorf("parse batch response: %w", err)}
		slog.Error("email worker: parse batch response", "error", err)
		for _, item := range sendable {
			fail(item, parseErr)
		}
		return leadErr
	}

	slog.Info("email worker: batch sent", "count", len(resps), "org_id", orgID)

	if len(resps) != len(sendable) {
		slog.Warn("email worker: batch response count mismatch",
			"expected", len(sendable), "got", len(resps), "org_id", orgID)
	}

	// Resend returns the responses in the order of the request array, so
	// resps[i] belongs to sendable[i].
	for i, item := range sendable {
		if i >= len(resps) || resps[i].ID == "" {
			fail(item, &nonRetryableError{fmt.Errorf("unmapped in batch response")})
			continue
		}

		slog.Info("email worker: batch item mapped",
			"index", i, "job_id", item.jobID,
			"email_id", item.emailID, "resend_email_id", resps[i].ID)

		w.finishSent(ctx, apiKey, item, orgID, resps[i], i < maxBatchMessageIDLookups)

		if item.jobID == leadJobID {
			continue // processJob marks the lead completed
		}
		if _, execErr := w.store.Q().Exec(ctx,
			`UPDATE email_jobs SET status='completed', heartbeat_at=now(), updated_at=now() WHERE id=$1`,
			item.jobID,
		); execErr != nil {
			slog.Error("email worker: failed to mark batch job completed", "job_id", item.jobID, "error", execErr)
		}
	}

	return leadErr
}

// mergeParticipantEmails updates the thread's participant_emails to include
// to_addresses and cc_addresses from the sent email.
func (w *EmailWorker) mergeParticipantEmails(ctx context.Context, emailID, threadID string) {
	var toJSON, ccJSON json.RawMessage
	if err := w.store.Q().QueryRow(ctx,
		`SELECT to_addresses, cc_addresses FROM emails WHERE id = $1`, emailID,
	).Scan(&toJSON, &ccJSON); err != nil {
		return
	}

	// Combine to + cc into a single JSON array
	var to, cc []string
	warnIfErr(json.Unmarshal(toJSON, &to), "email worker: failed to unmarshal to addresses for participant merge", "email_id", emailID)
	warnIfErr(json.Unmarshal(ccJSON, &cc), "email worker: failed to unmarshal cc addresses for participant merge", "email_id", emailID)
	// Thread participants are bare lowercase addresses.
	all := util.NormalizeAddresses(to, cc)
	if len(all) == 0 {
		return
	}

	allJSON, err := json.Marshal(all)
	if err != nil {
		return
	}

	if _, err := w.store.Q().Exec(ctx,
		`UPDATE threads SET participant_emails = (
		   SELECT jsonb_agg(DISTINCT val) FROM (
		     SELECT jsonb_array_elements(participant_emails) AS val
		     UNION
		     SELECT jsonb_array_elements($2::jsonb) AS val
		   ) sub
		 ), updated_at = now()
		 WHERE id = $1`, threadID, string(allJSON),
	); err != nil {
		slog.Error("email worker: failed to merge participant_emails", "thread_id", threadID, "error", err)
	}
}

// populateSentAsAlias sets sent_as_alias on the email if from_address matches an alias.
// Returns the alias address if matched, empty string otherwise.
func (w *EmailWorker) populateSentAsAlias(ctx context.Context, emailID, orgID string) string {
	var fromAddress string
	if err := w.store.Q().QueryRow(ctx,
		`SELECT from_address FROM emails WHERE id=$1`, emailID,
	).Scan(&fromAddress); err != nil {
		return ""
	}

	fromClean := strings.ToLower(strings.TrimSpace(fromAddress))
	var aliasAddress string
	if err := w.store.Q().QueryRow(ctx,
		`SELECT address FROM aliases WHERE org_id=$1 AND address=$2`,
		orgID, fromClean,
	).Scan(&aliasAddress); err != nil {
		return ""
	}

	if _, err := w.store.Q().Exec(ctx,
		`UPDATE emails SET sent_as_alias=$1, updated_at=now() WHERE id=$2`,
		aliasAddress, emailID,
	); err != nil {
		slog.Error("email worker: failed to set sent_as_alias", "email_id", emailID, "error", err)
	}
	return aliasAddress
}
