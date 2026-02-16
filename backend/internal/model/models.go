package model

import (
	"encoding/json"
	"time"
)

type Org struct {
	ID                     string    `json:"id"`
	Name                   string    `json:"name"`
	ResendAPIKeyEncrypted  *string   `json:"-"`
	ResendAPIKeyIV         *string   `json:"-"`
	ResendAPIKeyTag        *string   `json:"-"`
	ResendWebhookID        *string   `json:"-"`
	ResendWebhookSecret    *string   `json:"-"`
	OnboardingCompleted    bool      `json:"onboarding_completed"`
	CreatedAt              time.Time `json:"created_at"`
	UpdatedAt              time.Time `json:"updated_at"`
}

type User struct {
	ID                      string          `json:"id"`
	OrgID                   string          `json:"org_id"`
	Email                   string          `json:"email"`
	Name                    string          `json:"name"`
	PasswordHash            *string         `json:"-"`
	Role                    string          `json:"role"`
	Status                  string          `json:"status"`
	InviteToken             *string         `json:"-"`
	InviteExpiresAt         *time.Time      `json:"-"`
	ResetToken              *string         `json:"-"`
	ResetExpiresAt          *time.Time      `json:"-"`
	NotificationPreferences json.RawMessage `json:"notification_preferences"`
	CreatedAt               time.Time       `json:"created_at"`
	UpdatedAt               time.Time       `json:"updated_at"`
}

type Domain struct {
	ID              string          `json:"id"`
	OrgID           string          `json:"org_id"`
	Domain          string          `json:"domain"`
	ResendDomainID  *string         `json:"resend_domain_id,omitempty"`
	Status          string          `json:"status"`
	MXVerified      bool            `json:"mx_verified"`
	SPFVerified     bool            `json:"spf_verified"`
	DKIMVerified    bool            `json:"dkim_verified"`
	CatchAllEnabled bool            `json:"catch_all_enabled"`
	DisplayOrder    int             `json:"display_order"`
	DNSRecords      json.RawMessage `json:"dns_records,omitempty"`
	VerifiedAt      *time.Time      `json:"verified_at,omitempty"`
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

type Thread struct {
	ID                string          `json:"id"`
	OrgID             string          `json:"org_id"`
	UserID            string          `json:"user_id"`
	DomainID          string          `json:"domain_id"`
	Subject           string          `json:"subject"`
	ParticipantEmails json.RawMessage `json:"participant_emails"`
	LastMessageAt     time.Time       `json:"last_message_at"`
	MessageCount      int             `json:"message_count"`
	UnreadCount       int             `json:"unread_count"`
	Starred           bool            `json:"starred"`
	Folder            string          `json:"folder"`
	OriginalTo        *string         `json:"original_to,omitempty"`
	TrashExpiresAt    *time.Time      `json:"trash_expires_at,omitempty"`
	DeletedAt         *time.Time      `json:"deleted_at,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
	Emails            []Email         `json:"emails,omitempty"`
}

type Email struct {
	ID               string          `json:"id"`
	ThreadID         string          `json:"thread_id"`
	UserID           string          `json:"user_id"`
	OrgID            string          `json:"org_id"`
	DomainID         string          `json:"domain_id"`
	ResendEmailID    *string         `json:"resend_email_id,omitempty"`
	MessageID        *string         `json:"message_id,omitempty"`
	Direction        string          `json:"direction"`
	FromAddress      string          `json:"from_address"`
	ToAddresses      json.RawMessage `json:"to_addresses"`
	CCAddresses      json.RawMessage `json:"cc_addresses"`
	BCCAddresses     json.RawMessage `json:"bcc_addresses"`
	Subject          string          `json:"subject"`
	BodyHTML         *string         `json:"body_html,omitempty"`
	BodyPlain        *string         `json:"body_plain,omitempty"`
	Status           string          `json:"status"`
	Attachments      json.RawMessage `json:"attachments,omitempty"`
	InReplyTo        *string         `json:"in_reply_to,omitempty"`
	ReferencesHeader json.RawMessage `json:"references_header,omitempty"`
	DeliveredViaAlias *string        `json:"delivered_via_alias,omitempty"`
	SentAsAlias      *string         `json:"sent_as_alias,omitempty"`
	SpamScore        *float64        `json:"spam_score,omitempty"`
	SpamReasons      json.RawMessage `json:"spam_reasons,omitempty"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

type Alias struct {
	ID        string    `json:"id"`
	OrgID     string    `json:"org_id"`
	DomainID  string    `json:"domain_id"`
	Address   string    `json:"address"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type AliasUser struct {
	ID        string    `json:"id"`
	AliasID   string    `json:"alias_id"`
	UserID    string    `json:"user_id"`
	CanSendAs bool      `json:"can_send_as"`
	CreatedAt time.Time `json:"created_at"`
}

type DiscoveredAddress struct {
	ID         string    `json:"id"`
	DomainID   string    `json:"domain_id"`
	Address    string    `json:"address"`
	LocalPart  string    `json:"local_part"`
	Type       string    `json:"type"`
	UserID     *string   `json:"user_id,omitempty"`
	AliasID    *string   `json:"alias_id,omitempty"`
	EmailCount int       `json:"email_count"`
	CreatedAt  time.Time `json:"created_at"`
}
