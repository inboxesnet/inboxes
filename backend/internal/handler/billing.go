package handler

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/inboxes/backend/internal/event"
	"github.com/inboxes/backend/internal/middleware"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stripe/stripe-go/v82"
	billingportalSession "github.com/stripe/stripe-go/v82/billingportal/session"
	checkoutSession "github.com/stripe/stripe-go/v82/checkout/session"
	"github.com/stripe/stripe-go/v82/customer"
	"github.com/stripe/stripe-go/v82/subscription"
	"github.com/stripe/stripe-go/v82/webhook"
)

type BillingHandler struct {
	DB                 *pgxpool.Pool
	Bus                *event.Bus
	StripeKey          string
	StripePriceID      string
	StripeWebhookSecret string
	AppURL             string
}

// Checkout creates a Stripe checkout session for the org.
func (h *BillingHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	ctx := r.Context()

	stripe.Key = h.StripeKey

	// Get or create Stripe customer
	var stripeCustomerID *string
	var orgName string
	err := h.DB.QueryRow(ctx,
		"SELECT stripe_customer_id, name FROM orgs WHERE id = $1", claims.OrgID,
	).Scan(&stripeCustomerID, &orgName)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	var custID string
	if stripeCustomerID != nil && *stripeCustomerID != "" {
		custID = *stripeCustomerID
	} else {
		// Get admin email for customer
		var email string
		warnIfErr(h.DB.QueryRow(ctx,
			"SELECT email FROM users WHERE id = $1", claims.UserID,
		).Scan(&email), "billing: failed to look up admin email", "user_id", claims.UserID)

		params := &stripe.CustomerParams{
			Name:  stripe.String(orgName),
			Email: stripe.String(email),
		}
		cust, err := customer.New(params)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to create Stripe customer")
			return
		}
		custID = cust.ID
		if _, err := h.DB.Exec(ctx,
			"UPDATE orgs SET stripe_customer_id = $1 WHERE id = $2",
			custID, claims.OrgID,
		); err != nil {
			slog.Error("billing: failed to save stripe customer", "org_id", claims.OrgID, "error", err)
		}
	}

	// Get first domain ID for success redirect
	var firstDomainID string
	warnIfErr(h.DB.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order LIMIT 1",
		claims.OrgID,
	).Scan(&firstDomainID), "billing: failed to look up first domain for redirect", "org_id", claims.OrgID)

	// Validate domain ID is a proper UUID before interpolating into URLs
	if firstDomainID != "" {
		if _, err := uuid.Parse(firstDomainID); err != nil {
			slog.Error("billing: invalid domain ID for redirect", "domain_id", firstDomainID)
			firstDomainID = ""
		}
	}

	successURL := h.AppURL
	if firstDomainID != "" {
		successURL += "/d/" + firstDomainID + "/inbox?billing=success"
	} else {
		successURL += "/onboarding?billing=success"
	}
	cancelURL := h.AppURL
	if firstDomainID != "" {
		cancelURL += "/d/" + firstDomainID + "/inbox"
	}

	params := &stripe.CheckoutSessionParams{
		Mode:       stripe.String(string(stripe.CheckoutSessionModeSubscription)),
		Customer:   stripe.String(custID),
		SuccessURL: stripe.String(successURL),
		CancelURL:  stripe.String(cancelURL),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				Price:    stripe.String(h.StripePriceID),
				Quantity: stripe.Int64(1),
			},
		},
	}

	sess, err := checkoutSession.New(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create checkout session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": sess.URL})
}

// Portal creates a Stripe billing portal session.
func (h *BillingHandler) Portal(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	if claims.Role != "admin" {
		writeError(w, http.StatusForbidden, "admin required")
		return
	}
	ctx := r.Context()

	stripe.Key = h.StripeKey

	var stripeCustomerID *string
	err := h.DB.QueryRow(ctx,
		"SELECT stripe_customer_id FROM orgs WHERE id = $1", claims.OrgID,
	).Scan(&stripeCustomerID)
	if err != nil || stripeCustomerID == nil || *stripeCustomerID == "" {
		writeError(w, http.StatusBadRequest, "no billing account found")
		return
	}

	// Build return URL with domain context
	returnURL := h.AppURL
	var firstDomainID string
	warnIfErr(h.DB.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order LIMIT 1",
		claims.OrgID,
	).Scan(&firstDomainID), "billing: failed to look up first domain for portal redirect", "org_id", claims.OrgID)
	if firstDomainID != "" {
		if _, err := uuid.Parse(firstDomainID); err != nil {
			slog.Error("billing: invalid domain ID for portal redirect", "domain_id", firstDomainID)
			firstDomainID = ""
		}
	}
	if firstDomainID != "" {
		returnURL += "/d/" + firstDomainID + "/inbox"
	}

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(*stripeCustomerID),
		ReturnURL: stripe.String(returnURL),
	}

	sess, err := billingportalSession.New(params)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create portal session")
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"url": sess.URL})
}

// GetBilling returns the org's billing info.
func (h *BillingHandler) GetBilling(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ctx := r.Context()

	stripe.Key = h.StripeKey

	var plan string
	var planExpiresAt *time.Time
	var stripeSubID *string
	var stripeCustomerID *string
	err := h.DB.QueryRow(ctx,
		"SELECT plan, plan_expires_at, stripe_subscription_id, stripe_customer_id FROM orgs WHERE id = $1",
		claims.OrgID,
	).Scan(&plan, &planExpiresAt, &stripeSubID, &stripeCustomerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	// If grace period has expired, report as "free" to the client
	effectivePlan := plan
	if plan == "cancelled" && planExpiresAt != nil && planExpiresAt.Before(time.Now()) {
		effectivePlan = "free"
	}

	resp := map[string]interface{}{
		"plan":            effectivePlan,
		"plan_expires_at": planExpiresAt,
		"billing_enabled": true,
	}

	if stripeSubID != nil && *stripeSubID != "" {
		params := &stripe.SubscriptionParams{}
		params.AddExpand("items")
		sub, err := subscription.Get(*stripeSubID, params)
		if err == nil {
			subInfo := map[string]interface{}{
				"status":               sub.Status,
				"cancel_at_period_end": sub.CancelAtPeriodEnd,
			}
			// In stripe-go v82, current_period_end is on SubscriptionItem
			if sub.Items != nil && len(sub.Items.Data) > 0 {
				subInfo["current_period_end"] = time.Unix(sub.Items.Data[0].CurrentPeriodEnd, 0)
			}
			resp["subscription"] = subInfo
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

// HandleStripeWebhook processes Stripe webhook events.
func (h *BillingHandler) HandleStripeWebhook(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 65536))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	sig := r.Header.Get("Stripe-Signature")
	event, err := webhook.ConstructEvent(body, sig, h.StripeWebhookSecret)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid signature")
		return
	}

	ctx := r.Context()

	// Dedup: skip events we've already processed
	tag, err := h.DB.Exec(ctx,
		`INSERT INTO stripe_events (event_id, event_type) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		event.ID, event.Type,
	)
	if err != nil {
		slog.Error("stripe: dedup insert failed", "event_id", event.ID, "error", err)
		// Continue processing — better to risk a duplicate than to drop an event
	} else if tag.RowsAffected() == 0 {
		slog.Info("stripe: duplicate event skipped", "event_id", event.ID, "type", event.Type)
		w.WriteHeader(http.StatusOK)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := decodeStripeObject(event.Data.Raw, &session); err != nil {
			slog.Error("stripe: decode checkout.session.completed", "error", err)
			break
		}
		if session.Customer != nil && session.Subscription != nil {
			tag, err := h.DB.Exec(ctx,
				`UPDATE orgs SET stripe_subscription_id = $1, plan = 'pro', plan_expires_at = NULL
				 WHERE stripe_customer_id = $2`,
				session.Subscription.ID, session.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after checkout", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("stripe: no org found for customer", "customer_id", session.Customer.ID)
			} else {
				h.publishPlanChanged(ctx, session.Customer.ID, "pro")
			}
		}

	case "customer.subscription.created":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.created", "error", err)
			break
		}
		if sub.Customer != nil {
			tag, err := h.DB.Exec(ctx,
				`UPDATE orgs SET stripe_subscription_id = $1, plan = 'pro', plan_expires_at = NULL
				 WHERE stripe_customer_id = $2`,
				sub.ID, sub.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after subscription created", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("stripe: no org found for customer", "customer_id", sub.Customer.ID)
			} else {
				h.publishPlanChanged(ctx, sub.Customer.ID, "pro")
			}
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.deleted", "error", err)
			break
		}
		if sub.Customer != nil {
			tag, err := h.DB.Exec(ctx,
				`UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1, stripe_subscription_id = NULL
				 WHERE stripe_customer_id = $2`,
				time.Now().Add(7*24*time.Hour), sub.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after subscription deleted", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("stripe: no org found for customer", "customer_id", sub.Customer.ID)
			} else {
				h.publishPlanChanged(ctx, sub.Customer.ID, "cancelled")
			}
		}

	case "customer.subscription.updated":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.updated", "error", err)
			break
		}
		if sub.Customer != nil {
			var plan string
			switch sub.Status {
			case stripe.SubscriptionStatusActive, stripe.SubscriptionStatusTrialing:
				plan = "pro"
			case stripe.SubscriptionStatusPastDue:
				plan = "past_due"
			case stripe.SubscriptionStatusUnpaid, stripe.SubscriptionStatusCanceled:
				plan = "cancelled"
			default:
				slog.Info("stripe: subscription.updated with unhandled status", "status", sub.Status, "customer_id", sub.Customer.ID)
			}
			if plan != "" {
				updateQ := `UPDATE orgs SET plan = $1, updated_at = now() WHERE stripe_customer_id = $2`
				// Clear plan_expires_at when restoring to pro
				if plan == "pro" {
					updateQ = `UPDATE orgs SET plan = $1, plan_expires_at = NULL, updated_at = now() WHERE stripe_customer_id = $2`
				}
				tag, err := h.DB.Exec(ctx, updateQ, plan, sub.Customer.ID)
				if err != nil {
					slog.Error("stripe: update org after subscription updated", "error", err)
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				if tag.RowsAffected() == 0 {
					slog.Warn("stripe: no org found for customer", "customer_id", sub.Customer.ID)
				} else {
					h.publishPlanChanged(ctx, sub.Customer.ID, plan)
				}
			}
		}

	case "customer.subscription.paused":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.paused", "error", err)
			break
		}
		if sub.Customer != nil {
			// Treat paused like past_due — keep access for now
			tag, err := h.DB.Exec(ctx,
				`UPDATE orgs SET plan = 'past_due', updated_at = now() WHERE stripe_customer_id = $1`,
				sub.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after subscription paused", "error", err)
				break
			}
			if tag.RowsAffected() > 0 {
				h.publishPlanChanged(ctx, sub.Customer.ID, "past_due")
			}
		}

	case "customer.subscription.resumed":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.resumed", "error", err)
			break
		}
		if sub.Customer != nil {
			tag, err := h.DB.Exec(ctx,
				`UPDATE orgs SET plan = 'pro', plan_expires_at = NULL, updated_at = now() WHERE stripe_customer_id = $1`,
				sub.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after subscription resumed", "error", err)
				break
			}
			if tag.RowsAffected() > 0 {
				h.publishPlanChanged(ctx, sub.Customer.ID, "pro")
			}
		}

	case "invoice.payment_succeeded":
		var inv stripe.Invoice
		if err := decodeStripeObject(event.Data.Raw, &inv); err != nil {
			slog.Error("stripe: decode invoice.payment_succeeded", "error", err)
			break
		}
		if inv.Customer != nil {
			tag, err := h.DB.Exec(ctx,
				`UPDATE orgs SET plan = 'pro', plan_expires_at = NULL
				 WHERE stripe_customer_id = $1`,
				inv.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after payment succeeded", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("stripe: no org found for customer", "customer_id", inv.Customer.ID)
			} else {
				h.publishPlanChanged(ctx, inv.Customer.ID, "pro")
			}
		}

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := decodeStripeObject(event.Data.Raw, &inv); err != nil {
			slog.Error("stripe: decode invoice.payment_failed", "error", err)
			break
		}
		if inv.Customer != nil {
			// Set to past_due (not cancelled) — Stripe may retry payment
			tag, err := h.DB.Exec(ctx,
				`UPDATE orgs SET plan = 'past_due', plan_expires_at = $1
				 WHERE stripe_customer_id = $2`,
				time.Now().Add(14*24*time.Hour), inv.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after payment failed", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("stripe: no org found for customer", "customer_id", inv.Customer.ID)
			} else {
				h.publishPlanChanged(ctx, inv.Customer.ID, "past_due")
			}
		}

	case "invoice.upcoming":
		// Notification only — don't change plan state
		var inv stripe.Invoice
		if err := decodeStripeObject(event.Data.Raw, &inv); err != nil {
			slog.Error("stripe: decode invoice.upcoming", "error", err)
			break
		}
		if inv.Customer != nil {
			slog.Info("stripe: upcoming invoice", "customer_id", inv.Customer.ID)
		}

	default:
		slog.Warn("stripe: unhandled event type", "type", event.Type, "event_id", event.ID)
	}

	w.WriteHeader(http.StatusOK)
}

func (h *BillingHandler) publishPlanChanged(ctx context.Context, customerID, plan string) {
	if h.Bus == nil {
		return
	}
	var orgID string
	warnIfErr(h.DB.QueryRow(ctx, `SELECT id FROM orgs WHERE stripe_customer_id = $1`, customerID).Scan(&orgID),
		"billing: org lookup by stripe customer failed", "customer_id", customerID)
	if orgID == "" {
		return
	}
	h.Bus.Publish(ctx, event.Event{
		EventType: event.PlanChanged,
		OrgID:     orgID,
		Payload:   map[string]interface{}{"plan": plan},
	})
}

// decodeStripeObject parses raw JSON from a Stripe event into a typed struct.
func decodeStripeObject(raw []byte, dst interface{}) error {
	return json.Unmarshal(raw, dst)
}
