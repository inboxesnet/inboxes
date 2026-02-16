export interface User {
  id: string;
  org_id: string;
  email: string;
  name: string;
  role: "admin" | "member";
  status: "placeholder" | "invited" | "active" | "disabled";
  created_at: string;
}

export interface Org {
  id: string;
  name: string;
  onboarding_completed: boolean;
}

export interface Domain {
  id: string;
  org_id: string;
  domain: string;
  resend_domain_id: string;
  status: "pending" | "verified" | "active";
  mx_verified: boolean;
  spf_verified: boolean;
  dkim_verified: boolean;
  catch_all_enabled: boolean;
  display_order: number;
  dns_records: unknown;
  hidden: boolean;
  verified_at: string | null;
  created_at: string;
}

export interface Thread {
  id: string;
  org_id: string;
  user_id: string;
  domain_id: string;
  subject: string;
  participant_emails: string[];
  last_message_at: string;
  message_count: number;
  unread_count: number;
  starred: boolean;
  folder: Folder;
  snippet: string;
  original_to: string;
  created_at: string;
  emails?: Email[];
}

export interface ThreadListResponse {
  threads: Thread[];
  page: number;
  total: number;
}

export type Folder = "inbox" | "sent" | "drafts" | "archive" | "trash" | "spam";

export interface Email {
  id: string;
  thread_id: string;
  user_id: string;
  org_id: string;
  domain_id: string;
  resend_email_id: string;
  message_id: string;
  direction: "inbound" | "outbound";
  from_address: string;
  to_addresses: string[];
  cc_addresses: string[];
  bcc_addresses: string[];
  subject: string;
  body_html: string;
  body_plain: string;
  status: "received" | "sent" | "delivered" | "bounced" | "failed";
  attachments: Attachment[];
  delivered_via_alias: string;
  sent_as_alias: string;
  spam_score: number;
  created_at: string;
}

export interface Attachment {
  filename: string;
  content_type: string;
  size: number;
  url: string;
}

export interface Alias {
  id: string;
  org_id: string;
  domain_id: string;
  address: string;
  name: string;
  users?: AliasUser[];
  created_at: string;
}

export interface AliasUser {
  id: string;
  alias_id: string;
  user_id: string;
  can_send_as: boolean;
  user?: User;
}

export interface DiscoveredAddress {
  id: string;
  domain_id: string;
  address: string;
  local_part: string;
  type: "unclaimed" | "user" | "alias";
  email_count: number;
}

export interface Contact {
  email: string;
  name: string;
  count: number;
}

export interface UnreadCounts {
  [domainId: string]: number;
}

export interface Draft {
  id: string;
  domain_id: string;
  thread_id?: string;
  kind: "compose" | "reply" | "forward";
  subject: string;
  from_address: string;
  to_addresses: string[];
  cc_addresses: string[];
  bcc_addresses: string[];
  body_html: string;
  body_plain: string;
  created_at: string;
  updated_at: string;
}

export interface WSMessage {
  event: string;
  thread_id?: string;
  domain_id?: string;
  payload?: Record<string, unknown>;
}
