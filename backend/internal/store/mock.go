package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Compile-time assertion: MockStore must implement Store.
var _ Store = (*MockStore)(nil)

// MockStore implements Store for testing. Set function fields per test.
// Each method delegates to its Fn field if non-nil, otherwise returns safe zero values.
type MockStore struct {
	// ---- Pool / Q / Tx ----
	PoolFn       func() *pgxpool.Pool
	QFn          func() Querier
	WithTxFn     func(ctx context.Context, fn func(Store) error) error
	WithTxOptsFn func(ctx context.Context, opts pgx.TxOptions, fn func(Store) error) error

	// ---- Auth ----
	CountUsersFn             func(ctx context.Context) (int, error)
	CreateOrgAndAdminFn      func(ctx context.Context, orgName, email, name, passwordHash string, emailVerified bool, isOwner bool) (string, string, error)
	SetVerificationCodeFn    func(ctx context.Context, userID, code string, expires time.Time) error
	GetUserByEmailFn         func(ctx context.Context, email string) (id, orgID, name, role, status, passwordHash string, emailVerified bool, err error)
	GetOnboardingCompletedFn func(ctx context.Context, orgID string) (bool, error)
	SetResetTokenFn          func(ctx context.Context, email, token string, expires time.Time) (int64, error)
	ResetPasswordFn          func(ctx context.Context, passwordHash, token string) (string, error)
	ClaimInviteFn            func(ctx context.Context, passwordHash, name, token string) (string, string, string, string, error)
	VerifyEmailFn            func(ctx context.Context, email, code string) (string, string, string, string, error)
	ResendVerificationCodeFn func(ctx context.Context, email, code string, expires time.Time) (int64, error)
	ValidateInviteTokenFn    func(ctx context.Context, token string) (string, string, string, error)

	// ---- Threads ----
	GetUserAliasAddressesFn    func(ctx context.Context, userID string) ([]string, error)
	BatchFetchLabelsFn         func(ctx context.Context, threadIDs []string) (map[string][]string, error)
	ListThreadsFn              func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error)
	GetThreadFn                func(ctx context.Context, threadID, orgID string) (map[string]any, error)
	GetThreadEmailsFn          func(ctx context.Context, threadID, orgID string) ([]map[string]any, error)
	CheckThreadVisibilityFn    func(ctx context.Context, threadID string, aliasLabels []string) (bool, error)
	GetThreadDomainIDFn        func(ctx context.Context, threadID, orgID string) (string, error)
	FetchThreadSummaryFn       func(ctx context.Context, threadID, orgID string) (map[string]any, error)
	UpdateThreadUnreadFn       func(ctx context.Context, threadID, orgID string, unreadCount int) (int64, error)
	MarkAllEmailsReadFn        func(ctx context.Context, threadID, orgID string) error
	MarkLatestEmailUnreadFn    func(ctx context.Context, threadID, orgID string) error
	SoftDeleteThreadFn         func(ctx context.Context, threadID, orgID string) (int64, error)
	SetTrashExpiryFn           func(ctx context.Context, threadIDs []string, orgID string) error
	BulkUpdateUnreadFn         func(ctx context.Context, threadIDs []string, orgID string, unreadCount int) (int64, error)
	FilterTrashThreadIDsFn     func(ctx context.Context, threadIDs []string) ([]string, error)
	BulkSoftDeleteFn           func(ctx context.Context, threadIDs []string, orgID string) (int64, error)
	ResolveFilteredThreadIDsFn func(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string) ([]string, error)
	CreateThreadFn             func(ctx context.Context, orgID, userID, domainID, subject string, participantsJSON []byte, snippet, lastSender string) (string, error)
	AddLabelFn                 func(ctx context.Context, threadID, orgID, label string) error
	RemoveLabelFn              func(ctx context.Context, threadID, label string) error
	RemoveAllLabelsFn          func(ctx context.Context, threadID string) error
	HasLabelFn                 func(ctx context.Context, threadID, label string) bool
	GetLabelsFn                func(ctx context.Context, threadID string) []string
	BulkAddLabelFn             func(ctx context.Context, threadIDs []string, orgID, label string) error
	BulkRemoveLabelFn          func(ctx context.Context, threadIDs []string, label string) error

	// ---- Emails ----
	LoadAttachmentsForResendFn func(ctx context.Context, ids []string, orgID string) ([]map[string]string, error)
	CheckBouncedRecipientsFn   func(ctx context.Context, orgID string, addresses []string) ([]string, error)
	CanSendAsFn                func(ctx context.Context, userID, orgID, fromAddress, role string) (bool, error)
	GetDomainStatusFn          func(ctx context.Context, domainID, orgID string) (string, error)
	ResolveFromDisplayFn       func(ctx context.Context, orgID, address string) (string, error)
	LookupDomainByNameFn       func(ctx context.Context, orgID, domainName string) (string, error)
	InsertEmailFn              func(ctx context.Context, threadID, userID, orgID, domainID, direction, from string, toJSON, ccJSON, bccJSON []byte, subject, bodyHTML, bodyPlain, status string, inReplyTo string, refsJSON []byte) (string, error)
	UpdateThreadStatsFn        func(ctx context.Context, threadID, snippet, lastSender string) error
	CreateEmailJobFn           func(ctx context.Context, orgID, userID, domainID, jobType, emailID, threadID string, resendPayload []byte, draftID *string) (string, error)
	SearchEmailsFn             func(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error)
	ListAdminJobsFn            func(ctx context.Context, orgID string) ([]map[string]any, error)
	CheckSendJobExistsFn       func(ctx context.Context, draftID string) (bool, error)

	// ---- Domains ----
	ListDomainsFn            func(ctx context.Context, orgID string, includeHidden bool) ([]map[string]any, error)
	InsertDomainFn           func(ctx context.Context, orgID, domain, resendDomainID, status string, dnsRecords json.RawMessage) (string, error)
	GetResendDomainIDFn      func(ctx context.Context, domainID, orgID string) (string, error)
	UpdateDomainStatusFn     func(ctx context.Context, domainID, status string, dnsRecords json.RawMessage) error
	ReorderDomainsFn         func(ctx context.Context, orgID string, order []DomainOrder) error
	GetUnreadCountsFn        func(ctx context.Context, orgID string, role string, aliasAddrs []string) (map[string]int, error)
	UpdateDomainVisibilityFn func(ctx context.Context, orgID string, visibleIDs []string) error
	SyncDomainsFn            func(ctx context.Context, orgID string, resendDomains []ResendDomainInfo) error
	SoftDeleteDomainFn       func(ctx context.Context, domainID, orgID string) (int64, error)
	CascadeDeleteDomainFn    func(ctx context.Context, domainID string) error
	UpdateWebhookConfigFn    func(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error

	// ---- Users ----
	ListUsersFn           func(ctx context.Context, orgID string) ([]map[string]any, error)
	InsertInvitedUserFn   func(ctx context.Context, orgID, email, name, role, token string, expiresAt time.Time) (string, error)
	GetOrgNameFn          func(ctx context.Context, orgID string) (string, error)
	GetUserNameFn         func(ctx context.Context, userID string) (string, error)
	ReinviteUserFn        func(ctx context.Context, userID, orgID, token string, expiresAt time.Time) (string, error)
	GetUserRoleFn         func(ctx context.Context, userID, orgID string) (string, error)
	CountActiveAdminsFn   func(ctx context.Context, orgID string) (int, error)
	DisableUserFn         func(ctx context.Context, userID, orgID string) (int64, error)
	DeleteAliasUsersFn    func(ctx context.Context, userID string) error
	ReassignAndDisableFn  func(ctx context.Context, orgID, adminID, sourceID, targetID string) (map[string]any, error)
	GetMeFn               func(ctx context.Context, userID string) (map[string]any, error)
	UpdateUserNameFn      func(ctx context.Context, userID, name string) error
	GetPasswordHashFn     func(ctx context.Context, userID string) (string, error)
	UpdatePasswordFn      func(ctx context.Context, userID, hash string) error
	GetPreferencesFn      func(ctx context.Context, userID string) ([]byte, error)
	UpdatePreferencesFn   func(ctx context.Context, userID string, prefs map[string]any) error
	ListMyAliasesFn       func(ctx context.Context, userID, orgID string) ([]map[string]any, error)
	ChangeRoleFn          func(ctx context.Context, userID, orgID, role string) (int64, error)
	GetUserOwnerAndRoleFn func(ctx context.Context, userID, orgID string) (bool, string, error)
	EnableUserFn          func(ctx context.Context, userID, orgID string) (int64, error)

	// ---- Aliases ----
	ListAliasesFn             func(ctx context.Context, orgID, domainID string) ([]map[string]any, error)
	CreateAliasFn             func(ctx context.Context, orgID, domainID, address, name string) (string, error)
	UpdateAliasFn             func(ctx context.Context, aliasID, orgID, name string) (int64, error)
	DeleteAliasFn             func(ctx context.Context, aliasID, orgID string) (int64, error)
	AddAliasUserFn            func(ctx context.Context, aliasID, orgID, userID string, canSendAs bool) error
	RemoveAliasUserFn         func(ctx context.Context, aliasID, userID string) error
	SetDefaultAliasFn         func(ctx context.Context, aliasID, userID, orgID string) error
	ListDiscoveredAddressesFn func(ctx context.Context, orgID string) ([]map[string]any, error)
	CheckAliasOrgFn           func(ctx context.Context, aliasID, orgID string) (int, error)
	CheckUserOrgFn            func(ctx context.Context, userID, orgID string) (bool, error)

	// ---- Labels ----
	ListOrgLabelsFn  func(ctx context.Context, orgID string) ([]map[string]any, error)
	CreateOrgLabelFn func(ctx context.Context, orgID, name string) (string, error)
	RenameOrgLabelFn func(ctx context.Context, labelID, orgID, newName string) (string, error)
	DeleteOrgLabelFn func(ctx context.Context, labelID, orgID string) (string, error)

	// ---- Drafts ----
	ListDraftsFn  func(ctx context.Context, userID, orgID, domainID string) ([]map[string]any, error)
	CreateDraftFn func(ctx context.Context, orgID, userID, domainID string, threadID *string, kind, subject, fromAddress string, toJSON, ccJSON, bccJSON, attJSON []byte) (string, error)
	UpdateDraftFn func(ctx context.Context, draftID, userID string, sets []string, args []any) (int64, error)
	DeleteDraftFn func(ctx context.Context, draftID, userID string) (int64, error)
	GetDraftFn    func(ctx context.Context, draftID, userID, orgID string) (string, *string, string, string, string, string, string, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, error)

	// ---- Orgs ----
	GetOrgSettingsFn            func(ctx context.Context, orgID string) (map[string]any, error)
	UpdateOrgNameFn             func(ctx context.Context, orgID, name string) error
	UpdateOrgAPIKeyFn           func(ctx context.Context, orgID string, ciphertext, iv, tag string) error
	UpdateOrgRPSFn              func(ctx context.Context, orgID string, rps int) error
	UpdateOrgAutoPollFn         func(ctx context.Context, orgID string, enabled bool) error
	UpdateOrgAutoPollIntervalFn func(ctx context.Context, orgID string, interval int) error
	IsOrgOwnerFn                func(ctx context.Context, userID, orgID string) (bool, error)
	GetStripeSubscriptionIDFn   func(ctx context.Context, orgID string) (*string, error)
	GetWebhookIDFn              func(ctx context.Context, orgID string) (*string, error)
	SoftDeleteOrgFn             func(ctx context.Context, orgID string) error
	ListOrgUserIDsFn            func(ctx context.Context, orgID string) ([]string, error)
	CancelOrgJobsFn             func(ctx context.Context, orgID string) ([]string, error)
	GetOrgNameByIDFn            func(ctx context.Context, orgID string) (string, error)
	HardDeleteOrgFn             func(ctx context.Context, orgID string) (int64, error)

	// ---- Contacts ----
	SuggestContactsFn func(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error)

	// ---- Webhooks ----
	GetOrgWebhookSecretFn      func(ctx context.Context, orgID string) (string, string, string, *string, error)
	CheckWebhookDedupFn        func(ctx context.Context, orgID, resendEmailID, eventType string) (bool, error)
	InsertWebhookDedupFn       func(ctx context.Context, orgID, resendEmailID, eventType string) error
	UpdateEmailStatusFn        func(ctx context.Context, orgID, resendEmailID, status string) (int64, error)
	GetEmailThreadByResendIDFn func(ctx context.Context, orgID, resendEmailID string) (string, string, string, error)
	InsertBounceFn             func(ctx context.Context, orgID, address, bounceType string) error
	ClearBounceFn              func(ctx context.Context, orgID, fromAddress string) error

	// ---- Billing ----
	GetBillingInfoFn      func(ctx context.Context, orgID string) (map[string]any, error)
	GetStripeCustomerIDFn func(ctx context.Context, orgID string) (*string, error)
	SetStripeCustomerIDFn func(ctx context.Context, orgID, customerID string) error
	UpdateOrgPlanFn       func(ctx context.Context, orgID, plan, subID string) error
	SetPlanExpiryFn       func(ctx context.Context, orgID string, expiresAt time.Time) error
	ClearPlanExpiryFn     func(ctx context.Context, orgID string) error
	InsertStripeEventFn   func(ctx context.Context, eventID string) (bool, error)

	// ---- Onboarding ----
	HasAPIKeyFn              func(ctx context.Context, orgID string) (bool, error)
	CountVisibleDomainsFn    func(ctx context.Context, orgID string) (int, error)
	GetActiveSyncJobFn       func(ctx context.Context, orgID string) (string, string, error)
	CountEmailsFn            func(ctx context.Context, orgID string) (int, error)
	StoreEncryptedAPIKeyFn   func(ctx context.Context, orgID string, ciphertext, iv, tag string) error
	UpsertDomainFn           func(ctx context.Context, orgID, domain, resendDomainID, status string, records json.RawMessage, order int) (string, error)
	SelectDomainsFn          func(ctx context.Context, orgID string, domainIDs []string) error
	StoreWebhookConfigFn     func(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error
	GetDiscoveredAddressesFn func(ctx context.Context, orgID string) ([]map[string]any, error)
	SetupAddressFn           func(ctx context.Context, orgID, userID, address, addrType, name string) error
	CompleteOnboardingFn     func(ctx context.Context, orgID string) error
	GetFirstDomainIDFn       func(ctx context.Context, orgID string) (string, error)

	// ---- Setup ----
	SetupCountUsersFn     func(ctx context.Context) (int, error)
	CreateAdminSetupFn    func(ctx context.Context, orgName, email, name, passwordHash, systemResendKey, systemFromAddress, systemFromName string, encSvc interface{ Encrypt(string) (string, string, string, error) }) (string, string, error)
	UpsertSystemSettingFn func(ctx context.Context, key, value string) error
	GetUserEmailFn        func(ctx context.Context, userID string) (string, error)

	// ---- Events ----
	GetEventsSinceFn func(ctx context.Context, orgID string, since time.Time) ([]map[string]any, error)

	// ---- Sync ----
	CreateSyncJobFn func(ctx context.Context, orgID, userID string) (string, error)
	GetSyncJobFn    func(ctx context.Context, jobID, orgID string) (map[string]any, error)

	// ---- Cron ----
	PurgeExpiredTrashFn    func(ctx context.Context) (int64, error)
	CleanupStaleWebhooksFn func(ctx context.Context, orgIDs []string) error

	// ---- Worker ----
	GetEmailJobFn             func(ctx context.Context, jobID string) (map[string]any, error)
	UpdateEmailJobStatusFn    func(ctx context.Context, jobID, status, errorMsg string) error
	UpdateEmailJobHeartbeatFn func(ctx context.Context, jobID string) error
	IncrementJobAttemptsFn    func(ctx context.Context, jobID string) error
	GetStaleJobsFn            func(ctx context.Context, timeout time.Duration) ([]string, error)
	GetOrphanedJobsFn         func(ctx context.Context, age time.Duration) ([]string, error)

	// ---- Poller ----
	GetPollableOrgsFn       func(ctx context.Context) ([]map[string]any, error)
	HasPendingSyncJobFn     func(ctx context.Context, orgID string) (bool, error)
	EmailExistsByResendIDFn func(ctx context.Context, orgID, resendEmailID string) (bool, error)
	CreateFetchJobFn        func(ctx context.Context, orgID, resendEmailID, jobType string) (string, error)
}

// ===========================================================================
// Pool / Q / Tx
// ===========================================================================

func (m *MockStore) Pool() *pgxpool.Pool {
	if m.PoolFn != nil {
		return m.PoolFn()
	}
	return nil
}

func (m *MockStore) Q() Querier {
	if m.QFn != nil {
		return m.QFn()
	}
	return nil
}

func (m *MockStore) WithTx(ctx context.Context, fn func(Store) error) error {
	if m.WithTxFn != nil {
		return m.WithTxFn(ctx, fn)
	}
	return fn(m)
}

func (m *MockStore) WithTxOpts(ctx context.Context, opts pgx.TxOptions, fn func(Store) error) error {
	if m.WithTxOptsFn != nil {
		return m.WithTxOptsFn(ctx, opts, fn)
	}
	return fn(m)
}

// ===========================================================================
// AuthStore
// ===========================================================================

func (m *MockStore) CountUsers(ctx context.Context) (int, error) {
	if m.CountUsersFn != nil {
		return m.CountUsersFn(ctx)
	}
	return 0, nil
}

func (m *MockStore) CreateOrgAndAdmin(ctx context.Context, orgName, email, name, passwordHash string, emailVerified bool, isOwner bool) (string, string, error) {
	if m.CreateOrgAndAdminFn != nil {
		return m.CreateOrgAndAdminFn(ctx, orgName, email, name, passwordHash, emailVerified, isOwner)
	}
	return "", "", nil
}

func (m *MockStore) SetVerificationCode(ctx context.Context, userID, code string, expires time.Time) error {
	if m.SetVerificationCodeFn != nil {
		return m.SetVerificationCodeFn(ctx, userID, code, expires)
	}
	return nil
}

func (m *MockStore) GetUserByEmail(ctx context.Context, email string) (string, string, string, string, string, string, bool, error) {
	if m.GetUserByEmailFn != nil {
		return m.GetUserByEmailFn(ctx, email)
	}
	return "", "", "", "", "", "", false, nil
}

func (m *MockStore) GetOnboardingCompleted(ctx context.Context, orgID string) (bool, error) {
	if m.GetOnboardingCompletedFn != nil {
		return m.GetOnboardingCompletedFn(ctx, orgID)
	}
	return false, nil
}

func (m *MockStore) SetResetToken(ctx context.Context, email, token string, expires time.Time) (int64, error) {
	if m.SetResetTokenFn != nil {
		return m.SetResetTokenFn(ctx, email, token, expires)
	}
	return 0, nil
}

func (m *MockStore) ResetPassword(ctx context.Context, passwordHash, token string) (string, error) {
	if m.ResetPasswordFn != nil {
		return m.ResetPasswordFn(ctx, passwordHash, token)
	}
	return "", nil
}

func (m *MockStore) ClaimInvite(ctx context.Context, passwordHash, name, token string) (string, string, string, string, error) {
	if m.ClaimInviteFn != nil {
		return m.ClaimInviteFn(ctx, passwordHash, name, token)
	}
	return "", "", "", "", nil
}

func (m *MockStore) VerifyEmail(ctx context.Context, email, code string) (string, string, string, string, error) {
	if m.VerifyEmailFn != nil {
		return m.VerifyEmailFn(ctx, email, code)
	}
	return "", "", "", "", nil
}

func (m *MockStore) ResendVerificationCode(ctx context.Context, email, code string, expires time.Time) (int64, error) {
	if m.ResendVerificationCodeFn != nil {
		return m.ResendVerificationCodeFn(ctx, email, code, expires)
	}
	return 0, nil
}

func (m *MockStore) ValidateInviteToken(ctx context.Context, token string) (string, string, string, error) {
	if m.ValidateInviteTokenFn != nil {
		return m.ValidateInviteTokenFn(ctx, token)
	}
	return "", "", "", nil
}

// ===========================================================================
// ThreadStore
// ===========================================================================

func (m *MockStore) GetUserAliasAddresses(ctx context.Context, userID string) ([]string, error) {
	if m.GetUserAliasAddressesFn != nil {
		return m.GetUserAliasAddressesFn(ctx, userID)
	}
	return []string{}, nil
}

func (m *MockStore) BatchFetchLabels(ctx context.Context, threadIDs []string) (map[string][]string, error) {
	if m.BatchFetchLabelsFn != nil {
		return m.BatchFetchLabelsFn(ctx, threadIDs)
	}
	return map[string][]string{}, nil
}

func (m *MockStore) ListThreads(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string, page, limit int) ([]map[string]any, int, error) {
	if m.ListThreadsFn != nil {
		return m.ListThreadsFn(ctx, orgID, label, domainID, role, aliasAddrs, page, limit)
	}
	return []map[string]any{}, 0, nil
}

func (m *MockStore) GetThread(ctx context.Context, threadID, orgID string) (map[string]any, error) {
	if m.GetThreadFn != nil {
		return m.GetThreadFn(ctx, threadID, orgID)
	}
	return map[string]any{}, nil
}

func (m *MockStore) GetThreadEmails(ctx context.Context, threadID, orgID string) ([]map[string]any, error) {
	if m.GetThreadEmailsFn != nil {
		return m.GetThreadEmailsFn(ctx, threadID, orgID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) CheckThreadVisibility(ctx context.Context, threadID string, aliasLabels []string) (bool, error) {
	if m.CheckThreadVisibilityFn != nil {
		return m.CheckThreadVisibilityFn(ctx, threadID, aliasLabels)
	}
	return false, nil
}

func (m *MockStore) GetThreadDomainID(ctx context.Context, threadID, orgID string) (string, error) {
	if m.GetThreadDomainIDFn != nil {
		return m.GetThreadDomainIDFn(ctx, threadID, orgID)
	}
	return "", nil
}

func (m *MockStore) FetchThreadSummary(ctx context.Context, threadID, orgID string) (map[string]any, error) {
	if m.FetchThreadSummaryFn != nil {
		return m.FetchThreadSummaryFn(ctx, threadID, orgID)
	}
	return map[string]any{}, nil
}

func (m *MockStore) UpdateThreadUnread(ctx context.Context, threadID, orgID string, unreadCount int) (int64, error) {
	if m.UpdateThreadUnreadFn != nil {
		return m.UpdateThreadUnreadFn(ctx, threadID, orgID, unreadCount)
	}
	return 0, nil
}

func (m *MockStore) MarkAllEmailsRead(ctx context.Context, threadID, orgID string) error {
	if m.MarkAllEmailsReadFn != nil {
		return m.MarkAllEmailsReadFn(ctx, threadID, orgID)
	}
	return nil
}

func (m *MockStore) MarkLatestEmailUnread(ctx context.Context, threadID, orgID string) error {
	if m.MarkLatestEmailUnreadFn != nil {
		return m.MarkLatestEmailUnreadFn(ctx, threadID, orgID)
	}
	return nil
}

func (m *MockStore) SoftDeleteThread(ctx context.Context, threadID, orgID string) (int64, error) {
	if m.SoftDeleteThreadFn != nil {
		return m.SoftDeleteThreadFn(ctx, threadID, orgID)
	}
	return 0, nil
}

func (m *MockStore) SetTrashExpiry(ctx context.Context, threadIDs []string, orgID string) error {
	if m.SetTrashExpiryFn != nil {
		return m.SetTrashExpiryFn(ctx, threadIDs, orgID)
	}
	return nil
}

func (m *MockStore) BulkUpdateUnread(ctx context.Context, threadIDs []string, orgID string, unreadCount int) (int64, error) {
	if m.BulkUpdateUnreadFn != nil {
		return m.BulkUpdateUnreadFn(ctx, threadIDs, orgID, unreadCount)
	}
	return 0, nil
}

func (m *MockStore) FilterTrashThreadIDs(ctx context.Context, threadIDs []string) ([]string, error) {
	if m.FilterTrashThreadIDsFn != nil {
		return m.FilterTrashThreadIDsFn(ctx, threadIDs)
	}
	return []string{}, nil
}

func (m *MockStore) BulkSoftDelete(ctx context.Context, threadIDs []string, orgID string) (int64, error) {
	if m.BulkSoftDeleteFn != nil {
		return m.BulkSoftDeleteFn(ctx, threadIDs, orgID)
	}
	return 0, nil
}

func (m *MockStore) ResolveFilteredThreadIDs(ctx context.Context, orgID, label, domainID, role string, aliasAddrs []string) ([]string, error) {
	if m.ResolveFilteredThreadIDsFn != nil {
		return m.ResolveFilteredThreadIDsFn(ctx, orgID, label, domainID, role, aliasAddrs)
	}
	return []string{}, nil
}

func (m *MockStore) CreateThread(ctx context.Context, orgID, userID, domainID, subject string, participantsJSON []byte, snippet, lastSender string) (string, error) {
	if m.CreateThreadFn != nil {
		return m.CreateThreadFn(ctx, orgID, userID, domainID, subject, participantsJSON, snippet, lastSender)
	}
	return "", nil
}

func (m *MockStore) AddLabel(ctx context.Context, threadID, orgID, label string) error {
	if m.AddLabelFn != nil {
		return m.AddLabelFn(ctx, threadID, orgID, label)
	}
	return nil
}

func (m *MockStore) RemoveLabel(ctx context.Context, threadID, label string) error {
	if m.RemoveLabelFn != nil {
		return m.RemoveLabelFn(ctx, threadID, label)
	}
	return nil
}

func (m *MockStore) RemoveAllLabels(ctx context.Context, threadID string) error {
	if m.RemoveAllLabelsFn != nil {
		return m.RemoveAllLabelsFn(ctx, threadID)
	}
	return nil
}

func (m *MockStore) HasLabel(ctx context.Context, threadID, label string) bool {
	if m.HasLabelFn != nil {
		return m.HasLabelFn(ctx, threadID, label)
	}
	return false
}

func (m *MockStore) GetLabels(ctx context.Context, threadID string) []string {
	if m.GetLabelsFn != nil {
		return m.GetLabelsFn(ctx, threadID)
	}
	return []string{}
}

func (m *MockStore) BulkAddLabel(ctx context.Context, threadIDs []string, orgID, label string) error {
	if m.BulkAddLabelFn != nil {
		return m.BulkAddLabelFn(ctx, threadIDs, orgID, label)
	}
	return nil
}

func (m *MockStore) BulkRemoveLabel(ctx context.Context, threadIDs []string, label string) error {
	if m.BulkRemoveLabelFn != nil {
		return m.BulkRemoveLabelFn(ctx, threadIDs, label)
	}
	return nil
}

// ===========================================================================
// EmailStore
// ===========================================================================

func (m *MockStore) LoadAttachmentsForResend(ctx context.Context, ids []string, orgID string) ([]map[string]string, error) {
	if m.LoadAttachmentsForResendFn != nil {
		return m.LoadAttachmentsForResendFn(ctx, ids, orgID)
	}
	return []map[string]string{}, nil
}

func (m *MockStore) CheckBouncedRecipients(ctx context.Context, orgID string, addresses []string) ([]string, error) {
	if m.CheckBouncedRecipientsFn != nil {
		return m.CheckBouncedRecipientsFn(ctx, orgID, addresses)
	}
	return []string{}, nil
}

func (m *MockStore) CanSendAs(ctx context.Context, userID, orgID, fromAddress, role string) (bool, error) {
	if m.CanSendAsFn != nil {
		return m.CanSendAsFn(ctx, userID, orgID, fromAddress, role)
	}
	return false, nil
}

func (m *MockStore) GetDomainStatus(ctx context.Context, domainID, orgID string) (string, error) {
	if m.GetDomainStatusFn != nil {
		return m.GetDomainStatusFn(ctx, domainID, orgID)
	}
	return "", nil
}

func (m *MockStore) ResolveFromDisplay(ctx context.Context, orgID, address string) (string, error) {
	if m.ResolveFromDisplayFn != nil {
		return m.ResolveFromDisplayFn(ctx, orgID, address)
	}
	return "", nil
}

func (m *MockStore) LookupDomainByName(ctx context.Context, orgID, domainName string) (string, error) {
	if m.LookupDomainByNameFn != nil {
		return m.LookupDomainByNameFn(ctx, orgID, domainName)
	}
	return "", nil
}

func (m *MockStore) InsertEmail(ctx context.Context, threadID, userID, orgID, domainID, direction, from string, toJSON, ccJSON, bccJSON []byte, subject, bodyHTML, bodyPlain, status string, inReplyTo string, refsJSON []byte) (string, error) {
	if m.InsertEmailFn != nil {
		return m.InsertEmailFn(ctx, threadID, userID, orgID, domainID, direction, from, toJSON, ccJSON, bccJSON, subject, bodyHTML, bodyPlain, status, inReplyTo, refsJSON)
	}
	return "", nil
}

func (m *MockStore) UpdateThreadStats(ctx context.Context, threadID, snippet, lastSender string) error {
	if m.UpdateThreadStatsFn != nil {
		return m.UpdateThreadStatsFn(ctx, threadID, snippet, lastSender)
	}
	return nil
}

func (m *MockStore) CreateEmailJob(ctx context.Context, orgID, userID, domainID, jobType, emailID, threadID string, resendPayload []byte, draftID *string) (string, error) {
	if m.CreateEmailJobFn != nil {
		return m.CreateEmailJobFn(ctx, orgID, userID, domainID, jobType, emailID, threadID, resendPayload, draftID)
	}
	return "", nil
}

func (m *MockStore) SearchEmails(ctx context.Context, orgID, query, domainID, role string, aliasAddrs []string) ([]map[string]any, error) {
	if m.SearchEmailsFn != nil {
		return m.SearchEmailsFn(ctx, orgID, query, domainID, role, aliasAddrs)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) ListAdminJobs(ctx context.Context, orgID string) ([]map[string]any, error) {
	if m.ListAdminJobsFn != nil {
		return m.ListAdminJobsFn(ctx, orgID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) CheckSendJobExists(ctx context.Context, draftID string) (bool, error) {
	if m.CheckSendJobExistsFn != nil {
		return m.CheckSendJobExistsFn(ctx, draftID)
	}
	return false, nil
}

// ===========================================================================
// DomainStore
// ===========================================================================

func (m *MockStore) ListDomains(ctx context.Context, orgID string, includeHidden bool) ([]map[string]any, error) {
	if m.ListDomainsFn != nil {
		return m.ListDomainsFn(ctx, orgID, includeHidden)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) InsertDomain(ctx context.Context, orgID, domain, resendDomainID, status string, dnsRecords json.RawMessage) (string, error) {
	if m.InsertDomainFn != nil {
		return m.InsertDomainFn(ctx, orgID, domain, resendDomainID, status, dnsRecords)
	}
	return "", nil
}

func (m *MockStore) GetResendDomainID(ctx context.Context, domainID, orgID string) (string, error) {
	if m.GetResendDomainIDFn != nil {
		return m.GetResendDomainIDFn(ctx, domainID, orgID)
	}
	return "", nil
}

func (m *MockStore) UpdateDomainStatus(ctx context.Context, domainID, status string, dnsRecords json.RawMessage) error {
	if m.UpdateDomainStatusFn != nil {
		return m.UpdateDomainStatusFn(ctx, domainID, status, dnsRecords)
	}
	return nil
}

func (m *MockStore) ReorderDomains(ctx context.Context, orgID string, order []DomainOrder) error {
	if m.ReorderDomainsFn != nil {
		return m.ReorderDomainsFn(ctx, orgID, order)
	}
	return nil
}

func (m *MockStore) GetUnreadCounts(ctx context.Context, orgID string, role string, aliasAddrs []string) (map[string]int, error) {
	if m.GetUnreadCountsFn != nil {
		return m.GetUnreadCountsFn(ctx, orgID, role, aliasAddrs)
	}
	return map[string]int{}, nil
}

func (m *MockStore) UpdateDomainVisibility(ctx context.Context, orgID string, visibleIDs []string) error {
	if m.UpdateDomainVisibilityFn != nil {
		return m.UpdateDomainVisibilityFn(ctx, orgID, visibleIDs)
	}
	return nil
}

func (m *MockStore) SyncDomains(ctx context.Context, orgID string, resendDomains []ResendDomainInfo) error {
	if m.SyncDomainsFn != nil {
		return m.SyncDomainsFn(ctx, orgID, resendDomains)
	}
	return nil
}

func (m *MockStore) SoftDeleteDomain(ctx context.Context, domainID, orgID string) (int64, error) {
	if m.SoftDeleteDomainFn != nil {
		return m.SoftDeleteDomainFn(ctx, domainID, orgID)
	}
	return 0, nil
}

func (m *MockStore) CascadeDeleteDomain(ctx context.Context, domainID string) error {
	if m.CascadeDeleteDomainFn != nil {
		return m.CascadeDeleteDomainFn(ctx, domainID)
	}
	return nil
}

func (m *MockStore) UpdateWebhookConfig(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error {
	if m.UpdateWebhookConfigFn != nil {
		return m.UpdateWebhookConfigFn(ctx, orgID, webhookID, encSecret, encIV, encTag)
	}
	return nil
}

func (m *MockStore) ListDiscoveredDomains(ctx context.Context, orgID string) ([]map[string]any, error) {
	return nil, nil
}

func (m *MockStore) DismissDiscoveredDomain(ctx context.Context, id, orgID string) error {
	return nil
}

// ===========================================================================
// UserStore
// ===========================================================================

func (m *MockStore) ListUsers(ctx context.Context, orgID string) ([]map[string]any, error) {
	if m.ListUsersFn != nil {
		return m.ListUsersFn(ctx, orgID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) InsertInvitedUser(ctx context.Context, orgID, email, name, role, token string, expiresAt time.Time) (string, error) {
	if m.InsertInvitedUserFn != nil {
		return m.InsertInvitedUserFn(ctx, orgID, email, name, role, token, expiresAt)
	}
	return "", nil
}

func (m *MockStore) GetOrgName(ctx context.Context, orgID string) (string, error) {
	if m.GetOrgNameFn != nil {
		return m.GetOrgNameFn(ctx, orgID)
	}
	return "", nil
}

func (m *MockStore) GetUserName(ctx context.Context, userID string) (string, error) {
	if m.GetUserNameFn != nil {
		return m.GetUserNameFn(ctx, userID)
	}
	return "", nil
}

func (m *MockStore) ReinviteUser(ctx context.Context, userID, orgID, token string, expiresAt time.Time) (string, error) {
	if m.ReinviteUserFn != nil {
		return m.ReinviteUserFn(ctx, userID, orgID, token, expiresAt)
	}
	return "", nil
}

func (m *MockStore) GetUserRole(ctx context.Context, userID, orgID string) (string, error) {
	if m.GetUserRoleFn != nil {
		return m.GetUserRoleFn(ctx, userID, orgID)
	}
	return "", nil
}

func (m *MockStore) CountActiveAdmins(ctx context.Context, orgID string) (int, error) {
	if m.CountActiveAdminsFn != nil {
		return m.CountActiveAdminsFn(ctx, orgID)
	}
	return 0, nil
}

func (m *MockStore) DisableUser(ctx context.Context, userID, orgID string) (int64, error) {
	if m.DisableUserFn != nil {
		return m.DisableUserFn(ctx, userID, orgID)
	}
	return 0, nil
}

func (m *MockStore) DeleteAliasUsers(ctx context.Context, userID string) error {
	if m.DeleteAliasUsersFn != nil {
		return m.DeleteAliasUsersFn(ctx, userID)
	}
	return nil
}

func (m *MockStore) ReassignAndDisable(ctx context.Context, orgID, adminID, sourceID, targetID string) (map[string]any, error) {
	if m.ReassignAndDisableFn != nil {
		return m.ReassignAndDisableFn(ctx, orgID, adminID, sourceID, targetID)
	}
	return map[string]any{}, nil
}

func (m *MockStore) GetMe(ctx context.Context, userID string) (map[string]any, error) {
	if m.GetMeFn != nil {
		return m.GetMeFn(ctx, userID)
	}
	return map[string]any{}, nil
}

func (m *MockStore) UpdateUserName(ctx context.Context, userID, name string) error {
	if m.UpdateUserNameFn != nil {
		return m.UpdateUserNameFn(ctx, userID, name)
	}
	return nil
}

func (m *MockStore) GetPasswordHash(ctx context.Context, userID string) (string, error) {
	if m.GetPasswordHashFn != nil {
		return m.GetPasswordHashFn(ctx, userID)
	}
	return "", nil
}

func (m *MockStore) UpdatePassword(ctx context.Context, userID, hash string) error {
	if m.UpdatePasswordFn != nil {
		return m.UpdatePasswordFn(ctx, userID, hash)
	}
	return nil
}

func (m *MockStore) GetPreferences(ctx context.Context, userID string) ([]byte, error) {
	if m.GetPreferencesFn != nil {
		return m.GetPreferencesFn(ctx, userID)
	}
	return []byte{}, nil
}

func (m *MockStore) UpdatePreferences(ctx context.Context, userID string, prefs map[string]any) error {
	if m.UpdatePreferencesFn != nil {
		return m.UpdatePreferencesFn(ctx, userID, prefs)
	}
	return nil
}

func (m *MockStore) ListMyAliases(ctx context.Context, userID, orgID string) ([]map[string]any, error) {
	if m.ListMyAliasesFn != nil {
		return m.ListMyAliasesFn(ctx, userID, orgID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) ChangeRole(ctx context.Context, userID, orgID, role string) (int64, error) {
	if m.ChangeRoleFn != nil {
		return m.ChangeRoleFn(ctx, userID, orgID, role)
	}
	return 0, nil
}

func (m *MockStore) GetUserOwnerAndRole(ctx context.Context, userID, orgID string) (bool, string, error) {
	if m.GetUserOwnerAndRoleFn != nil {
		return m.GetUserOwnerAndRoleFn(ctx, userID, orgID)
	}
	return false, "", nil
}

func (m *MockStore) EnableUser(ctx context.Context, userID, orgID string) (int64, error) {
	if m.EnableUserFn != nil {
		return m.EnableUserFn(ctx, userID, orgID)
	}
	return 0, nil
}

// ===========================================================================
// AliasStore
// ===========================================================================

func (m *MockStore) ListAliases(ctx context.Context, orgID, domainID string) ([]map[string]any, error) {
	if m.ListAliasesFn != nil {
		return m.ListAliasesFn(ctx, orgID, domainID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) CreateAlias(ctx context.Context, orgID, domainID, address, name string) (string, error) {
	if m.CreateAliasFn != nil {
		return m.CreateAliasFn(ctx, orgID, domainID, address, name)
	}
	return "", nil
}

func (m *MockStore) UpdateAlias(ctx context.Context, aliasID, orgID, name string) (int64, error) {
	if m.UpdateAliasFn != nil {
		return m.UpdateAliasFn(ctx, aliasID, orgID, name)
	}
	return 0, nil
}

func (m *MockStore) DeleteAlias(ctx context.Context, aliasID, orgID string) (int64, error) {
	if m.DeleteAliasFn != nil {
		return m.DeleteAliasFn(ctx, aliasID, orgID)
	}
	return 0, nil
}

func (m *MockStore) AddAliasUser(ctx context.Context, aliasID, orgID, userID string, canSendAs bool) error {
	if m.AddAliasUserFn != nil {
		return m.AddAliasUserFn(ctx, aliasID, orgID, userID, canSendAs)
	}
	return nil
}

func (m *MockStore) RemoveAliasUser(ctx context.Context, aliasID, userID string) error {
	if m.RemoveAliasUserFn != nil {
		return m.RemoveAliasUserFn(ctx, aliasID, userID)
	}
	return nil
}

func (m *MockStore) SetDefaultAlias(ctx context.Context, aliasID, userID, orgID string) error {
	if m.SetDefaultAliasFn != nil {
		return m.SetDefaultAliasFn(ctx, aliasID, userID, orgID)
	}
	return nil
}

func (m *MockStore) ListDiscoveredAddresses(ctx context.Context, orgID string) ([]map[string]any, error) {
	if m.ListDiscoveredAddressesFn != nil {
		return m.ListDiscoveredAddressesFn(ctx, orgID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) CheckAliasOrg(ctx context.Context, aliasID, orgID string) (int, error) {
	if m.CheckAliasOrgFn != nil {
		return m.CheckAliasOrgFn(ctx, aliasID, orgID)
	}
	return 0, nil
}

func (m *MockStore) CheckUserOrg(ctx context.Context, userID, orgID string) (bool, error) {
	if m.CheckUserOrgFn != nil {
		return m.CheckUserOrgFn(ctx, userID, orgID)
	}
	return false, nil
}

// ===========================================================================
// LabelStore
// ===========================================================================

func (m *MockStore) ListOrgLabels(ctx context.Context, orgID string) ([]map[string]any, error) {
	if m.ListOrgLabelsFn != nil {
		return m.ListOrgLabelsFn(ctx, orgID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) CreateOrgLabel(ctx context.Context, orgID, name string) (string, error) {
	if m.CreateOrgLabelFn != nil {
		return m.CreateOrgLabelFn(ctx, orgID, name)
	}
	return "", nil
}

func (m *MockStore) RenameOrgLabel(ctx context.Context, labelID, orgID, newName string) (string, error) {
	if m.RenameOrgLabelFn != nil {
		return m.RenameOrgLabelFn(ctx, labelID, orgID, newName)
	}
	return "", nil
}

func (m *MockStore) DeleteOrgLabel(ctx context.Context, labelID, orgID string) (string, error) {
	if m.DeleteOrgLabelFn != nil {
		return m.DeleteOrgLabelFn(ctx, labelID, orgID)
	}
	return "", nil
}

// ===========================================================================
// DraftStore
// ===========================================================================

func (m *MockStore) ListDrafts(ctx context.Context, userID, orgID, domainID string) ([]map[string]any, error) {
	if m.ListDraftsFn != nil {
		return m.ListDraftsFn(ctx, userID, orgID, domainID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) CreateDraft(ctx context.Context, orgID, userID, domainID string, threadID *string, kind, subject, fromAddress string, toJSON, ccJSON, bccJSON, attJSON []byte) (string, error) {
	if m.CreateDraftFn != nil {
		return m.CreateDraftFn(ctx, orgID, userID, domainID, threadID, kind, subject, fromAddress, toJSON, ccJSON, bccJSON, attJSON)
	}
	return "", nil
}

func (m *MockStore) UpdateDraft(ctx context.Context, draftID, userID string, sets []string, args []any) (int64, error) {
	if m.UpdateDraftFn != nil {
		return m.UpdateDraftFn(ctx, draftID, userID, sets, args)
	}
	return 0, nil
}

func (m *MockStore) DeleteDraft(ctx context.Context, draftID, userID string) (int64, error) {
	if m.DeleteDraftFn != nil {
		return m.DeleteDraftFn(ctx, draftID, userID)
	}
	return 0, nil
}

func (m *MockStore) GetDraft(ctx context.Context, draftID, userID, orgID string) (string, *string, string, string, string, string, string, json.RawMessage, json.RawMessage, json.RawMessage, json.RawMessage, error) {
	if m.GetDraftFn != nil {
		return m.GetDraftFn(ctx, draftID, userID, orgID)
	}
	return "", nil, "", "", "", "", "", json.RawMessage{}, json.RawMessage{}, json.RawMessage{}, json.RawMessage{}, nil
}

// ===========================================================================
// OrgStore
// ===========================================================================

func (m *MockStore) GetOrgSettings(ctx context.Context, orgID string) (map[string]any, error) {
	if m.GetOrgSettingsFn != nil {
		return m.GetOrgSettingsFn(ctx, orgID)
	}
	return map[string]any{}, nil
}

func (m *MockStore) UpdateOrgName(ctx context.Context, orgID, name string) error {
	if m.UpdateOrgNameFn != nil {
		return m.UpdateOrgNameFn(ctx, orgID, name)
	}
	return nil
}

func (m *MockStore) UpdateOrgAPIKey(ctx context.Context, orgID string, ciphertext, iv, tag string) error {
	if m.UpdateOrgAPIKeyFn != nil {
		return m.UpdateOrgAPIKeyFn(ctx, orgID, ciphertext, iv, tag)
	}
	return nil
}

func (m *MockStore) UpdateOrgRPS(ctx context.Context, orgID string, rps int) error {
	if m.UpdateOrgRPSFn != nil {
		return m.UpdateOrgRPSFn(ctx, orgID, rps)
	}
	return nil
}

func (m *MockStore) UpdateOrgAutoPoll(ctx context.Context, orgID string, enabled bool) error {
	if m.UpdateOrgAutoPollFn != nil {
		return m.UpdateOrgAutoPollFn(ctx, orgID, enabled)
	}
	return nil
}

func (m *MockStore) UpdateOrgAutoPollInterval(ctx context.Context, orgID string, interval int) error {
	if m.UpdateOrgAutoPollIntervalFn != nil {
		return m.UpdateOrgAutoPollIntervalFn(ctx, orgID, interval)
	}
	return nil
}

func (m *MockStore) IsOrgOwner(ctx context.Context, userID, orgID string) (bool, error) {
	if m.IsOrgOwnerFn != nil {
		return m.IsOrgOwnerFn(ctx, userID, orgID)
	}
	return false, nil
}

func (m *MockStore) GetStripeSubscriptionID(ctx context.Context, orgID string) (*string, error) {
	if m.GetStripeSubscriptionIDFn != nil {
		return m.GetStripeSubscriptionIDFn(ctx, orgID)
	}
	return nil, nil
}

func (m *MockStore) GetWebhookID(ctx context.Context, orgID string) (*string, error) {
	if m.GetWebhookIDFn != nil {
		return m.GetWebhookIDFn(ctx, orgID)
	}
	return nil, nil
}

func (m *MockStore) SoftDeleteOrg(ctx context.Context, orgID string) error {
	if m.SoftDeleteOrgFn != nil {
		return m.SoftDeleteOrgFn(ctx, orgID)
	}
	return nil
}

func (m *MockStore) ListOrgUserIDs(ctx context.Context, orgID string) ([]string, error) {
	if m.ListOrgUserIDsFn != nil {
		return m.ListOrgUserIDsFn(ctx, orgID)
	}
	return []string{}, nil
}

func (m *MockStore) CancelOrgJobs(ctx context.Context, orgID string) ([]string, error) {
	if m.CancelOrgJobsFn != nil {
		return m.CancelOrgJobsFn(ctx, orgID)
	}
	return []string{}, nil
}

func (m *MockStore) GetOrgNameByID(ctx context.Context, orgID string) (string, error) {
	if m.GetOrgNameByIDFn != nil {
		return m.GetOrgNameByIDFn(ctx, orgID)
	}
	return "", nil
}

func (m *MockStore) HardDeleteOrg(ctx context.Context, orgID string) (int64, error) {
	if m.HardDeleteOrgFn != nil {
		return m.HardDeleteOrgFn(ctx, orgID)
	}
	return 0, nil
}

// ===========================================================================
// ContactStore
// ===========================================================================

func (m *MockStore) SuggestContacts(ctx context.Context, orgID, query string, limit int) ([]map[string]any, error) {
	if m.SuggestContactsFn != nil {
		return m.SuggestContactsFn(ctx, orgID, query, limit)
	}
	return []map[string]any{}, nil
}

// ===========================================================================
// WebhookStore
// ===========================================================================

func (m *MockStore) GetOrgWebhookSecret(ctx context.Context, orgID string) (string, string, string, *string, error) {
	if m.GetOrgWebhookSecretFn != nil {
		return m.GetOrgWebhookSecretFn(ctx, orgID)
	}
	return "", "", "", nil, nil
}

func (m *MockStore) CheckWebhookDedup(ctx context.Context, orgID, resendEmailID, eventType string) (bool, error) {
	if m.CheckWebhookDedupFn != nil {
		return m.CheckWebhookDedupFn(ctx, orgID, resendEmailID, eventType)
	}
	return false, nil
}

func (m *MockStore) InsertWebhookDedup(ctx context.Context, orgID, resendEmailID, eventType string) error {
	if m.InsertWebhookDedupFn != nil {
		return m.InsertWebhookDedupFn(ctx, orgID, resendEmailID, eventType)
	}
	return nil
}

func (m *MockStore) UpdateEmailStatus(ctx context.Context, orgID, resendEmailID, status string) (int64, error) {
	if m.UpdateEmailStatusFn != nil {
		return m.UpdateEmailStatusFn(ctx, orgID, resendEmailID, status)
	}
	return 0, nil
}

func (m *MockStore) GetEmailThreadByResendID(ctx context.Context, orgID, resendEmailID string) (string, string, string, error) {
	if m.GetEmailThreadByResendIDFn != nil {
		return m.GetEmailThreadByResendIDFn(ctx, orgID, resendEmailID)
	}
	return "", "", "", nil
}

func (m *MockStore) InsertBounce(ctx context.Context, orgID, address, bounceType string) error {
	if m.InsertBounceFn != nil {
		return m.InsertBounceFn(ctx, orgID, address, bounceType)
	}
	return nil
}

func (m *MockStore) ClearBounce(ctx context.Context, orgID, fromAddress string) error {
	if m.ClearBounceFn != nil {
		return m.ClearBounceFn(ctx, orgID, fromAddress)
	}
	return nil
}

// ===========================================================================
// BillingStore
// ===========================================================================

func (m *MockStore) GetBillingInfo(ctx context.Context, orgID string) (map[string]any, error) {
	if m.GetBillingInfoFn != nil {
		return m.GetBillingInfoFn(ctx, orgID)
	}
	return map[string]any{}, nil
}

func (m *MockStore) GetStripeCustomerID(ctx context.Context, orgID string) (*string, error) {
	if m.GetStripeCustomerIDFn != nil {
		return m.GetStripeCustomerIDFn(ctx, orgID)
	}
	return nil, nil
}

func (m *MockStore) SetStripeCustomerID(ctx context.Context, orgID, customerID string) error {
	if m.SetStripeCustomerIDFn != nil {
		return m.SetStripeCustomerIDFn(ctx, orgID, customerID)
	}
	return nil
}

func (m *MockStore) UpdateOrgPlan(ctx context.Context, orgID, plan, subID string) error {
	if m.UpdateOrgPlanFn != nil {
		return m.UpdateOrgPlanFn(ctx, orgID, plan, subID)
	}
	return nil
}

func (m *MockStore) SetPlanExpiry(ctx context.Context, orgID string, expiresAt time.Time) error {
	if m.SetPlanExpiryFn != nil {
		return m.SetPlanExpiryFn(ctx, orgID, expiresAt)
	}
	return nil
}

func (m *MockStore) ClearPlanExpiry(ctx context.Context, orgID string) error {
	if m.ClearPlanExpiryFn != nil {
		return m.ClearPlanExpiryFn(ctx, orgID)
	}
	return nil
}

func (m *MockStore) InsertStripeEvent(ctx context.Context, eventID string) (bool, error) {
	if m.InsertStripeEventFn != nil {
		return m.InsertStripeEventFn(ctx, eventID)
	}
	return false, nil
}

// ===========================================================================
// OnboardingStore
// ===========================================================================

func (m *MockStore) HasAPIKey(ctx context.Context, orgID string) (bool, error) {
	if m.HasAPIKeyFn != nil {
		return m.HasAPIKeyFn(ctx, orgID)
	}
	return false, nil
}

func (m *MockStore) CountVisibleDomains(ctx context.Context, orgID string) (int, error) {
	if m.CountVisibleDomainsFn != nil {
		return m.CountVisibleDomainsFn(ctx, orgID)
	}
	return 0, nil
}

func (m *MockStore) GetActiveSyncJob(ctx context.Context, orgID string) (string, string, error) {
	if m.GetActiveSyncJobFn != nil {
		return m.GetActiveSyncJobFn(ctx, orgID)
	}
	return "", "", nil
}

func (m *MockStore) CountEmails(ctx context.Context, orgID string) (int, error) {
	if m.CountEmailsFn != nil {
		return m.CountEmailsFn(ctx, orgID)
	}
	return 0, nil
}

func (m *MockStore) StoreEncryptedAPIKey(ctx context.Context, orgID string, ciphertext, iv, tag string) error {
	if m.StoreEncryptedAPIKeyFn != nil {
		return m.StoreEncryptedAPIKeyFn(ctx, orgID, ciphertext, iv, tag)
	}
	return nil
}

func (m *MockStore) UpsertDomain(ctx context.Context, orgID, domain, resendDomainID, status string, records json.RawMessage, order int) (string, error) {
	if m.UpsertDomainFn != nil {
		return m.UpsertDomainFn(ctx, orgID, domain, resendDomainID, status, records, order)
	}
	return "", nil
}

func (m *MockStore) SelectDomains(ctx context.Context, orgID string, domainIDs []string) error {
	if m.SelectDomainsFn != nil {
		return m.SelectDomainsFn(ctx, orgID, domainIDs)
	}
	return nil
}

func (m *MockStore) StoreWebhookConfig(ctx context.Context, orgID, webhookID string, encSecret, encIV, encTag string) error {
	if m.StoreWebhookConfigFn != nil {
		return m.StoreWebhookConfigFn(ctx, orgID, webhookID, encSecret, encIV, encTag)
	}
	return nil
}

func (m *MockStore) GetDiscoveredAddresses(ctx context.Context, orgID string) ([]map[string]any, error) {
	if m.GetDiscoveredAddressesFn != nil {
		return m.GetDiscoveredAddressesFn(ctx, orgID)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) SetupAddress(ctx context.Context, orgID, userID, address, addrType, name string) error {
	if m.SetupAddressFn != nil {
		return m.SetupAddressFn(ctx, orgID, userID, address, addrType, name)
	}
	return nil
}

func (m *MockStore) CompleteOnboarding(ctx context.Context, orgID string) error {
	if m.CompleteOnboardingFn != nil {
		return m.CompleteOnboardingFn(ctx, orgID)
	}
	return nil
}

func (m *MockStore) GetFirstDomainID(ctx context.Context, orgID string) (string, error) {
	if m.GetFirstDomainIDFn != nil {
		return m.GetFirstDomainIDFn(ctx, orgID)
	}
	return "", nil
}

// ===========================================================================
// SetupStore
// ===========================================================================

func (m *MockStore) SetupCountUsers(ctx context.Context) (int, error) {
	if m.SetupCountUsersFn != nil {
		return m.SetupCountUsersFn(ctx)
	}
	return 0, nil
}

func (m *MockStore) CreateAdminSetup(ctx context.Context, orgName, email, name, passwordHash, systemResendKey, systemFromAddress, systemFromName string, encSvc interface{ Encrypt(string) (string, string, string, error) }) (string, string, error) {
	if m.CreateAdminSetupFn != nil {
		return m.CreateAdminSetupFn(ctx, orgName, email, name, passwordHash, systemResendKey, systemFromAddress, systemFromName, encSvc)
	}
	return "", "", nil
}

func (m *MockStore) UpsertSystemSetting(ctx context.Context, key, value string) error {
	if m.UpsertSystemSettingFn != nil {
		return m.UpsertSystemSettingFn(ctx, key, value)
	}
	return nil
}

func (m *MockStore) GetUserEmail(ctx context.Context, userID string) (string, error) {
	if m.GetUserEmailFn != nil {
		return m.GetUserEmailFn(ctx, userID)
	}
	return "", nil
}

// ===========================================================================
// EventStore
// ===========================================================================

func (m *MockStore) GetEventsSince(ctx context.Context, orgID string, since time.Time) ([]map[string]any, error) {
	if m.GetEventsSinceFn != nil {
		return m.GetEventsSinceFn(ctx, orgID, since)
	}
	return []map[string]any{}, nil
}

// ===========================================================================
// SyncStore
// ===========================================================================

func (m *MockStore) CreateSyncJob(ctx context.Context, orgID, userID string) (string, error) {
	if m.CreateSyncJobFn != nil {
		return m.CreateSyncJobFn(ctx, orgID, userID)
	}
	return "", nil
}

func (m *MockStore) GetSyncJob(ctx context.Context, jobID, orgID string) (map[string]any, error) {
	if m.GetSyncJobFn != nil {
		return m.GetSyncJobFn(ctx, jobID, orgID)
	}
	return map[string]any{}, nil
}

// ===========================================================================
// CronStore
// ===========================================================================

func (m *MockStore) PurgeExpiredTrash(ctx context.Context) (int64, error) {
	if m.PurgeExpiredTrashFn != nil {
		return m.PurgeExpiredTrashFn(ctx)
	}
	return 0, nil
}

func (m *MockStore) CleanupStaleWebhooks(ctx context.Context, orgIDs []string) error {
	if m.CleanupStaleWebhooksFn != nil {
		return m.CleanupStaleWebhooksFn(ctx, orgIDs)
	}
	return nil
}

// ===========================================================================
// WorkerStore
// ===========================================================================

func (m *MockStore) GetEmailJob(ctx context.Context, jobID string) (map[string]any, error) {
	if m.GetEmailJobFn != nil {
		return m.GetEmailJobFn(ctx, jobID)
	}
	return map[string]any{}, nil
}

func (m *MockStore) UpdateEmailJobStatus(ctx context.Context, jobID, status, errorMsg string) error {
	if m.UpdateEmailJobStatusFn != nil {
		return m.UpdateEmailJobStatusFn(ctx, jobID, status, errorMsg)
	}
	return nil
}

func (m *MockStore) UpdateEmailJobHeartbeat(ctx context.Context, jobID string) error {
	if m.UpdateEmailJobHeartbeatFn != nil {
		return m.UpdateEmailJobHeartbeatFn(ctx, jobID)
	}
	return nil
}

func (m *MockStore) IncrementJobAttempts(ctx context.Context, jobID string) error {
	if m.IncrementJobAttemptsFn != nil {
		return m.IncrementJobAttemptsFn(ctx, jobID)
	}
	return nil
}

func (m *MockStore) GetStaleJobs(ctx context.Context, timeout time.Duration) ([]string, error) {
	if m.GetStaleJobsFn != nil {
		return m.GetStaleJobsFn(ctx, timeout)
	}
	return []string{}, nil
}

func (m *MockStore) GetOrphanedJobs(ctx context.Context, age time.Duration) ([]string, error) {
	if m.GetOrphanedJobsFn != nil {
		return m.GetOrphanedJobsFn(ctx, age)
	}
	return []string{}, nil
}

// ===========================================================================
// PollerStore
// ===========================================================================

func (m *MockStore) GetPollableOrgs(ctx context.Context) ([]map[string]any, error) {
	if m.GetPollableOrgsFn != nil {
		return m.GetPollableOrgsFn(ctx)
	}
	return []map[string]any{}, nil
}

func (m *MockStore) HasPendingSyncJob(ctx context.Context, orgID string) (bool, error) {
	if m.HasPendingSyncJobFn != nil {
		return m.HasPendingSyncJobFn(ctx, orgID)
	}
	return false, nil
}

func (m *MockStore) EmailExistsByResendID(ctx context.Context, orgID, resendEmailID string) (bool, error) {
	if m.EmailExistsByResendIDFn != nil {
		return m.EmailExistsByResendIDFn(ctx, orgID, resendEmailID)
	}
	return false, nil
}

func (m *MockStore) CreateFetchJob(ctx context.Context, orgID, resendEmailID, jobType string) (string, error) {
	if m.CreateFetchJobFn != nil {
		return m.CreateFetchJobFn(ctx, orgID, resendEmailID, jobType)
	}
	return "", nil
}

// MockQuerier implements Querier for testing. All methods return safe zero values.
var _ Querier = (*MockQuerier)(nil)

type MockQuerier struct {
	ExecFn     func(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryFn    func(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRowFn func(ctx context.Context, sql string, args ...any) pgx.Row
}

func (q *MockQuerier) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if q.ExecFn != nil {
		return q.ExecFn(ctx, sql, args...)
	}
	return pgconn.NewCommandTag(""), nil
}

func (q *MockQuerier) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if q.QueryFn != nil {
		return q.QueryFn(ctx, sql, args...)
	}
	return nil, nil
}

func (q *MockQuerier) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if q.QueryRowFn != nil {
		return q.QueryRowFn(ctx, sql, args...)
	}
	return nil
}
