export interface User {
  id: string;
  org_id: string;
  email: string;
  name: string;
  role: "admin" | "member";
  status: "placeholder" | "invited" | "active" | "disabled";
  is_owner?: boolean;
  created_at: string;
  has_webhook?: boolean;
  signature_html?: string;
  undo_send_seconds?: number;
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
  status: "pending" | "verified" | "active" | "disconnected" | "deleted";
  display_order: number;
  dns_records: unknown;
  hidden: boolean;
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
  snoozed_until?: string;
  created_at: string;
  emails?: Email[];
}

export interface ThreadListResponse {
  threads: Thread[];
  page: number;
  total: number;
}

// Label type for view routing (URL segment, not thread data)
export type Label = "inbox" | "sent" | "drafts" | "archive" | "starred" | "snoozed" | "trash" | "spam" | "failed" | "deleted_forever";

export function hasLabel(thread: { labels?: string[] }, label: string): boolean {
  return thread.labels?.includes(label) ?? false;
}

// Views that the server computes from data that is not in the thread labels
// (failed sends, active snoozes). The client cannot decide membership from a
// thread summary; it must refetch these lists.
export function isVirtualView(viewLabel: string): boolean {
  return viewLabel === "failed" || viewLabel === "snoozed";
}

function isSnoozed(thread: { snoozed_until?: string | null }): boolean {
  return !!thread.snoozed_until && new Date(thread.snoozed_until).getTime() > Date.now();
}

// Do not call this for a virtual view; check isVirtualView first.
export function threadBelongsInView(
  thread: { labels?: string[]; snoozed_until?: string | null },
  viewLabel: string
): boolean {
  if (viewLabel === "trash" || viewLabel === "spam") return hasLabel(thread, viewLabel);
  if (viewLabel === "archive") return !hasLabel(thread, "inbox") && !hasLabel(thread, "trash") && !hasLabel(thread, "spam");
  // The server hides a snoozed thread from the inbox until it wakes.
  if (viewLabel === "inbox" && isSnoozed(thread)) return false;
  // inbox, sent, starred, custom labels:
  return hasLabel(thread, viewLabel) && !hasLabel(thread, "trash") && !hasLabel(thread, "spam");
}

/** Compute the labels of a thread after a bulk or single action. */
export function applyBulkLabels(
  labels: string[],
  action: string,
  label: string
): string[] {
  switch (action) {
    case "archive":
      return labels.filter((l) => l !== "inbox");
    case "trash":
      return [...labels.filter((l) => l !== "trash"), "trash"];
    case "spam":
      return [...labels.filter((l) => l !== "spam"), "spam"];
    case "move":
      switch (label) {
        case "inbox":
          return [
            ...labels.filter(
              (l) => l !== "inbox" && l !== "trash" && l !== "spam"
            ),
            "inbox",
          ];
        case "trash":
          return [...labels.filter((l) => l !== "trash"), "trash"];
        case "spam":
          // The server also restores the thread from trash.
          return [...labels.filter((l) => l !== "spam" && l !== "trash"), "spam"];
        case "archive":
          // The server also restores the thread from trash and spam.
          return labels.filter(
            (l) => l !== "inbox" && l !== "trash" && l !== "spam"
          );
        default:
          return labels.includes(label) ? labels : [...labels, label];
      }
    case "mute":
      return labels.includes("muted") ? labels : [...labels, "muted"];
    case "unmute":
      return labels.filter((l) => l !== "muted");
    case "label":
      return labels.includes(label) ? labels : [...labels, label];
    case "unlabel":
      return labels.filter((l) => l !== label);
    default:
      return labels;
  }
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
  dismissed_at?: string;
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
  in_reply_to?: string;
  references_header?: string;
  scheduled_send_at?: string;
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
  plan: "free" | "pro" | "past_due" | "cancelled";
  plan_expires_at: string | null;
  lapsed_at?: string | null;
  read_only?: boolean;
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
