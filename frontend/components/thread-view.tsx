"use client";

import { useState, useEffect, useRef } from "react";
import DOMPurify from "dompurify";
import { sanitizeLinkNode } from "@/lib/sanitize-links";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";
import { useDomains } from "@/contexts/domain-context";
import { useEmailWindow } from "@/contexts/email-window-context";
import { usePreferences } from "@/contexts/preferences-context";
import { useThread, useStarThread, useMuteThread, useThreadAction } from "@/hooks/use-threads";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/query-keys";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
import { ConfirmDialog } from "@/components/ui/confirm-dialog";
import {
  AlertTriangle,
  Archive,
  Trash2,
  Star,
  MailOpen,
  Mail,
  Reply,
  ReplyAll,
  Forward,
  ArrowLeft,
  Inbox,
  BellOff,
  Bell,
} from "lucide-react";
import { ContactCard } from "@/components/contact-card";
import { hasLabel } from "@/lib/types";
import type { Thread, Email } from "@/lib/types";

interface ThreadViewProps {
  threadId: string;
  domainId: string;
  label?: string;
  onBack?: () => void;
}

function formatSenderName(email: string): string {
  const local = email.split("@")[0] || email;
  return local
    .replace(/[._-]/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

function TrashCountdown({ expiresAt }: { expiresAt: string }) {
  const [days, setDays] = useState(() => Math.max(0, Math.ceil((new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24))));
  useEffect(() => {
    const timer = setInterval(() => {
      setDays(Math.max(0, Math.ceil((new Date(expiresAt).getTime() - Date.now()) / (1000 * 60 * 60 * 24))));
    }, 60 * 60 * 1000); // Update every hour
    return () => clearInterval(timer);
  }, [expiresAt]);

  return (
    <div className="bg-destructive/10 text-destructive text-xs text-center py-1.5 px-4 border-b shrink-0">
      {days > 0
        ? `This conversation will be permanently deleted in ${days} day${days !== 1 ? "s" : ""}`
        : "This conversation is scheduled for deletion"}
    </div>
  );
}

export function ThreadView({
  threadId,
  domainId,
  label,
  onBack,
}: ThreadViewProps) {
  const { data: thread, isLoading } = useThread(threadId);
  const { activeDomain } = useDomains();
  const { openCompose } = useEmailWindow();
  const { stripTrackingParams } = usePreferences();
  const qc = useQueryClient();
  const lastMessageRef = useRef<HTMLDivElement>(null);
  const markedReadRef = useRef<string | null>(null);

  const starMutation = useStarThread();
  const muteMutation = useMuteThread();
  const actionMutation = useThreadAction();
  const isBusy = starMutation.isPending || muteMutation.isPending || actionMutation.isPending;
  const [confirmDeleteOpen, setConfirmDeleteOpen] = useState(false);

  // Mark as read on first load — optimistically update cache so list reflects immediately
  useEffect(() => {
    if (thread && thread.unread_count > 0 && markedReadRef.current !== threadId) {
      markedReadRef.current = threadId;
      // Optimistic: update thread list cache before API responds
      qc.setQueriesData<{ threads: Array<{ id: string; unread_count: number }>; total: number }>(
        { queryKey: queryKeys.threads.lists() },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            threads: old.threads.map((t) =>
              t.id === threadId ? { ...t, unread_count: 0 } : t
            ),
          };
        }
      );
      // Optimistic: update thread detail cache too
      qc.setQueryData<Thread>(
        queryKeys.threads.detail(threadId),
        (old) => (old ? { ...old, unread_count: 0 } : old)
      );
      // Optimistic: decrement domain unread count immediately
      qc.setQueryData<Record<string, number>>(queryKeys.domains.unreadCounts(), (old) => {
        if (!old) return old;
        const current = old[domainId] || 0;
        return { ...old, [domainId]: Math.max(0, current - 1) };
      });
      api.patch(`/api/threads/${threadId}/read`)
        .then(() => qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() }))
        .catch(() => toast.error("Failed to mark as read"));
    }
  }, [thread, threadId, qc]);

  // Reset state when threadId changes
  useEffect(() => {
    markedReadRef.current = null;
  }, [threadId]);

  // Scroll to the last message when thread loads
  useEffect(() => {
    if (thread?.emails?.length) {
      // Small delay to let DOM render
      requestAnimationFrame(() => {
        lastMessageRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
      });
    }
  }, [threadId, thread?.emails?.length]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  if (!thread) {
    return (
      <div className="flex items-center justify-center h-full text-muted-foreground">
        Thread not found
      </div>
    );
  }

  const emails = thread.emails || [];
  const lastEmail = emails[emails.length - 1];

  function handleReply(email: Email, mode: "reply" | "replyAll" | "forward") {
    if (!thread) return;
    const domainName = activeDomain?.domain || "";
    const fromAddress = getReplyFromAddress(emails, domainName);
    const { defaultTo, defaultCc } = getReplyRecipients(email, mode, fromAddress);
    const subject = getReplySubject(thread.subject, mode);
    const quotedHtml = getQuotedHtml(email, mode);

    openCompose({
      fromAddress,
      toAddresses: defaultTo,
      ccAddresses: defaultCc,
      subject,
      quotedHtml,
      replyToThreadId: thread.id,
      inReplyTo: email.message_id || undefined,
      references: email.message_id
        ? [...(email.references || []), email.message_id]
        : undefined,
    });
  }

  function handleStar() {
    if (!thread) return;
    starMutation.mutate({ threadId, starred: !hasLabel(thread, "starred") });
  }

  function handleMute() {
    muteMutation.mutate(threadId);
  }

  function handleAction(action: string) {
    if (action === "delete") {
      setConfirmDeleteOpen(true);
      return;
    }
    actionMutation.mutate({ threadId, action });
    const navigateAway = ["archive", "trash", "spam", "unread"];
    if (navigateAway.includes(action) || action.startsWith("move:")) {
      onBack?.();
    }
  }

  function handleConfirmDelete() {
    actionMutation.mutate({ threadId, action: "delete" });
    onBack?.();
  }

  return (
    <div className="flex flex-col h-full">
      <ConfirmDialog
        open={confirmDeleteOpen}
        onOpenChange={setConfirmDeleteOpen}
        title="Permanently delete this conversation?"
        description="This action cannot be undone."
        confirmLabel="Delete"
        onConfirm={handleConfirmDelete}
        destructive
      />
      {/* Header */}
      <div className="h-14 flex items-center gap-2 md:gap-3 pl-14 pr-4 md:px-6 border-b shrink-0 relative z-[25] bg-background">
        {onBack && (
          <button onClick={onBack} className="text-muted-foreground hover:text-foreground">
            <ArrowLeft className="h-5 w-5" />
          </button>
        )}
        <h2 className="flex-1 font-semibold text-sm truncate">
          {thread.subject}
        </h2>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            onClick={handleStar}
            disabled={isBusy}
            title={hasLabel(thread, "starred") ? "Unstar" : "Star"}
          >
            <Star
              className={`h-4 w-4 ${hasLabel(thread, "starred") ? "text-yellow-500 fill-yellow-500" : ""}`}
            />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={handleMute}
            disabled={isBusy}
            title={hasLabel(thread, "muted") ? "Unmute" : "Mute"}
          >
            {hasLabel(thread, "muted") ? (
              <BellOff className="h-4 w-4 text-muted-foreground" />
            ) : (
              <Bell className="h-4 w-4" />
            )}
          </Button>
          <Button
            variant="ghost"
            size="icon"
            disabled={isBusy}
            onClick={() =>
              handleAction(thread.unread_count > 0 ? "read" : "unread")
            }
            title={thread.unread_count > 0 ? "Mark read" : "Mark unread"}
          >
            {thread.unread_count > 0 ? (
              <MailOpen className="h-4 w-4" />
            ) : (
              <Mail className="h-4 w-4" />
            )}
          </Button>
          {/* Primary: Archive or Move to Inbox */}
          {(label === "trash" || label === "archive" || label === "spam") ? (
            <Button
              variant="ghost"
              size="icon"
              disabled={isBusy}
              onClick={() => handleAction("move:inbox")}
              title="Move to Inbox"
            >
              <Inbox className="h-4 w-4" />
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="icon"
              disabled={isBusy}
              onClick={() => handleAction("archive")}
              title="Archive"
            >
              <Archive className="h-4 w-4" />
            </Button>
          )}
          {/* Report Spam — not on sent or spam */}
          {label !== "sent" && label !== "spam" && (
            <Button
              variant="ghost"
              size="icon"
              disabled={isBusy}
              onClick={() => handleAction("spam")}
              title="Report spam"
            >
              <AlertTriangle className="h-4 w-4" />
            </Button>
          )}
          {/* Destructive: Trash or Delete permanently */}
          {label === "trash" ? (
            <Button
              variant="ghost"
              size="icon"
              disabled={isBusy}
              onClick={() => handleAction("delete")}
              title="Delete permanently"
              className="text-destructive hover:text-destructive"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="icon"
              disabled={isBusy}
              onClick={() => handleAction("trash")}
              title="Trash"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

      {/* Trash countdown banner */}
      {thread.trash_expires_at && <TrashCountdown expiresAt={thread.trash_expires_at} />}

      {/* Emails — Gmail style: all visible, older collapsed, newest expanded */}
      <div className="flex-1 overflow-y-auto px-4 md:px-6 py-4 space-y-2">
        {emails.map((email, i) => {
          const isLast = i === emails.length - 1;
          return (
            <div key={email.id} ref={isLast ? lastMessageRef : undefined}>
              <EmailMessage
                email={email}
                defaultExpanded={isLast}
                onReply={(em, mode) => handleReply(em, mode)}
                stripTracking={stripTrackingParams}
              />
            </div>
          );
        })}
      </div>

      {/* Reply bar */}
      <div className="border-t px-4 md:px-6 py-3 shrink-0 flex items-center gap-2">
        <Button
          variant="outline"
          size="sm"
          className="rounded-full h-8 px-4 text-xs"
          onClick={() => lastEmail && handleReply(lastEmail, "reply")}
        >
          <Reply className="h-3 w-3 mr-1.5" />
          Reply
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="rounded-full h-8 px-4 text-xs"
          onClick={() => lastEmail && handleReply(lastEmail, "replyAll")}
        >
          <ReplyAll className="h-3 w-3 mr-1.5" />
          Reply All
        </Button>
        <Button
          variant="outline"
          size="sm"
          className="rounded-full h-8 px-4 text-xs"
          onClick={() => lastEmail && handleReply(lastEmail, "forward")}
        >
          <Forward className="h-3 w-3 mr-1.5" />
          Forward
        </Button>
      </div>
    </div>
  );
}

// ─── Email Sanitization ─────────────────────────────────────────────────

const ALLOWED_CSS_PROPERTIES = new Set([
  'color', 'background-color', 'font-size', 'font-weight',
  'font-family', 'font-style', 'text-align', 'text-decoration',
  'line-height', 'letter-spacing', 'word-spacing',
  'margin', 'margin-top', 'margin-right', 'margin-bottom', 'margin-left',
  'padding', 'padding-top', 'padding-right', 'padding-bottom', 'padding-left',
  'border', 'border-radius', 'border-color', 'border-style', 'border-width',
  'border-top', 'border-right', 'border-bottom', 'border-left',
  'width', 'height', 'max-width', 'max-height', 'min-width', 'min-height',
  'display', 'vertical-align', 'list-style-type', 'white-space',
  'overflow', 'text-overflow', 'word-break',
  'table-layout', 'border-collapse', 'border-spacing',
]);

function sanitizeEmailHtml(html: string, showImages: boolean, stripTracking = true) {
  let hasBlockedImages = false;

  DOMPurify.addHook('afterSanitizeAttributes', (node) => {
    // PRD-013: Block external images (tracking pixels)
    if (node.tagName === 'IMG') {
      const src = node.getAttribute('src');
      if (src && !src.startsWith('data:') && !src.startsWith('cid:')) {
        hasBlockedImages = true;
        if (!showImages) {
          node.setAttribute('data-original-src', src);
          node.removeAttribute('src');
          node.setAttribute('alt', '[Image blocked for privacy]');
        }
      }
    }

    // Open links in new tab + optionally strip tracking params
    sanitizeLinkNode(node, stripTracking);

    // PRD-014: Sanitize CSS against allowlist
    if (node.hasAttribute('style')) {
      const style = (node as HTMLElement).style;
      const safeStyles: string[] = [];
      for (let i = 0; i < style.length; i++) {
        const prop = style[i];
        if (ALLOWED_CSS_PROPERTIES.has(prop)) {
          const value = style.getPropertyValue(prop);
          if (!value.includes('url(')) {
            safeStyles.push(`${prop}: ${value}`);
          }
        }
      }
      if (safeStyles.length > 0) {
        node.setAttribute('style', safeStyles.join('; '));
      } else {
        node.removeAttribute('style');
      }
    }
  });

  const result = DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      "p", "br", "strong", "em", "u", "a", "ul", "ol", "li",
      "h1", "h2", "h3", "h4", "blockquote", "pre", "code",
      "img", "table", "thead", "tbody", "tr", "td", "th",
      "div", "span",
    ],
    ALLOWED_ATTR: [
      "href", "src", "alt", "style", "class", "target", "rel", "width", "height",
      "data-original-src", "dir",
    ],
  });

  DOMPurify.removeAllHooks();
  return { html: result, hasBlockedImages };
}

// ─── Email Message ─────────────────────────────────────────────────────

function EmailMessage({
  email,
  defaultExpanded = true,
  onReply,
  stripTracking = true,
}: {
  email: Email;
  defaultExpanded?: boolean;
  onReply?: (email: Email, mode: "reply" | "replyAll" | "forward") => void;
  stripTracking?: boolean;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const [showImages, setShowImages] = useState(false);

  const sanitized = email.body_html
    ? sanitizeEmailHtml(email.body_html, showImages, stripTracking)
    : null;

  if (!expanded) {
    // Collapsed: thin Gmail-style row — just sender, snippet, time
    return (
      <button
        onClick={() => setExpanded(true)}
        className="flex items-center gap-3 w-full px-4 py-2 text-left rounded-lg hover:bg-muted/50 transition-colors"
        aria-expanded={false}
        aria-label={`Expand email from ${formatSenderName(email.from_address)}`}
      >
        <div className="h-7 w-7 rounded-full bg-primary/10 text-primary flex items-center justify-center text-xs font-semibold shrink-0">
          {email.from_address.charAt(0).toUpperCase()}
        </div>
        <span className="text-[13px] font-semibold shrink-0">
          {formatSenderName(email.from_address)}
        </span>
        <span className="text-[13px] text-muted-foreground/70 truncate flex-1 min-w-0">
          {email.body_plain?.slice(0, 100)}
        </span>
        <span className="text-xs text-muted-foreground shrink-0">
          {formatRelativeTime(email.created_at)}
        </span>
      </button>
    );
  }

  // Expanded: full email card
  return (
    <div className="rounded-lg border">
      <button
        onClick={() => setExpanded(false)}
        className="flex items-baseline gap-3 w-full px-4 py-2.5 text-left"
        aria-expanded={true}
        aria-label={`Collapse email from ${formatSenderName(email.from_address)}`}
      >
        <div className="relative h-8 w-8 rounded-full bg-primary/10 text-primary flex items-center justify-center text-xs font-semibold shrink-0 self-center">
          {email.from_address.charAt(0).toUpperCase()}
          {email.is_read === false && (
            <span className="absolute -top-0.5 -right-0.5 h-2.5 w-2.5 rounded-full bg-primary border-2 border-background" />
          )}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-baseline gap-2">
            <ContactCard email={email.from_address}>
              <span className="text-[13px] font-semibold truncate">
                {formatSenderName(email.from_address)}
              </span>
            </ContactCard>
            <span className="hidden md:inline text-xs text-muted-foreground truncate">
              {email.from_address}
            </span>
            {email.direction === "outbound" && (
              <StatusBadge status={email.status} />
            )}
            <span className="text-xs text-muted-foreground shrink-0 ml-auto">
              {formatRelativeTime(email.created_at)}
            </span>
          </div>
        </div>
      </button>

      <div className="px-4 pb-4">
        <div className="text-xs text-muted-foreground mb-3 space-y-0.5">
          <p>To: {parseAddresses(email.to_addresses).join(", ")}</p>
          {parseAddresses(email.cc_addresses).length > 0 && (
            <p>Cc: {parseAddresses(email.cc_addresses).join(", ")}</p>
          )}
          <BccLine email={email} />
        </div>
        {sanitized?.html ? (
          <>
            {sanitized.hasBlockedImages && !showImages && (
              <button
                onClick={() => setShowImages(true)}
                className="w-full text-xs text-muted-foreground bg-muted/50 rounded-md px-3 py-1.5 mb-2 hover:bg-muted transition-colors text-left"
              >
                Images are hidden for your privacy. Click to load.
              </button>
            )}
            <div style={{ position: 'relative', overflow: 'hidden' }}>
              <div
                dir="auto"
                className="max-w-2xl text-[13px] leading-relaxed dark:bg-white dark:text-black dark:rounded-md dark:p-3 dark:-mx-3 [&_p]:my-1.5 [&_ul]:my-1.5 [&_ol]:my-1.5 [&_blockquote]:border-l-2 [&_blockquote]:border-muted [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_a]:text-primary dark:[&_a]:text-blue-600 [&_a]:underline [&_img]:max-w-full [&_img]:h-auto [&_pre]:bg-muted [&_pre]:p-2 [&_pre]:rounded [&_pre]:text-xs [&_pre]:overflow-x-auto"
                dangerouslySetInnerHTML={{ __html: sanitized.html }}
              />
            </div>
          </>
        ) : (
          <pre dir="auto" className="text-[13px] whitespace-pre-wrap font-sans leading-relaxed max-w-2xl dark:bg-white dark:text-black dark:rounded-md dark:p-3 dark:-mx-3">
            {email.body_plain}
          </pre>
        )}
        {email.attachments && email.attachments.length > 0 && (
          <div className="mt-3 flex flex-wrap gap-2">
            {email.attachments.map((att, i) => (
              <a
                key={i}
                href={att.url}
                className="flex items-center gap-1.5 text-xs border rounded-md px-2 py-1 hover:bg-muted"
                download
              >
                {att.filename}
                <span className="text-muted-foreground">
                  ({Math.round(att.size / 1024)}KB)
                </span>
              </a>
            ))}
          </div>
        )}
        {onReply && (
          <div className="mt-3 flex items-center gap-2 pt-2 border-t">
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onReply(email, "reply")}
              className="h-7 text-xs"
            >
              <Reply className="h-3 w-3 mr-1" />
              Reply
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onReply(email, "replyAll")}
              className="h-7 text-xs"
            >
              <ReplyAll className="h-3 w-3 mr-1" />
              Reply All
            </Button>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onReply(email, "forward")}
              className="h-7 text-xs"
            >
              <Forward className="h-3 w-3 mr-1" />
              Forward
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}

// ─── Status Badge ──────────────────────────────────────────────────────

function StatusBadge({ status }: { status: string }) {
  const config: Record<string, { color: string; label: string }> = {
    delivered: { color: "bg-green-500", label: "Delivered" },
    sent: { color: "bg-yellow-500", label: "Sent" },
    queued: { color: "bg-yellow-500", label: "Queued" },
    bounced: { color: "bg-red-500", label: "Bounced" },
    failed: { color: "bg-red-500", label: "Failed" },
  };
  const c = config[status] || { color: "bg-gray-400", label: status };
  return (
    <span className="inline-flex items-center gap-1 text-[10px] text-muted-foreground shrink-0">
      <span className={`h-1.5 w-1.5 rounded-full ${c.color}`} />
      {c.label}
    </span>
  );
}

// ─── BCC Display ────────────────────────────────────────────────────────

function BccLine({ email }: { email: Email }) {
  const bccAddresses = parseAddresses(email.bcc_addresses);
  if (bccAddresses.length === 0) return null;

  // Outbound: you sent it — show full BCC list
  if (email.direction === "outbound") {
    return <p>Bcc: {bccAddresses.join(", ")}</p>;
  }

  // Inbound: check if you were BCC'd (your address is in BCC but not in TO/CC)
  const toAddresses = parseAddresses(email.to_addresses);
  const ccAddresses = parseAddresses(email.cc_addresses);
  const { activeDomain } = useDomains();
  const domainName = activeDomain?.domain || "";

  const isBccRecipient = bccAddresses.some(
    (a) =>
      a.endsWith(`@${domainName}`) &&
      !toAddresses.includes(a) &&
      !ccAddresses.includes(a)
  );

  if (isBccRecipient) {
    return <p>Bcc: you</p>;
  }

  return null;
}

// ─── Helpers ───────────────────────────────────────────────────────────

function parseAddresses(raw: string[] | string): string[] {
  if (Array.isArray(raw)) return raw;
  if (typeof raw === "string") {
    try { return JSON.parse(raw); } catch { return []; }
  }
  return [];
}

function getReplyFromAddress(emails: Email[], domainName: string): string {
  // Find the last outbound email's from address
  for (let i = emails.length - 1; i >= 0; i--) {
    if (emails[i].direction === "outbound") {
      return emails[i].from_address;
    }
  }
  // No outbound emails yet — use the address the last inbound was sent to (check TO, CC, BCC)
  for (let i = emails.length - 1; i >= 0; i--) {
    if (emails[i].direction === "inbound") {
      const allAddrs = [
        ...parseAddresses(emails[i].to_addresses),
        ...parseAddresses(emails[i].cc_addresses),
        ...parseAddresses(emails[i].bcc_addresses),
      ];
      const match = allAddrs.find((a) => a.endsWith(`@${domainName}`));
      if (match) return match;
    }
  }
  return `hello@${domainName}`;
}

function getReplyRecipients(
  lastEmail: Email,
  mode: "reply" | "replyAll" | "forward",
  myAddress: string
): { defaultTo: string[]; defaultCc: string[] } {
  if (mode === "forward") {
    return { defaultTo: [], defaultCc: [] };
  }

  const toAddresses = parseAddresses(lastEmail.to_addresses);
  const ccAddresses = parseAddresses(lastEmail.cc_addresses);
  const bccAddresses = parseAddresses(lastEmail.bcc_addresses);

  // If I was BCC'd, reply only to sender — never expose other recipients
  const isBccRecipient =
    lastEmail.direction === "inbound" &&
    bccAddresses.includes(myAddress);

  if (isBccRecipient) {
    const replyTarget =
      lastEmail.reply_to_addresses?.length
        ? lastEmail.reply_to_addresses[0]
        : lastEmail.from_address;
    return { defaultTo: [replyTarget], defaultCc: [] };
  }

  if (lastEmail.direction === "outbound") {
    // Replying to our own sent email — keep same recipients
    if (mode === "replyAll") {
      const allCc = ccAddresses.filter((a) => a !== myAddress);
      return { defaultTo: toAddresses, defaultCc: allCc };
    }
    return { defaultTo: toAddresses, defaultCc: [] };
  }

  // Inbound — reply to Reply-To address if present, otherwise From
  const replyTarget =
    lastEmail.reply_to_addresses?.length
      ? lastEmail.reply_to_addresses[0]
      : lastEmail.from_address;

  if (mode === "replyAll") {
    const allRecipients = [...toAddresses, ...ccAddresses].filter(
      (a) => a !== myAddress && a !== replyTarget && a !== lastEmail.from_address
    );
    return { defaultTo: [replyTarget], defaultCc: allRecipients };
  }
  return { defaultTo: [replyTarget], defaultCc: [] };
}

function getReplySubject(subject: string, mode: "reply" | "replyAll" | "forward"): string {
  const stripped = subject.replace(/^(Re|Fwd|Fw):\s*/i, "");
  if (mode === "forward") {
    return `Fwd: ${stripped}`;
  }
  return `Re: ${stripped}`;
}

function escapeHtml(str: string): string {
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

function getQuotedHtml(email: Email, mode: "reply" | "replyAll" | "forward"): string {
  const date = escapeHtml(new Date(email.created_at).toLocaleString());
  const sender = escapeHtml(email.from_address);
  const sanitizedBody = DOMPurify.sanitize(email.body_html || email.body_plain || "");

  if (mode === "forward") {
    const toAddresses = escapeHtml(parseAddresses(email.to_addresses).join(", "));
    const subject = escapeHtml(email.subject || "");
    return `<div style="margin-top:16px;padding-top:16px;border-top:1px solid #ccc">
      <p style="color:#666;font-size:12px">---------- Forwarded message ----------<br>
      From: ${sender}<br>
      Date: ${date}<br>
      Subject: ${subject}<br>
      To: ${toAddresses}</p>
      <div>${sanitizedBody}</div>
    </div>`;
  }

  return `<div style="margin-top:16px;padding-top:16px;border-top:1px solid #ccc">
    <p style="color:#666;font-size:12px">On ${date}, ${sender} wrote:</p>
    <blockquote style="margin:0;padding-left:8px;border-left:2px solid #ccc;color:#666">${sanitizedBody}</blockquote>
  </div>`;
}
