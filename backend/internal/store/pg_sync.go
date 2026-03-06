package store

import (
	"context"
	"time"
)

func (s *PgStore) CreateSyncJob(ctx context.Context, orgID, userID string) (string, error) {
	var jobID string
	err := s.q.QueryRow(ctx,
		`INSERT INTO sync_jobs (org_id, user_id) VALUES ($1, $2) RETURNING id`,
		orgID, userID,
	).Scan(&jobID)
	return jobID, err
}

func (s *PgStore) GetSyncJob(ctx context.Context, jobID, orgID string) (map[string]any, error) {
	var id, status, phase string
	var imported, total, sentCount, receivedCount, threadCount, addressCount int
	var errorMessage *string
	var createdAt, updatedAt time.Time

	err := s.q.QueryRow(ctx,
		`SELECT id, status, phase, imported, total, sent_count, received_count,
		 thread_count, address_count, error_message, created_at, updated_at
		 FROM sync_jobs WHERE id = $1 AND org_id = $2`, jobID, orgID,
	).Scan(&id, &status, &phase, &imported, &total, &sentCount, &receivedCount,
		&threadCount, &addressCount, &errorMessage, &createdAt, &updatedAt)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"id":             id,
		"status":         status,
		"phase":          phase,
		"imported":       imported,
		"total":          total,
		"sent_count":     sentCount,
		"received_count": receivedCount,
		"thread_count":   threadCount,
		"address_count":  addressCount,
		"created_at":     createdAt,
		"updated_at":     updatedAt,
	}
	if errorMessage != nil {
		result["error_message"] = *errorMessage
	}
	return result, nil
}
