-- +goose Up

-- Enums
CREATE TYPE user_role AS ENUM ('admin', 'member');
CREATE TYPE user_status AS ENUM ('placeholder', 'invited', 'active', 'disabled');
CREATE TYPE domain_status AS ENUM ('pending', 'verified', 'active', 'disconnected', 'deleted');
CREATE TYPE email_direction AS ENUM ('inbound', 'outbound');
CREATE TYPE email_status AS ENUM ('received', 'queued', 'sent', 'delivered', 'bounced', 'failed', 'complained');
CREATE TYPE address_type AS ENUM ('unclaimed', 'individual', 'group');
CREATE TYPE sync_job_status AS ENUM ('pending', 'running', 'completed', 'failed');

-- Orgs
CREATE TABLE orgs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  resend_api_key_encrypted TEXT,
  resend_api_key_iv TEXT,
  resend_api_key_tag TEXT,
  resend_webhook_id TEXT,
  resend_webhook_secret TEXT,
  resend_webhook_secret_encrypted TEXT,
  resend_webhook_secret_iv TEXT,
  resend_webhook_secret_tag TEXT,
  stripe_customer_id TEXT,
  stripe_subscription_id TEXT,
  plan TEXT NOT NULL DEFAULT 'free',
  plan_expires_at TIMESTAMPTZ,
  resend_rps INTEGER NOT NULL DEFAULT 2,
  auto_poll_enabled BOOLEAN NOT NULL DEFAULT false,
  auto_poll_interval INT NOT NULL DEFAULT 300,
  last_polled_at TIMESTAMPTZ,
  onboarding_completed BOOLEAN NOT NULL DEFAULT false,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX idx_orgs_stripe_customer ON orgs(stripe_customer_id) WHERE stripe_customer_id IS NOT NULL;

-- Users
CREATE TABLE users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  email TEXT NOT NULL UNIQUE, -- NOTE: Global uniqueness blocks multi-org. See PRD-120: change to UNIQUE(org_id, email) when multi-org is needed.
  name TEXT NOT NULL DEFAULT '',
  password_hash TEXT,
  role user_role NOT NULL DEFAULT 'member',
  status user_status NOT NULL DEFAULT 'placeholder',
  invite_token TEXT,
  invite_expires_at TIMESTAMPTZ,
  reset_token TEXT,
  reset_expires_at TIMESTAMPTZ,
  email_verified BOOLEAN NOT NULL DEFAULT true,
  verification_code TEXT,
  verification_expires_at TIMESTAMPTZ,
  is_owner BOOLEAN NOT NULL DEFAULT false,
  notification_preferences JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_org ON users(org_id);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_invite_token ON users(invite_token) WHERE invite_token IS NOT NULL;
CREATE INDEX idx_users_org_role_status ON users(org_id, role, status);

-- Domains
CREATE TABLE domains (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  domain TEXT NOT NULL,
  resend_domain_id TEXT,
  status domain_status NOT NULL DEFAULT 'pending',
  display_order INT NOT NULL DEFAULT 0,
  dns_records JSONB,
  hidden BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_domains_org ON domains(org_id);
CREATE UNIQUE INDEX idx_domains_unique_active ON domains(domain) WHERE status NOT IN ('deleted');

-- Threads
CREATE TABLE threads (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  user_id UUID NOT NULL REFERENCES users(id),
  domain_id UUID NOT NULL REFERENCES domains(id),
  subject TEXT NOT NULL,
  participant_emails JSONB NOT NULL DEFAULT '[]',
  last_message_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  message_count INT NOT NULL DEFAULT 0,
  unread_count INT NOT NULL DEFAULT 0,
  snippet TEXT NOT NULL DEFAULT '',
  original_to TEXT,
  trash_expires_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_threads_last_message ON threads(last_message_at DESC);
CREATE INDEX idx_threads_user_domain_active ON threads(user_id, domain_id, deleted_at, last_message_at DESC);
CREATE INDEX idx_threads_org_active_date ON threads(org_id, last_message_at DESC) WHERE deleted_at IS NULL;
CREATE INDEX idx_threads_deleted_at ON threads(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE INDEX idx_threads_participant_emails_gin ON threads USING GIN(participant_emails);

-- Emails
CREATE TABLE emails (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  thread_id UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id),
  org_id UUID NOT NULL REFERENCES orgs(id),
  domain_id UUID NOT NULL REFERENCES domains(id),
  resend_email_id TEXT,
  message_id TEXT,
  direction email_direction NOT NULL,
  from_address TEXT NOT NULL,
  to_addresses JSONB NOT NULL DEFAULT '[]',
  cc_addresses JSONB NOT NULL DEFAULT '[]',
  bcc_addresses JSONB NOT NULL DEFAULT '[]',
  subject TEXT NOT NULL,
  body_html TEXT,
  body_plain TEXT,
  status email_status NOT NULL DEFAULT 'received',
  attachments JSONB DEFAULT '[]',
  reply_to_addresses JSONB DEFAULT '[]',
  in_reply_to TEXT,
  references_header JSONB DEFAULT '[]',
  headers JSONB,
  last_event TEXT,
  delivered_via_alias TEXT,
  sent_as_alias TEXT,
  spam_score FLOAT,
  attachment_ids JSONB DEFAULT '[]',
  is_read BOOLEAN DEFAULT false,
  spam_reasons JSONB,
  search_vector TSVECTOR,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_emails_thread ON emails(thread_id);
CREATE INDEX idx_emails_user ON emails(user_id);
CREATE INDEX idx_emails_domain ON emails(domain_id);
CREATE INDEX idx_emails_message_id ON emails(message_id);
CREATE UNIQUE INDEX idx_emails_resend_id ON emails(resend_email_id) WHERE resend_email_id IS NOT NULL;
CREATE INDEX idx_emails_search ON emails USING GIN(search_vector);
CREATE INDEX idx_emails_delivered_via_alias ON emails(delivered_via_alias) WHERE delivered_via_alias IS NOT NULL;
CREATE INDEX idx_emails_sent_as_alias ON emails(sent_as_alias) WHERE sent_as_alias IS NOT NULL;
CREATE INDEX idx_emails_thread_alias ON emails(thread_id, delivered_via_alias, sent_as_alias);
CREATE INDEX idx_emails_to_addresses_gin ON emails USING GIN(to_addresses);
CREATE INDEX idx_emails_cc_addresses_gin ON emails USING GIN(cc_addresses);
CREATE INDEX idx_emails_org ON emails(org_id);

-- Aliases
CREATE TABLE aliases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  domain_id UUID NOT NULL REFERENCES domains(id),
  address TEXT NOT NULL,
  name TEXT NOT NULL DEFAULT '',
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(org_id, address)
);
CREATE INDEX idx_aliases_org ON aliases(org_id);
CREATE INDEX idx_aliases_domain ON aliases(domain_id);

-- Alias Users (junction)
CREATE TABLE alias_users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  alias_id UUID NOT NULL REFERENCES aliases(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id),
  can_send_as BOOLEAN NOT NULL DEFAULT true,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(alias_id, user_id)
);
CREATE INDEX idx_alias_users_user_id ON alias_users(user_id);

-- Discovered Addresses
CREATE TABLE discovered_addresses (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  domain_id UUID NOT NULL REFERENCES domains(id),
  address TEXT NOT NULL,
  local_part TEXT NOT NULL,
  type address_type NOT NULL DEFAULT 'unclaimed',
  user_id UUID REFERENCES users(id),
  alias_id UUID REFERENCES aliases(id),
  email_count INT NOT NULL DEFAULT 0,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(domain_id, address)
);
CREATE INDEX idx_discovered_domain ON discovered_addresses(domain_id);
CREATE INDEX idx_discovered_addresses_user_id ON discovered_addresses(user_id) WHERE user_id IS NOT NULL;

-- Discovered Domains (domains in Resend not yet added by the user)
CREATE TABLE discovered_domains (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  domain TEXT NOT NULL,
  first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  dismissed BOOLEAN NOT NULL DEFAULT false,
  UNIQUE(org_id, domain)
);

-- Thread Labels
CREATE TABLE thread_labels (
  thread_id UUID NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
  org_id    UUID NOT NULL REFERENCES orgs(id),
  label     TEXT NOT NULL,
  added_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (thread_id, label)
);
CREATE INDEX idx_thread_labels_org_label ON thread_labels(org_id, label, thread_id);
CREATE INDEX idx_thread_labels_thread ON thread_labels(thread_id, label);

-- Email Bounces
CREATE TABLE email_bounces (
  id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id     UUID NOT NULL REFERENCES orgs(id),
  address    TEXT NOT NULL,
  reason     TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX idx_email_bounces_org_address ON email_bounces(org_id, lower(address));

-- Stripe Events
CREATE TABLE stripe_events (
    event_id   TEXT PRIMARY KEY,
    event_type TEXT NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_stripe_events_processed_at ON stripe_events(processed_at);

-- Attachments
CREATE TABLE attachments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    user_id UUID NOT NULL REFERENCES users(id),
    filename TEXT NOT NULL,
    content_type TEXT NOT NULL,
    size INTEGER NOT NULL CHECK (size <= 10485760),
    data BYTEA NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_attachments_org ON attachments(org_id);

-- Org Labels
CREATE TABLE org_labels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    UNIQUE(org_id, name)
);

-- User Reassignments
CREATE TABLE user_reassignments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES orgs(id),
    source_user_id UUID NOT NULL REFERENCES users(id),
    target_user_id UUID NOT NULL REFERENCES users(id),
    reassigned_by UUID NOT NULL REFERENCES users(id),
    threads_moved INT NOT NULL DEFAULT 0,
    aliases_moved INT NOT NULL DEFAULT 0,
    drafts_moved INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- System Settings
CREATE TABLE system_settings (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  iv         TEXT,
  tag        TEXT,
  encrypted  BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Email Jobs
CREATE TABLE email_jobs (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          UUID NOT NULL REFERENCES orgs(id),
  user_id         UUID NOT NULL REFERENCES users(id),
  domain_id       UUID REFERENCES domains(id),
  job_type        TEXT NOT NULL,
  status          TEXT NOT NULL DEFAULT 'pending',

  -- Send jobs
  email_id        UUID REFERENCES emails(id),
  thread_id       UUID REFERENCES threads(id),
  resend_payload  JSONB,
  draft_id        UUID,

  -- Fetch jobs
  resend_email_id TEXT,
  webhook_data    JSONB,

  -- Retry
  retry_count     INT NOT NULL DEFAULT 0,
  max_retries     INT NOT NULL DEFAULT 5,
  error_message   TEXT,
  heartbeat_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_email_jobs_org_status ON email_jobs(org_id, status);
CREATE INDEX idx_email_jobs_status_heartbeat ON email_jobs(status, heartbeat_at) WHERE status = 'running';
CREATE INDEX idx_email_jobs_pending_send ON email_jobs(org_id, status, created_at) WHERE status = 'pending' AND job_type = 'send';
CREATE UNIQUE INDEX idx_email_jobs_resend_id_pending ON email_jobs(resend_email_id) WHERE status IN ('pending', 'running');
CREATE INDEX idx_email_jobs_user_id ON email_jobs(user_id);
CREATE INDEX idx_email_jobs_domain_id ON email_jobs(domain_id);

-- Sync Jobs
CREATE TABLE sync_jobs (
  id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id          UUID NOT NULL REFERENCES orgs(id),
  user_id         UUID NOT NULL REFERENCES users(id),
  status          sync_job_status NOT NULL DEFAULT 'pending',
  sent_cursor     TEXT NOT NULL DEFAULT '',
  received_cursor TEXT NOT NULL DEFAULT '',
  phase           TEXT NOT NULL DEFAULT 'pending',
  imported        INT NOT NULL DEFAULT 0,
  total           INT NOT NULL DEFAULT 0,
  sent_count      INT NOT NULL DEFAULT 0,
  received_count  INT NOT NULL DEFAULT 0,
  thread_count    INT NOT NULL DEFAULT 0,
  address_count   INT NOT NULL DEFAULT 0,
  retry_count     INT NOT NULL DEFAULT 0,
  max_retries     INT NOT NULL DEFAULT 3,
  error_message   TEXT,
  heartbeat_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_sync_jobs_org_status ON sync_jobs(org_id, status);
CREATE INDEX idx_sync_jobs_heartbeat ON sync_jobs(status, heartbeat_at) WHERE status = 'running';
CREATE INDEX idx_sync_jobs_user_id ON sync_jobs(user_id);

-- Drafts
CREATE TABLE drafts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  user_id UUID NOT NULL REFERENCES users(id),
  domain_id UUID NOT NULL REFERENCES domains(id),
  thread_id UUID REFERENCES threads(id),
  kind TEXT NOT NULL DEFAULT 'compose',
  subject TEXT NOT NULL DEFAULT '',
  from_address TEXT NOT NULL DEFAULT '',
  to_addresses JSONB NOT NULL DEFAULT '[]',
  cc_addresses JSONB NOT NULL DEFAULT '[]',
  bcc_addresses JSONB NOT NULL DEFAULT '[]',
  body_html TEXT NOT NULL DEFAULT '',
  body_plain TEXT NOT NULL DEFAULT '',
  attachment_ids JSONB DEFAULT '[]',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_drafts_user_domain ON drafts(user_id, domain_id);

-- Events
CREATE TABLE events (
  id         BIGSERIAL PRIMARY KEY,
  event_type TEXT NOT NULL,
  org_id     UUID NOT NULL,
  user_id    UUID,
  domain_id  UUID,
  thread_id  UUID,
  payload    JSONB NOT NULL DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_events_org_created ON events(org_id, created_at);
CREATE INDEX idx_events_created ON events(created_at);

-- Full-text search trigger
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION emails_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english',
    coalesce(NEW.subject, '') || ' ' ||
    coalesce(NEW.body_plain, '') || ' ' ||
    coalesce(NEW.from_address, '')
  );
  RETURN NEW;
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER emails_search_update
  BEFORE INSERT OR UPDATE ON emails
  FOR EACH ROW EXECUTE FUNCTION emails_search_trigger();
