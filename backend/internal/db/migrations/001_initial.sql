-- +goose Up

-- Enums
CREATE TYPE user_role AS ENUM ('admin', 'member');
CREATE TYPE user_status AS ENUM ('placeholder', 'invited', 'active', 'disabled');
CREATE TYPE domain_status AS ENUM ('pending', 'verified', 'active');
CREATE TYPE folder AS ENUM ('inbox', 'sent', 'archive', 'trash', 'spam');
CREATE TYPE email_direction AS ENUM ('inbound', 'outbound');
CREATE TYPE email_status AS ENUM ('received', 'sent', 'delivered', 'bounced', 'failed');
CREATE TYPE address_type AS ENUM ('unclaimed', 'user', 'alias');

-- Orgs
CREATE TABLE orgs (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  name TEXT NOT NULL,
  resend_api_key_encrypted TEXT,
  resend_api_key_iv TEXT,
  resend_api_key_tag TEXT,
  resend_webhook_id TEXT,
  resend_webhook_secret TEXT,
  onboarding_completed BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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
  notification_preferences JSONB DEFAULT '{}',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_users_org ON users(org_id);
CREATE INDEX idx_users_email ON users(email);
CREATE INDEX idx_users_invite_token ON users(invite_token) WHERE invite_token IS NOT NULL;

-- Domains
CREATE TABLE domains (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  domain TEXT NOT NULL UNIQUE,
  resend_domain_id TEXT,
  status domain_status NOT NULL DEFAULT 'pending',
  mx_verified BOOLEAN NOT NULL DEFAULT false,
  spf_verified BOOLEAN NOT NULL DEFAULT false,
  dkim_verified BOOLEAN NOT NULL DEFAULT false,
  catch_all_enabled BOOLEAN NOT NULL DEFAULT false,
  display_order INT NOT NULL DEFAULT 0,
  dns_records JSONB,
  verified_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_domains_org ON domains(org_id);

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
  starred BOOLEAN NOT NULL DEFAULT false,
  folder folder NOT NULL DEFAULT 'inbox',
  original_to TEXT,
  trash_expires_at TIMESTAMPTZ,
  deleted_at TIMESTAMPTZ,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_threads_user_domain_folder ON threads(user_id, domain_id, folder);
CREATE INDEX idx_threads_domain_folder ON threads(domain_id, folder);
CREATE INDEX idx_threads_last_message ON threads(last_message_at DESC);

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
  in_reply_to TEXT,
  references_header JSONB DEFAULT '[]',
  delivered_via_alias TEXT,
  sent_as_alias TEXT,
  spam_score FLOAT,
  spam_reasons JSONB,
  search_vector TSVECTOR,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_emails_thread ON emails(thread_id);
CREATE INDEX idx_emails_user ON emails(user_id);
CREATE INDEX idx_emails_domain ON emails(domain_id);
CREATE INDEX idx_emails_message_id ON emails(message_id);
CREATE INDEX idx_emails_resend_id ON emails(resend_email_id) WHERE resend_email_id IS NOT NULL;
CREATE INDEX idx_emails_search ON emails USING GIN(search_vector);

-- Aliases
CREATE TABLE aliases (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id UUID NOT NULL REFERENCES orgs(id),
  domain_id UUID NOT NULL REFERENCES domains(id),
  address TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_aliases_org ON aliases(org_id);
CREATE INDEX idx_aliases_domain ON aliases(domain_id);

-- Alias Users (junction)
CREATE TABLE alias_users (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  alias_id UUID NOT NULL REFERENCES aliases(id) ON DELETE CASCADE,
  user_id UUID NOT NULL REFERENCES users(id),
  can_send_as BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  UNIQUE(alias_id, user_id)
);

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

-- +goose Down
DROP TRIGGER IF EXISTS emails_search_update ON emails;
DROP FUNCTION IF EXISTS emails_search_trigger();
DROP TABLE IF EXISTS discovered_addresses;
DROP TABLE IF EXISTS alias_users;
DROP TABLE IF EXISTS aliases;
DROP TABLE IF EXISTS emails;
DROP TABLE IF EXISTS threads;
DROP TABLE IF EXISTS domains;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS orgs;
DROP TYPE IF EXISTS address_type;
DROP TYPE IF EXISTS email_status;
DROP TYPE IF EXISTS email_direction;
DROP TYPE IF EXISTS folder;
DROP TYPE IF EXISTS domain_status;
DROP TYPE IF EXISTS user_status;
DROP TYPE IF EXISTS user_role;
