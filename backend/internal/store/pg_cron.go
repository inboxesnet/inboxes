package store

import (
	"context"
	"log/slog"
)

func (s *PgStore) PurgeExpiredTrash(ctx context.Context) (int64, error) {
	// Soft-delete threads with trash label past expiry
	tag, err := s.q.Exec(ctx,
		`WITH expired AS (
			SELECT t.id FROM threads t
			JOIN thread_labels tl ON tl.thread_id = t.id AND tl.label = 'trash'
			WHERE t.trash_expires_at < now() AND t.deleted_at IS NULL
		)
		UPDATE threads SET deleted_at = now(), updated_at = now() WHERE id IN (SELECT id FROM expired)`)
	if err != nil {
		return 0, err
	}

	// Clean up orphaned labels
	if _, err := s.q.Exec(ctx,
		`DELETE FROM thread_labels WHERE thread_id IN (SELECT id FROM threads WHERE deleted_at IS NOT NULL)`); err != nil {
		slog.Error("cron: label cleanup failed", "error", err)
	}

	return tag.RowsAffected(), nil
}

func (s *PgStore) CleanupStaleWebhooks(ctx context.Context, orgIDs []string) error {
	// This method is a no-op at the store level because the actual cleanup
	// requires Resend API calls. The handler iterates orgs and calls
	// ResendSvc.Fetch to list/delete webhooks. The store only provides the
	// org data. The orgIDs parameter is unused — the handler does its own
	// query for orgs with webhook IDs.
	return nil
}
