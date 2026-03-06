export interface User {
  id: string;
  org_id: string;
  email: string;
  name: string;
  role: "admin" | "member";
  status: "placeholder" | "invited" | "active" | "disabled";
  is_owner?: boolean;
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
  status: "pending" | "not_started" | "verified" | "active";
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
  labels: string[];
  snippet: string;
  last_sender: string;
  original_to: string;
  trash_expires_at?: string;
  created_at: string;
  emails?: Email[];
}

export interface ThreadListResponse {
  threads: Thread[];
  page: number;
  total: number;
}

// Label type for view routing (URL segment, not thread data)
export type Label = "inbox" | "sent" | "drafts" | "archive" | "starred" | "trash" | "spam" | "deleted_forever";

export function hasLabel(thread: { labels?: string[] }, label: string): boolean {
  return thread.labels?.includes(label) ?? false;
}

export function threadBelongsInView(thread: { labels?: string[] }, viewLabel: string): boolean {
  if (viewLabel === "trash" || viewLabel === "spam") return hasLabel(thread, viewLabel);
  if (viewLabel === "archive") return !hasLabel(thread, "inbox") && !hasLabel(thread, "trash") && !hasLabel(thread, "spam");
  // inbox, sent, starred:
  return hasLabel(thread, viewLabel) && !hasLabel(thread, "trash") && !hasLabel(thread, "spam");
}

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
  reply_to_addresses?: string[];
  subject: string;
  body_html: string;
  body_plain: string;
  status: "received" | "sent" | "delivered" | "bounced" | "failed" | "queued" | "complained";
  attachments: Attachment[];
  in_reply_to?: string;
  references?: string[];
  delivered_via_alias: string;
  sent_as_alias: string;
  spam_score: number;
  is_read?: boolean;
  created_at: string;
}

export interface Attachment {
  id?: string;
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
  type: "unclaimed" | "individual" | "group";
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
  attachment_ids?: string[];
  created_at: string;
  updated_at: string;
}

export interface SyncJob {
  id: string;
  status: "pending" | "running" | "completed" | "failed";
  phase: string;
  imported: number;
  total: number;
  sent_count: number;
  received_count: number;
  thread_count: number;
  address_count: number;
  error_message?: string;
  already_active?: boolean;
  created_at: string;
  updated_at?: string;
}

export interface BillingInfo {
  plan: "free" | "pro" | "cancelled";
  plan_expires_at: string | null;
  billing_enabled: boolean;
  subscription?: {
    status: string;
    current_period_end: string;
    cancel_at_period_end: boolean;
  };
}

export interface WSMessage {
  id?: number;
  event: string;
  thread_id?: string;
  domain_id?: string;
  payload?: Record<string, unknown>;
}
