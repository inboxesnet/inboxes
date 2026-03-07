package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Querier is implemented by both *pgxpool.Pool and pgx.Tx.
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

// TxRunner abstracts transaction support.
type TxRunner interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// Store combines all domain-specific store interfaces.
type Store interface {
	AuthStore
	ThreadStore
	EmailStore
	DomainStore
	UserStore
	AliasStore
	LabelStore
	DraftStore
	OrgStore
	ContactStore
	WebhookStore
	BillingStore
	OnboardingStore
	SetupStore
	EventStore
	SyncStore
	CronStore
	WorkerStore
	PollerStore

	// Pool exposes the underlying pool for cases that need direct access (e.g., middleware).
	Pool() *pgxpool.Pool
	// Q returns the querier (pool or tx).
	Q() Querier
	// WithTx runs fn within a transaction. If fn returns an error, the transaction is rolled back.
	WithTx(ctx context.Context, fn func(Store) error) error
	// WithTxOpts runs fn within a transaction with custom options.
	WithTxOpts(ctx context.Context, opts pgx.TxOptions, fn func(Store) error) error
}

// ---- Auth ----

type AuthStore interface {
	CountUsers(ctx context.Context) (int, error)
	CreateOrgAndAdmin(ctx context.Context, orgName, email, name, passwordHash string, emailVerified bool, isOwner bool) (orgID, userID string, err error)
	SetVerificationCode(ctx context.Context, userID, code string, expires time.Time) error
	GetUserByEmail(ctx context.Context, email string) (id, orgID, name, role, status, passwordHash string, emailVerified bool, err error)
	GetOnboardingCompleted(ctx context.Context, orgID string) (bool, error)
	SetResetToken(ctx context.Context, email, token string, expires time.Time) (rowsAffected int64, err error)
	ResetPassword(ctx context.Context, passwordHash, token string) (userID string, err error)
	ClaimInvite(ctx context.Context, passwordHash, name, token string) (userID, orgID, email, role string, err error)
	VerifyEmail(ctx context.Context, email, code string) (userID, orgID, name, role string, err error)
	ResendVerificationCode(ctx context.Context, email, code string, expires time.Time) (int64, error)
	ValidateInviteToken(ctx context.Context, token string) (email, name, status string, err error)
}

// ---- Threads ----

type ThreadStore interface {
	GetUserAliasAddresses(ctx context.Context, userID string) ([]string, error)
	BatchFetchLabels(ctx context.Context, threadIDs []string) (map[string][]string, error)
	ListThreads(ctx context.Context, orgID, label, domainID string, role string, aliasAddrs []string, page, limit int) (threads []map[string]any, total int, err error)
	GetThread(ctx context.Context, threadID, orgID string) (map[string]any, error)
	GetThreadEmails(ctx context.Context, threadID, orgID string) ([]map[string]any, error)
	CheckThreadVisibility(ctx context.Context, threadID string, aliasLabels []string) (bool, error)
	GetThreadDomainID(ctx context.Context, threadID, orgID string) (string, error)
	FetchThreadSummary(ctx context.Context, threadID, orgID string) (map[string]any, error)
	UpdateThreadUnread(ctx context.Context, threadID, orgID string, unreadCount int) (int64, error)
	MarkAllEmailsRead(ctx context.Context, threadID, orgID string) error
	MarkLatestEmailUnread(ctx context.Context, threadID, orgID string) error
	SoftDeleteThread(ctx context.Context, threadID, orgID string) (int64, error)
	SetTrashExpiry(ctx context.Context, threadIDs []string, orgID string) error
	BulkUpdateUnread(ctx context.Context, threadIDs []string, orgID string, unreadCount int) (int64, error)
	FilterTrashThreadIDs(ctx context.Context, threadIDs []string) ([]string, error)
	BulkSoftDelete(ctx context.Context, threadIDs []string, orgID string) (int64, error)
	ResolveFilteredThreadIDs(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string) ([]string, error)
	CreateThread(ctx context.Context, orgID, userID, domainID, subject string, participantsJSON []byte, snippet, lastSender string) (string, error)

	// Label operations (work on both pool and tx)
	AddLabel(ctx context.Context, threadID, orgID, label string) error
	RemoveLabel(ctx context.Context, threadID, label string) error
	RemoveAllLabels(ctx context.Context, threadID string) error
	HasLabel(ctx context.Context, threadID, label string) bool
	GetLabels(ctx context.Context, threadID string) []string
	BulkAddLabel(ctx context.Context, threadIDs []string, orgID, label string) error
	BulkRemoveLabel(ctx context.Context, threadIDs []string, label string) error
}

// ---- Emails ----

type EmailStore interface {
	LoadAttachmentsForResend(ctx context.Context, ids []string, orgID string) ([]map[string]string, error)
	CheckBouncedRecipients(ctx context.Context, orgID string, addresses []string) ([]string, error)
	CanSendAs(ctx context.Context, userID, orgID, fromAddress, role string) (bool, error)
	GetDomainStatus(ctx context.Context, domainID, orgID string) (string, error)
	ResolveFromDisplay(ctx context.Context, orgID, address string) (string, error)
	LookupDomainByName(ctx context.Context, orgID, domainName string) (string, error)
	InsertEmail(ctx context.Context, threadID, userID, orgID, domainID, direction, from string, toJSON, ccJSON, bccJSON []byte, subject, bodyHTML, bodyPlain, status string, inReplyTo string, refsJSON []byte) (string, error)
	UpdateThreadStats(ctx context.Context, threadID, snippet, lastSender string) error
	CreateEmailJob(ctx context.Context, orgID, userID, domainID, jobType, emailID, threadID string, resendPayload []byte, draftID *string) (string, error)
	SearchEmails(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error)
	ListAdminJobs(ctx context.Context, orgID string) ([]map[string]any, error)
	CheckSendJobExists(ctx context.Context, draftID string) (bool, error)
}

// ---- Domains ----

type DomainStore interface {
	ListDomains(ctx context.Context, orgID string, includeHidden bool) ([]map[string]any, error)
	InsertDomain(ctx context.Context, orgID, domain, resendDomainID, status string, dnsRecords json.RawMessage) (string, error)
	GetResendDomainID(ctx context.Context, domainID, orgID string) (string, error)
	UpdateDomainStatus(ctx context.Context, domainID, status string, dnsRecords json.RawMessage) error
	ReorderDomains(ctx context.Context, orgID string, order []DomainOrder) error
	GetUnreadCounts(ctx context.Context, orgID string, role string, aliasAddrs []string) (map[string]int, error)
	UpdateDomainVisibility(ctx context.Context, orgID string, visibleIDs []string) error
	SyncDomains(ctx context.Context, orgID string, resendDomains []ResendDomainInfo) error
	SoftDeleteDomain(ctx context.Context, domainID, orgID string) (int64, error)
	CascadeDeleteDomain(ctx context.Context, domainID string) error
	UpdateWebhookConfig(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error
	ListDiscoveredDomains(ctx context.Context, orgID string) ([]map[string]any, error)
	DismissDiscoveredDomain(ctx context.Context, id, orgID string) error
}

// DomainOrder represents a domain reorder request item.
type DomainOrder struct {
	ID    string
	Order int
}

// ResendDomainInfo represents domain data from the Resend API.
type ResendDomainInfo struct {
	ID     string
	Name   string
	Status string
}

// ---- Users ----

type UserStore interface {
	ListUsers(ctx context.Context, orgID string) ([]map[string]any, error)
	InsertInvitedUser(ctx context.Context, orgID, email, name, role, token string, expiresAt time.Time) (string, error)
	GetOrgName(ctx context.Context, orgID string) (string, error)
	GetUserName(ctx context.Context, userID string) (string, error)
	ReinviteUser(ctx context.Context, userID, orgID, token string, expiresAt time.Time) (email string, err error)
	GetUserRole(ctx context.Context, userID, orgID string) (string, error)
	CountActiveAdmins(ctx context.Context, orgID string) (int, error)
	DisableUser(ctx context.Context, userID, orgID string) (int64, error)
	DeleteAliasUsers(ctx context.Context, userID string) error
	ReassignAndDisable(ctx context.Context, orgID, adminID, sourceID, targetID string) (map[string]any, error)
	GetMe(ctx context.Context, userID string) (map[string]any, error)
	UpdateUserName(ctx context.Context, userID, name string) error
	GetPasswordHash(ctx context.Context, userID string) (string, error)
	UpdatePassword(ctx context.Context, userID, hash string) error
	GetPreferences(ctx context.Context, userID string) ([]byte, error)
	UpdatePreferences(ctx context.Context, userID string, prefs map[string]any) error
	ListMyAliases(ctx context.Context, userID, orgID string) ([]map[string]any, error)
	ChangeRole(ctx context.Context, userID, orgID, role string) (int64, error)
	GetUserOwnerAndRole(ctx context.Context, userID, orgID string) (isOwner bool, currentRole string, err error)
	EnableUser(ctx context.Context, userID, orgID string) (int64, error)
}

// ---- Aliases ----

type AliasStore interface {
	ListAliases(ctx context.Context, orgID, domainID string) ([]map[string]any, error)
	CreateAlias(ctx context.Context, orgID, domainID, address, name string) (string, error)
	UpdateAlias(ctx context.Context, aliasID, orgID, name string) (int64, error)
	DeleteAlias(ctx context.Context, aliasID, orgID string) (int64, error)
	AddAliasUser(ctx context.Context, aliasID, orgID, userID string, canSendAs bool) error
	RemoveAliasUser(ctx context.Context, aliasID, userID string) error
	SetDefaultAlias(ctx context.Context, aliasID, userID, orgID string) error
	ListDiscoveredAddresses(ctx context.Context, orgID string) ([]map[string]any, error)
	CheckAliasOrg(ctx context.Context, aliasID, orgID string) (int, error)
	CheckUserOrg(ctx context.Context, userID, orgID string) (bool, error)
}

// ---- Labels ----

type LabelStore interface {
	ListOrgLabels(ctx context.Context, orgID string) ([]map[string]any, error)
	CreateOrgLabel(ctx context.Context, orgID, name string) (string, error)
	RenameOrgLabel(ctx context.Context, labelID, orgID, newName string) (oldName string, err error)
	DeleteOrgLabel(ctx context.Context, labelID, orgID string) (labelName string, err error)
}

// ---- Drafts ----

type DraftStore interface {
	ListDrafts(ctx context.Context, userID, orgID, domainID string) ([]map[string]any, error)
	CreateDraft(ctx context.Context, orgID, userID, domainID string, threadID *string, kind, subject, fromAddress string, toJSON, ccJSON, bccJSON, attJSON []byte) (string, error)
	UpdateDraft(ctx context.Context, draftID, userID string, sets []string, args []any) (int64, error)
	DeleteDraft(ctx context.Context, draftID, userID string) (int64, error)
	GetDraft(ctx context.Context, draftID, userID, orgID string) (domainID string, threadID *string, kind, subject, fromAddr, bodyHTML, bodyPlain string, toAddr, ccAddr, bccAddr, attIDsRaw json.RawMessage, err error)
}

// ---- Orgs ----

type OrgStore interface {
	GetOrgSettings(ctx context.Context, orgID string) (map[string]any, error)
	UpdateOrgName(ctx context.Context, orgID, name string) error
	UpdateOrgAPIKey(ctx context.Context, orgID string, ciphertext, iv, tag string) error
	UpdateOrgRPS(ctx context.Context, orgID string, rps int) error
	UpdateOrgAutoPoll(ctx context.Context, orgID string, enabled bool) error
	UpdateOrgAutoPollInterval(ctx context.Context, orgID string, interval int) error
	IsOrgOwner(ctx context.Context, userID, orgID string) (bool, error)
	GetStripeSubscriptionID(ctx context.Context, orgID string) (*string, error)
	GetWebhookID(ctx context.Context, orgID string) (*string, error)
	SoftDeleteOrg(ctx context.Context, orgID string) error
	ListOrgUserIDs(ctx context.Context, orgID string) ([]string, error)
	CancelOrgJobs(ctx context.Context, orgID string) ([]string, error)
	GetOrgNameByID(ctx context.Context, orgID string) (string, error)
	HardDeleteOrg(ctx context.Context, orgID string) (int64, error)
}

// ---- Contacts ----

type ContactStore interface {
	SuggestContacts(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error)
}

// ---- Webhooks ----

type WebhookStore interface {
	GetOrgWebhookSecret(ctx context.Context, orgID string) (encSecret, encIV, encTag string, plainSecret *string, err error)
	CheckWebhookDedup(ctx context.Context, orgID, resendEmailID, eventType string) (bool, error)
	InsertWebhookDedup(ctx context.Context, orgID, resendEmailID, eventType string) error
	UpdateEmailStatus(ctx context.Context, orgID, resendEmailID, status string) (int64, error)
	GetEmailThreadByResendID(ctx context.Context, orgID, resendEmailID string) (emailID, threadID, domainID string, err error)
	InsertBounce(ctx context.Context, orgID, address, bounceType string) error
	ClearBounce(ctx context.Context, orgID, fromAddress string) error
}

// ---- Billing ----

type BillingStore interface {
	GetBillingInfo(ctx context.Context, orgID string) (map[string]any, error)
	GetStripeCustomerID(ctx context.Context, orgID string) (*string, error)
	SetStripeCustomerID(ctx context.Context, orgID, customerID string) error
	UpdateOrgPlan(ctx context.Context, orgID, plan, subID string) error
	SetPlanExpiry(ctx context.Context, orgID string, expiresAt time.Time) error
	ClearPlanExpiry(ctx context.Context, orgID string) error
	InsertStripeEvent(ctx context.Context, eventID string) (bool, error)
}

// ---- Onboarding ----

type OnboardingStore interface {
	HasAPIKey(ctx context.Context, orgID string) (bool, error)
	CountVisibleDomains(ctx context.Context, orgID string) (int, error)
	GetActiveSyncJob(ctx context.Context, orgID string) (jobID, phase string, err error)
	CountEmails(ctx context.Context, orgID string) (int, error)
	StoreEncryptedAPIKey(ctx context.Context, orgID string, ciphertext, iv, tag string) error
	UpsertDomain(ctx context.Context, orgID, domain, resendDomainID, status string, records json.RawMessage, order int) (string, error)
	SelectDomains(ctx context.Context, orgID string, domainIDs []string) error
	StoreWebhookConfig(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error
	GetDiscoveredAddresses(ctx context.Context, orgID string) ([]map[string]any, error)
	SetupAddress(ctx context.Context, orgID, userID, address, addrType, name string) error
	CompleteOnboarding(ctx context.Context, orgID string) error
	GetFirstDomainID(ctx context.Context, orgID string) (string, error)
}

// ---- Setup ----

type SetupStore interface {
	SetupCountUsers(ctx context.Context) (int, error)
	CreateAdminSetup(ctx context.Context, orgName, email, name, passwordHash, systemResendKey, systemFromAddress, systemFromName string, encSvc interface{ Encrypt(string) (string, string, string, error) }) (orgID, userID string, err error)
	UpsertSystemSetting(ctx context.Context, key, value string) error
	GetUserEmail(ctx context.Context, userID string) (string, error)
}

// ---- Events ----

type EventStore interface {
	GetEventsSince(ctx context.Context, orgID string, since time.Time) ([]map[string]any, error)
}

// ---- Sync ----

type SyncStore interface {
	CreateSyncJob(ctx context.Context, orgID, userID string) (string, error)
	GetSyncJob(ctx context.Context, jobID, orgID string) (map[string]any, error)
}

// ---- Cron ----

type CronStore interface {
	PurgeExpiredTrash(ctx context.Context) (int64, error)
	CleanupStaleWebhooks(ctx context.Context, orgIDs []string) error
}

// ---- Worker ----

type WorkerStore interface {
	// Email job operations
	GetEmailJob(ctx context.Context, jobID string) (map[string]any, error)
	UpdateEmailJobStatus(ctx context.Context, jobID, status, errorMsg string) error
	UpdateEmailJobHeartbeat(ctx context.Context, jobID string) error
	IncrementJobAttempts(ctx context.Context, jobID string) error
	GetStaleJobs(ctx context.Context, timeout time.Duration) ([]string, error)
	GetOrphanedJobs(ctx context.Context, age time.Duration) ([]string, error)
}

// ---- Poller ----

type PollerStore interface {
	GetPollableOrgs(ctx context.Context) ([]map[string]any, error)
	HasPendingSyncJob(ctx context.Context, orgID string) (bool, error)
	EmailExistsByResendID(ctx context.Context, orgID, resendEmailID string) (bool, error)
	CreateFetchJob(ctx context.Context, orgID, resendEmailID, jobType string) (string, error)
}
