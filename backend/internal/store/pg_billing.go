package store

import (
	"context"
	"time"
)

func (s *PgStore) GetBillingInfo(ctx context.Context, orgID string) (map[string]any, error) {
	var plan string
	var planExpiresAt *time.Time
	var stripeSubID *string
	var stripeCustomerID *string

	err := s.q.QueryRow(ctx,
		"SELECT plan, plan_expires_at, stripe_subscription_id, stripe_customer_id FROM orgs WHERE id = $1",
		orgID,
	).Scan(&plan, &planExpiresAt, &stripeSubID, &stripeCustomerID)
	if err != nil {
		return nil, err
	}

	result := map[string]any{
		"plan":                   plan,
		"plan_expires_at":        planExpiresAt,
		"stripe_subscription_id": stripeSubID,
		"stripe_customer_id":     stripeCustomerID,
	}
	return result, nil
}

func (s *PgStore) GetStripeCustomerID(ctx context.Context, orgID string) (*string, error) {
	var customerID *string
	err := s.q.QueryRow(ctx,
		"SELECT stripe_customer_id FROM orgs WHERE id = $1", orgID,
	).Scan(&customerID)
	if err != nil {
		return nil, err
	}
	return customerID, nil
}

func (s *PgStore) SetStripeCustomerID(ctx context.Context, orgID, customerID string) error {
	_, err := s.q.Exec(ctx,
		"UPDATE orgs SET stripe_customer_id = $1 WHERE id = $2",
		customerID, orgID,
	)
	return err
}

func (s *PgStore) UpdateOrgPlan(ctx context.Context, orgID, plan, subID string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET plan = $1, stripe_subscription_id = $2, updated_at = now() WHERE id = $3`,
		plan, subID, orgID,
	)
	return err
}

func (s *PgStore) SetPlanExpiry(ctx context.Context, orgID string, expiresAt time.Time) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET plan_expires_at = $1, updated_at = now() WHERE id = $2`,
		expiresAt, orgID,
	)
	return err
}

func (s *PgStore) ClearPlanExpiry(ctx context.Context, orgID string) error {
	_, err := s.q.Exec(ctx,
		`UPDATE orgs SET plan_expires_at = NULL, updated_at = now() WHERE id = $1`,
		orgID,
	)
	return err
}

func (s *PgStore) InsertStripeEvent(ctx context.Context, eventID string) (bool, error) {
	tag, err := s.q.Exec(ctx,
		`INSERT INTO stripe_events (event_id, event_type) VALUES ($1, '') ON CONFLICT DO NOTHING`,
		eventID,
	)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}
