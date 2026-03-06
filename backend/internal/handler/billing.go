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
	"github.com/inboxes/backend/internal/store"
	"github.com/stripe/stripe-go/v84"
	billingportalSession "github.com/stripe/stripe-go/v84/billingportal/session"
	checkoutSession "github.com/stripe/stripe-go/v84/checkout/session"
	"github.com/stripe/stripe-go/v84/customer"
	"github.com/stripe/stripe-go/v84/subscription"
	"github.com/stripe/stripe-go/v84/webhook"
)

type BillingHandler struct {
	Store               store.Store
	Bus                 *event.Bus
	StripeKey           string
	StripePriceID       string
	StripeWebhookSecret string
	AppURL              string

	// VerifyWebhook overrides Stripe SDK signature verification (for testing).
	// If nil, the handler uses webhook.ConstructEvent from the Stripe SDK.
	VerifyWebhook func(payload []byte, header string, secret string) (stripe.Event, error)
}

// Checkout creates a Stripe checkout session for the org.
func (h *BillingHandler) Checkout(w http.ResponseWriter, r *http.Request) {
	claims := middleware.GetCurrentUser(r.Context())
	ctx := r.Context()

	stripe.Key = h.StripeKey

	// Get or create Stripe customer
	stripeCustomerID, err := h.Store.GetStripeCustomerID(ctx, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	var custID string
	if stripeCustomerID != nil && *stripeCustomerID != "" {
		custID = *stripeCustomerID
	} else {
		// Get org name for Stripe customer creation
		orgSettings, _ := h.Store.GetOrgSettings(ctx, claims.OrgID)
		orgName := ""
		if orgSettings != nil {
			if n, ok := orgSettings["name"].(string); ok {
				orgName = n
			}
		}

		// Get admin email for customer
		var email string
		warnIfErr(h.Store.Q().QueryRow(ctx,
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
		if err := h.Store.SetStripeCustomerID(ctx, claims.OrgID, custID); err != nil {
			slog.Error("billing: failed to save stripe customer", "org_id", claims.OrgID, "error", err)
		}
	}

	// Get first domain ID for success redirect
	var firstDomainID string
	warnIfErr(h.Store.Q().QueryRow(ctx,
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
	ctx := r.Context()

	stripe.Key = h.StripeKey

	stripeCustomerID, err := h.Store.GetStripeCustomerID(ctx, claims.OrgID)
	if err != nil || stripeCustomerID == nil || *stripeCustomerID == "" {
		writeError(w, http.StatusBadRequest, "no billing account found")
		return
	}

	// Build return URL with domain context
	returnURL := h.AppURL
	var firstDomainID string
	warnIfErr(h.Store.Q().QueryRow(ctx,
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

	billingInfo, err := h.Store.GetBillingInfo(ctx, claims.OrgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "org not found")
		return
	}

	plan, _ := billingInfo["plan"].(string)
	planExpiresAt, _ := billingInfo["plan_expires_at"].(*time.Time)
	stripeSubID, _ := billingInfo["stripe_subscription_id"].(*string)

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
		stripe.Key = h.StripeKey
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

	var event stripe.Event
	if h.VerifyWebhook != nil {
		event, err = h.VerifyWebhook(body, sig, h.StripeWebhookSecret)
	} else {
		event, err = webhook.ConstructEvent(body, sig, h.StripeWebhookSecret)
	}
	if err != nil {
		slog.Error("stripe: webhook signature verification failed", "error", err, "sig_header", sig[:min(len(sig), 50)])
		writeError(w, http.StatusBadRequest, "invalid signature")
		return
	}

	ctx := r.Context()

	// Dedup: skip events we've already processed
	inserted, err := h.Store.InsertStripeEvent(ctx, event.ID)
	if err != nil {
		slog.Error("stripe: dedup insert failed", "event_id", event.ID, "error", err)
		// Continue processing — better to risk a duplicate than to drop an event
	} else if !inserted {
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
			h.updateOrgByCustomer(ctx, w, session.Customer.ID, "pro", session.Subscription.ID, nil)
		}

	case "customer.subscription.created":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.created", "error", err)
			break
		}
		if sub.Customer != nil {
			h.updateOrgByCustomer(ctx, w, sub.Customer.ID, "pro", sub.ID, nil)
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.deleted", "error", err)
			break
		}
		if sub.Customer != nil {
			expiry := time.Now().Add(7 * 24 * time.Hour)
			h.updateOrgByCustomerCancelled(ctx, w, sub.Customer.ID, &expiry)
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
				tag, err := h.Store.Q().Exec(ctx, updateQ, plan, sub.Customer.ID)
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
			tag, err := h.Store.Q().Exec(ctx,
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
			tag, err := h.Store.Q().Exec(ctx,
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
			tag, err := h.Store.Q().Exec(ctx,
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
			tag, err := h.Store.Q().Exec(ctx,
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

	case "invoice.paid":
		var inv stripe.Invoice
		if err := decodeStripeObject(event.Data.Raw, &inv); err != nil {
			slog.Error("stripe: decode invoice.paid", "error", err)
			break
		}
		if inv.Customer != nil {
			tag, err := h.Store.Q().Exec(ctx,
				`UPDATE orgs SET plan = 'pro', plan_expires_at = NULL
				 WHERE stripe_customer_id = $1`,
				inv.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after invoice paid", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("stripe: no org found for customer", "customer_id", inv.Customer.ID)
			} else {
				h.publishPlanChanged(ctx, inv.Customer.ID, "pro")
			}
		}

	case "invoice.payment_action_required":
		var inv stripe.Invoice
		if err := decodeStripeObject(event.Data.Raw, &inv); err != nil {
			slog.Error("stripe: decode invoice.payment_action_required", "error", err)
			break
		}
		if inv.Customer != nil {
			slog.Warn("stripe: payment action required", "customer_id", inv.Customer.ID)
		}

	case "invoice.marked_uncollectible":
		var inv stripe.Invoice
		if err := decodeStripeObject(event.Data.Raw, &inv); err != nil {
			slog.Error("stripe: decode invoice.marked_uncollectible", "error", err)
			break
		}
		if inv.Customer != nil {
			expiry := time.Now().Add(7 * 24 * time.Hour)
			tag, err := h.Store.Q().Exec(ctx,
				`UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1
				 WHERE stripe_customer_id = $2`,
				expiry, inv.Customer.ID,
			)
			if err != nil {
				slog.Error("stripe: update org after invoice uncollectible", "error", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			if tag.RowsAffected() == 0 {
				slog.Warn("stripe: no org found for customer", "customer_id", inv.Customer.ID)
			} else {
				h.publishPlanChanged(ctx, inv.Customer.ID, "cancelled")
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

	case "customer.subscription.pending_update_applied":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.pending_update_applied", "error", err)
			break
		}
		if sub.Customer != nil {
			slog.Info("stripe: pending subscription update applied", "customer_id", sub.Customer.ID)
		}

	case "customer.subscription.pending_update_expired":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.pending_update_expired", "error", err)
			break
		}
		if sub.Customer != nil {
			slog.Info("stripe: pending subscription update expired", "customer_id", sub.Customer.ID)
		}

	case "customer.subscription.trial_will_end":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			slog.Error("stripe: decode customer.subscription.trial_will_end", "error", err)
			break
		}
		if sub.Customer != nil {
			slog.Info("stripe: trial ending soon", "customer_id", sub.Customer.ID)
		}

	case "payment_intent.succeeded":
		slog.Info("stripe: payment intent succeeded", "event_id", event.ID)

	case "payment_intent.payment_failed":
		slog.Info("stripe: payment intent failed", "event_id", event.ID)

	case "payment_intent.canceled":
		slog.Info("stripe: payment intent canceled", "event_id", event.ID)

	default:
		slog.Warn("stripe: unhandled event type", "type", event.Type, "event_id", event.ID)
	}

	w.WriteHeader(http.StatusOK)
}

// updateOrgByCustomer updates an org's plan and subscription by Stripe customer ID.
func (h *BillingHandler) updateOrgByCustomer(ctx context.Context, w http.ResponseWriter, customerID, plan, subID string, expiry *time.Time) {
	tag, err := h.Store.Q().Exec(ctx,
		`UPDATE orgs SET stripe_subscription_id = $1, plan = $2, plan_expires_at = NULL
		 WHERE stripe_customer_id = $3`,
		subID, plan, customerID,
	)
	if err != nil {
		slog.Error("stripe: update org", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		slog.Warn("stripe: no org found for customer", "customer_id", customerID)
	} else {
		h.publishPlanChanged(ctx, customerID, plan)
	}
}

// updateOrgByCustomerCancelled handles subscription cancellation with grace period.
func (h *BillingHandler) updateOrgByCustomerCancelled(ctx context.Context, w http.ResponseWriter, customerID string, expiry *time.Time) {
	tag, err := h.Store.Q().Exec(ctx,
		`UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1, stripe_subscription_id = NULL
		 WHERE stripe_customer_id = $2`,
		expiry, customerID,
	)
	if err != nil {
		slog.Error("stripe: update org after subscription deleted", "error", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if tag.RowsAffected() == 0 {
		slog.Warn("stripe: no org found for customer", "customer_id", customerID)
	} else {
		h.publishPlanChanged(ctx, customerID, "cancelled")
	}
}

func (h *BillingHandler) publishPlanChanged(ctx context.Context, customerID, plan string) {
	if h.Bus == nil {
		return
	}
	var orgID string
	warnIfErr(h.Store.Q().QueryRow(ctx, `SELECT id FROM orgs WHERE stripe_customer_id = $1`, customerID).Scan(&orgID),
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
