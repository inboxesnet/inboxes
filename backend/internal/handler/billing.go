package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

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
		h.DB.QueryRow(ctx,
			"SELECT email FROM users WHERE id = $1", claims.UserID,
		).Scan(&email)

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
		h.DB.Exec(ctx,
			"UPDATE orgs SET stripe_customer_id = $1 WHERE id = $2",
			custID, claims.OrgID,
		)
	}

	// Get first domain ID for success redirect
	var firstDomainID string
	h.DB.QueryRow(ctx,
		"SELECT id FROM domains WHERE org_id = $1 AND hidden = false ORDER BY display_order LIMIT 1",
		claims.OrgID,
	).Scan(&firstDomainID)

	successURL := h.AppURL
	if firstDomainID != "" {
		successURL += "/d/" + firstDomainID + "/inbox?billing=success"
	} else {
		successURL += "/onboarding"
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

	params := &stripe.BillingPortalSessionParams{
		Customer:  stripe.String(*stripeCustomerID),
		ReturnURL: stripe.String(h.AppURL),
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

	resp := map[string]interface{}{
		"plan":            plan,
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

	switch event.Type {
	case "checkout.session.completed":
		var session stripe.CheckoutSession
		if err := decodeStripeObject(event.Data.Raw, &session); err != nil {
			break
		}
		if session.Customer != nil && session.Subscription != nil {
			h.DB.Exec(ctx,
				`UPDATE orgs SET stripe_subscription_id = $1, plan = 'pro', plan_expires_at = NULL
				 WHERE stripe_customer_id = $2`,
				session.Subscription.ID, session.Customer.ID,
			)
		}

	case "customer.subscription.deleted":
		var sub stripe.Subscription
		if err := decodeStripeObject(event.Data.Raw, &sub); err != nil {
			break
		}
		if sub.Customer != nil {
			h.DB.Exec(ctx,
				`UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1, stripe_subscription_id = NULL
				 WHERE stripe_customer_id = $2`,
				time.Now().Add(7*24*time.Hour), sub.Customer.ID,
			)
		}

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := decodeStripeObject(event.Data.Raw, &inv); err != nil {
			break
		}
		if inv.Customer != nil {
			h.DB.Exec(ctx,
				`UPDATE orgs SET plan = 'cancelled', plan_expires_at = $1
				 WHERE stripe_customer_id = $2`,
				time.Now().Add(3*24*time.Hour), inv.Customer.ID,
			)
		}
	}

	w.WriteHeader(http.StatusOK)
}

// decodeStripeObject parses raw JSON from a Stripe event into a typed struct.
func decodeStripeObject(raw []byte, dst interface{}) error {
	return json.Unmarshal(raw, dst)
}
