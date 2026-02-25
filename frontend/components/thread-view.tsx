"use client";

import { useState, useEffect, useRef } from "react";
import DOMPurify from "dompurify";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";
import { useDomains } from "@/contexts/domain-context";
import { useEmailWindow } from "@/contexts/email-window-context";
import { useThread, useStarThread, useThreadAction } from "@/hooks/use-threads";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/query-keys";
import { Button } from "@/components/ui/button";
import { Spinner } from "@/components/ui/spinner";
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
} from "lucide-react";
import { ContactCard } from "@/components/contact-card";
import type { Thread, Email } from "@/lib/types";

interface ThreadViewProps {
  threadId: string;
  domainId: string;
  folder?: string;
  onBack?: () => void;
}

function formatSenderName(email: string): string {
  const local = email.split("@")[0] || email;
  return local
    .replace(/[._-]/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

export function ThreadView({
  threadId,
  domainId,
  folder,
  onBack,
}: ThreadViewProps) {
  const { data: thread, isLoading } = useThread(threadId);
  const { activeDomain } = useDomains();
  const { openCompose } = useEmailWindow();
  const qc = useQueryClient();
  const lastMessageRef = useRef<HTMLDivElement>(null);
  const markedReadRef = useRef<string | null>(null);

  const starMutation = useStarThread();
  const actionMutation = useThreadAction();

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
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
      api.patch(`/api/threads/${threadId}/read`);
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
    starMutation.mutate(threadId);
  }

  function handleAction(action: string) {
    actionMutation.mutate({ threadId, action });
    const navigateAway = ["archive", "trash", "spam", "delete", "unread"];
    if (navigateAway.includes(action) || action.startsWith("move:")) {
      onBack?.();
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-2 md:gap-3 pl-14 pr-4 md:px-6 py-3 border-b shrink-0">
        {onBack && (
          <button onClick={onBack} className="text-muted-foreground hover:text-foreground">
            <ArrowLeft className="h-5 w-5" />
          </button>
        )}
        <h2 className="flex-1 font-semibold text-lg truncate">
          {thread.subject}
        </h2>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="icon"
            onClick={handleStar}
            title={thread.starred ? "Unstar" : "Star"}
          >
            <Star
              className={`h-4 w-4 ${thread.starred ? "text-yellow-500 fill-yellow-500" : ""}`}
            />
          </Button>
          <Button
            variant="ghost"
            size="icon"
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
          {(folder === "trash" || folder === "archive" || folder === "spam") ? (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => handleAction("move:inbox")}
              title="Move to Inbox"
            >
              <Inbox className="h-4 w-4" />
            </Button>
          ) : (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => handleAction("archive")}
              title="Archive"
            >
              <Archive className="h-4 w-4" />
            </Button>
          )}
          {/* Report Spam — not on sent or spam */}
          {folder !== "sent" && folder !== "spam" && (
            <Button
              variant="ghost"
              size="icon"
              onClick={() => handleAction("spam")}
              title="Report spam"
            >
              <AlertTriangle className="h-4 w-4" />
            </Button>
          )}
          {/* Destructive: Trash or Delete permanently */}
          {folder === "trash" ? (
            <Button
              variant="ghost"
              size="icon"
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
              onClick={() => handleAction("trash")}
              title="Trash"
            >
              <Trash2 className="h-4 w-4" />
            </Button>
          )}
        </div>
      </div>

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

// ─── Email Message ─────────────────────────────────────────────────────

function EmailMessage({
  email,
  defaultExpanded = true,
  onReply,
}: {
  email: Email;
  defaultExpanded?: boolean;
  onReply?: (email: Email, mode: "reply" | "replyAll") => void;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  const sanitizedHtml = email.body_html
    ? DOMPurify.sanitize(email.body_html, {
        ALLOWED_TAGS: [
          "p", "br", "strong", "em", "u", "a", "ul", "ol", "li",
          "h1", "h2", "h3", "h4", "blockquote", "pre", "code",
          "img", "table", "thead", "tbody", "tr", "td", "th",
          "div", "span",
        ],
        ALLOWED_ATTR: [
          "href", "src", "alt", "style", "class", "target", "width", "height",
        ],
      })
    : null;

  if (!expanded) {
    // Collapsed: thin Gmail-style row — just sender, snippet, time
    return (
      <button
        onClick={() => setExpanded(true)}
        className="flex items-center gap-3 w-full px-4 py-2 text-left rounded-lg hover:bg-muted/50 transition-colors"
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
      >
        <div className="h-8 w-8 rounded-full bg-primary/10 text-primary flex items-center justify-center text-xs font-semibold shrink-0 self-center">
          {email.from_address.charAt(0).toUpperCase()}
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
        </div>
        {sanitizedHtml ? (
          <div
            className="max-w-2xl text-[13px] leading-relaxed [&_p]:my-1.5 [&_ul]:my-1.5 [&_ol]:my-1.5 [&_blockquote]:border-l-2 [&_blockquote]:border-muted [&_blockquote]:pl-3 [&_blockquote]:text-muted-foreground [&_a]:text-primary [&_a]:underline [&_img]:max-w-full [&_img]:h-auto [&_pre]:bg-muted [&_pre]:p-2 [&_pre]:rounded [&_pre]:text-xs [&_pre]:overflow-x-auto"
            dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
          />
        ) : (
          <pre className="text-[13px] whitespace-pre-wrap font-sans leading-relaxed max-w-2xl">
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
          </div>
        )}
      </div>
    </div>
  );
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
  // No outbound emails yet — use the address the last inbound was sent to
  for (let i = emails.length - 1; i >= 0; i--) {
    if (emails[i].direction === "inbound") {
      const toAddrs = parseAddresses(emails[i].to_addresses);
      const match = toAddrs.find((a) => a.endsWith(`@${domainName}`));
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

  if (lastEmail.direction === "outbound") {
    // Replying to our own sent email — keep same recipients
    if (mode === "replyAll") {
      const allCc = ccAddresses.filter((a) => a !== myAddress);
      return { defaultTo: toAddresses, defaultCc: allCc };
    }
    return { defaultTo: toAddresses, defaultCc: [] };
  }

  // Inbound — reply to sender
  if (mode === "replyAll") {
    const allRecipients = [...toAddresses, ...ccAddresses].filter(
      (a) => a !== myAddress && a !== lastEmail.from_address
    );
    return { defaultTo: [lastEmail.from_address], defaultCc: allRecipients };
  }
  return { defaultTo: [lastEmail.from_address], defaultCc: [] };
}

function getReplySubject(subject: string, mode: "reply" | "replyAll" | "forward"): string {
  const stripped = subject.replace(/^(Re|Fwd|Fw):\s*/i, "");
  if (mode === "forward") {
    return `Fwd: ${stripped}`;
  }
  return `Re: ${stripped}`;
}

function getQuotedHtml(email: Email, mode: "reply" | "replyAll" | "forward"): string {
  const date = new Date(email.created_at).toLocaleString();
  const sender = email.from_address;
  const body = email.body_html || email.body_plain || "";

  if (mode === "forward") {
    const toAddresses = parseAddresses(email.to_addresses).join(", ");
    return `<div style="margin-top:16px;padding-top:16px;border-top:1px solid #ccc">
      <p style="color:#666;font-size:12px">---------- Forwarded message ----------<br>
      From: ${sender}<br>
      Date: ${date}<br>
      Subject: ${email.subject}<br>
      To: ${toAddresses}</p>
      <div>${body}</div>
    </div>`;
  }

  return `<div style="margin-top:16px;padding-top:16px;border-top:1px solid #ccc">
    <p style="color:#666;font-size:12px">On ${date}, ${sender} wrote:</p>
    <blockquote style="margin:0;padding-left:8px;border-left:2px solid #ccc;color:#666">${body}</blockquote>
  </div>`;
}
