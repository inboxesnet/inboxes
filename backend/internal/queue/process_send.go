package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/service"
)

type sendItem struct {
	jobID    string
	emailID  string
	threadID string
	payload  json.RawMessage
	draftID  *string
	userID   string
}

func (w *EmailWorker) processSend(ctx context.Context, jobID, orgID, userID string) error {
	var emailID, threadID string
	var resendPayloadRaw []byte
	var draftID *string
	err := w.store.Q().QueryRow(ctx,
		`SELECT email_id, thread_id, resend_payload, draft_id
		 FROM email_jobs WHERE id = $1`,
		jobID,
	).Scan(&emailID, &threadID, &resendPayloadRaw, &draftID)
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
		if domainStatus == "disconnected" || domainStatus == "pending" {
			return fmt.Errorf("domain is %s, cannot send email", domainStatus)
		}
	}

	apiKey, err := w.resendSvc.GetOrgAPIKey(ctx, orgID)
	if err != nil {
		return fmt.Errorf("get org api key: %w", err)
	}

	items := []sendItem{{
		jobID:    jobID,
		emailID:  emailID,
		threadID: threadID,
		payload:  resendPayloadRaw,
		draftID:  draftID,
		userID:   userID,
	}}

	// Batch collection: grab up to 99 more pending send jobs for same org
	rows, err := w.store.Q().Query(ctx,
		`SELECT id, email_id, thread_id, resend_payload, draft_id, user_id
		 FROM email_jobs
		 WHERE org_id = $1 AND status = 'pending' AND job_type = 'send' AND id != $2
		 ORDER BY created_at ASC
		 LIMIT 99
		 FOR UPDATE SKIP LOCKED`,
		orgID, jobID,
	)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item sendItem
			if err := rows.Scan(&item.jobID, &item.emailID, &item.threadID, &item.payload, &item.draftID, &item.userID); err != nil {
				continue
			}
			items = append(items, item)
		}
		rows.Close()

		// Mark additional items as running
		for _, item := range items[1:] {
			if _, err := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET status='running', heartbeat_at=now(), updated_at=now() WHERE id=$1`,
				item.jobID,
			); err != nil {
				slog.Error("email worker: failed to mark batch item running", "job_id", item.jobID, "error", err)
			}
			w.rdb.LRem(ctx, emailJobsQueue, 1, item.jobID)
		}
	}

	if len(items) == 1 {
		return w.sendSingle(ctx, apiKey, items[0], orgID)
	}

	return w.sendBatch(ctx, apiKey, items, orgID)
}

func (w *EmailWorker) sendSingle(ctx context.Context, apiKey string, item sendItem, orgID string) error {
	var payload interface{}
	if err := json.Unmarshal(item.payload, &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		return err
	}

	data, err := service.DoRequest(apiKey, "POST", service.ResendBaseURL()+"/emails", payload)
	if err != nil {
		return err
	}

	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse resend response: %w", err)
	}

	if _, err := w.store.Q().Exec(ctx,
		`UPDATE emails SET resend_email_id=$1, status='sent', updated_at=now() WHERE id=$2`,
		resp.ID, item.emailID,
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

	slog.Info("email worker: sent email", "email_id", item.emailID, "resend_email_id", resp.ID)

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

	return nil
}

func (w *EmailWorker) sendBatch(ctx context.Context, apiKey string, items []sendItem, orgID string) error {
	var payloads []interface{}
	for _, item := range items {
		var p interface{}
		if err := json.Unmarshal(item.payload, &p); err != nil {
			continue
		}
		payloads = append(payloads, p)
	}

	if err := w.limiter.WaitForOrg(ctx, orgID); err != nil {
		return err
	}

	data, err := service.DoRequest(apiKey, "POST", service.ResendBaseURL()+"/emails/batch", payloads)
	if err != nil {
		slog.Error("email worker: batch send failed, re-enqueueing individually", "error", err, "count", len(items))
		for _, item := range items {
			if _, execErr := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET status='pending', retry_count=retry_count+1,
				 error_message=$1, updated_at=now() WHERE id=$2`,
				err.Error(), item.jobID,
			); execErr != nil {
				slog.Error("email worker: failed to reset batch item", "job_id", item.jobID, "error", execErr)
			}
			if lpushErr := w.rdb.LPush(ctx, emailJobsQueue, item.jobID).Err(); lpushErr != nil {
				slog.Error("email worker: failed to re-enqueue batch item", "job_id", item.jobID, "error", lpushErr)
			}
		}
		return nil
	}

	var resps []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &resps); err != nil {
		slog.Error("email worker: parse batch response", "error", err)
		for _, item := range items {
			if _, execErr := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET status='pending', retry_count=retry_count+1,
				 error_message='failed to parse batch response', updated_at=now() WHERE id=$1`,
				item.jobID,
			); execErr != nil {
				slog.Error("email worker: failed to reset batch item", "job_id", item.jobID, "error", execErr)
			}
			if lpushErr := w.rdb.LPush(ctx, emailJobsQueue, item.jobID).Err(); lpushErr != nil {
				slog.Error("email worker: failed to re-enqueue batch item", "job_id", item.jobID, "error", lpushErr)
			}
		}
		return nil
	}

	slog.Info("email worker: batch sent", "count", len(resps), "org_id", orgID)

	if len(resps) != len(items) {
		slog.Warn("email worker: batch response count mismatch",
			"expected", len(items), "got", len(resps), "org_id", orgID)
	}

	// ORDERING ASSUMPTION: Resend's batch API returns responses in the same
	// positional order as the request array, so resps[i] corresponds to items[i].
	// If this ever changes, mismatched resend_email_id mapping will be caught by
	// the count-mismatch warning above and the per-item audit log below.
	for i, item := range items {
		if i >= len(resps) {
			if _, execErr := w.store.Q().Exec(ctx,
				`UPDATE email_jobs SET status='pending', retry_count=retry_count+1,
				 error_message='unmapped in batch response', updated_at=now() WHERE id=$1`,
				item.jobID,
			); execErr != nil {
				slog.Error("email worker: failed to reset unmapped item", "job_id", item.jobID, "error", execErr)
			}
			if lpushErr := w.rdb.LPush(ctx, emailJobsQueue, item.jobID).Err(); lpushErr != nil {
				slog.Error("email worker: failed to re-enqueue unmapped item", "job_id", item.jobID, "error", lpushErr)
			}
			continue
		}

		resendID := resps[i].ID
		slog.Info("email worker: batch item mapped",
			"index", i, "job_id", item.jobID,
			"email_id", item.emailID, "resend_email_id", resendID)

		if _, execErr := w.store.Q().Exec(ctx,
			`UPDATE emails SET resend_email_id=$1, status='sent', updated_at=now() WHERE id=$2`,
			resendID, item.emailID,
		); execErr != nil {
			slog.Error("email worker: failed to update batch email", "email_id", item.emailID, "error", execErr)
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
			if _, execErr := w.store.Q().Exec(ctx, "DELETE FROM drafts WHERE id = $1", *item.draftID); execErr != nil {
				slog.Error("email worker: failed to delete draft in batch", "draft_id", *item.draftID, "error", execErr)
			}
		}

		if _, execErr := w.store.Q().Exec(ctx,
			`UPDATE email_jobs SET status='completed', heartbeat_at=now(), updated_at=now() WHERE id=$1`,
			item.jobID,
		); execErr != nil {
			slog.Error("email worker: failed to mark batch job completed", "job_id", item.jobID, "error", execErr)
		}

		var domainID string
		warnIfErr(w.store.Q().QueryRow(ctx, `SELECT domain_id FROM emails WHERE id=$1`, item.emailID).Scan(&domainID),
			"email worker: domain_id lookup for batch event failed", "email_id", item.emailID)
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

	return nil
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
	all := append(to, cc...)
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
