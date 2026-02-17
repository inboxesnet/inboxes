"use client";

import { useState, useEffect, useRef } from "react";
import DOMPurify from "dompurify";
import { api } from "@/lib/api";
import { formatRelativeTime } from "@/lib/utils";
import { useDomains } from "@/contexts/domain-context";
import { useThread, useStarThread, useThreadAction } from "@/hooks/use-threads";
import { useQueryClient } from "@tanstack/react-query";
import { queryKeys } from "@/lib/query-keys";
import { TipTapEditor } from "@/components/tiptap-editor";
import { RecipientInput } from "@/components/recipient-input";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import {
  Archive,
  Trash2,
  Star,
  MailOpen,
  Mail,
  Reply,
  ReplyAll,
  Forward,
  ArrowLeft,
  ChevronDown,
  Send,
  X,
} from "lucide-react";
import { ContactCard } from "@/components/contact-card";
import type { Thread, Email } from "@/lib/types";

interface ThreadViewProps {
  threadId: string;
  domainId: string;
  onBack?: () => void;
}

type ReplyMode = "reply" | "replyAll" | "forward" | null;

export function ThreadView({
  threadId,
  domainId,
  onBack,
}: ThreadViewProps) {
  const { data: thread, isLoading } = useThread(threadId);
  const { activeDomain } = useDomains();
  const qc = useQueryClient();
  const bottomRef = useRef<HTMLDivElement>(null);
  const markedReadRef = useRef<string | null>(null);
  const [showCollapsed, setShowCollapsed] = useState(false);
  const [replyMode, setReplyMode] = useState<ReplyMode>(null);
  const [replyToEmail, setReplyToEmail] = useState<Email | null>(null);

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
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
      api.patch(`/api/threads/${threadId}/read`);
    }
  }, [thread, threadId, qc]);

  // Reset state when threadId changes
  useEffect(() => {
    markedReadRef.current = null;
    setShowCollapsed(false);
    setReplyMode(null);
    setReplyToEmail(null);
  }, [threadId]);

  // Scroll to bottom when reply opens or thread loads
  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [thread?.emails?.length, replyMode]);

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
  const collapsedCount = emails.length > 1 ? emails.length - 1 : 0;
  const lastEmail = emails[emails.length - 1];
  const olderEmails = emails.slice(0, -1);

  function handleStar() {
    starMutation.mutate(threadId);
  }

  function handleAction(action: string) {
    actionMutation.mutate({ threadId, action });
    const navigateAway = ["archive", "trash", "spam", "unread"];
    if (navigateAway.includes(action)) {
      onBack?.();
    }
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <div className="flex items-center gap-3 px-6 py-3 border-b shrink-0">
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
          <Button
            variant="ghost"
            size="icon"
            onClick={() => handleAction("archive")}
            title="Archive"
          >
            <Archive className="h-4 w-4" />
          </Button>
          <Button
            variant="ghost"
            size="icon"
            onClick={() => handleAction("trash")}
            title="Trash"
          >
            <Trash2 className="h-4 w-4" />
          </Button>
        </div>
      </div>

      {/* Emails */}
      <div className="flex-1 overflow-y-auto px-6 py-4 space-y-4">
        {/* Collapsed older emails */}
        {collapsedCount > 0 && !showCollapsed && (
          <button
            onClick={() => setShowCollapsed(true)}
            className="flex items-center gap-2 text-sm text-muted-foreground hover:text-foreground w-full justify-center py-2 rounded-lg border border-dashed hover:border-solid transition-colors"
          >
            <ChevronDown className="h-3.5 w-3.5" />
            {collapsedCount} earlier message{collapsedCount > 1 ? "s" : ""}
          </button>
        )}

        {/* Show older emails if expanded */}
        {showCollapsed &&
          olderEmails.map((email) => (
            <EmailMessage
              key={email.id}
              email={email}
              defaultExpanded={false}
              onReply={(em, mode) => {
                setReplyToEmail(em);
                setReplyMode(mode);
              }}
            />
          ))}

        {/* Most recent email — always expanded */}
        {lastEmail && (
          <EmailMessage
            key={lastEmail.id}
            email={lastEmail}
            defaultExpanded
            onReply={(em, mode) => {
              setReplyToEmail(em);
              setReplyMode(mode);
            }}
          />
        )}

        {/* Inline reply editor */}
        {replyMode && lastEmail && (
          <InlineReplyEditor
            thread={thread}
            lastEmail={replyToEmail || lastEmail}
            mode={replyMode}
            domainId={domainId}
            domainName={activeDomain?.domain || ""}
            onClose={() => { setReplyMode(null); setReplyToEmail(null); }}
            onSent={() => {
              setReplyMode(null);
              setReplyToEmail(null);
              qc.invalidateQueries({ queryKey: queryKeys.threads.detail(threadId) });
              qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
            }}
          />
        )}

        <div ref={bottomRef} />
      </div>

      {/* Reply bar (only if reply editor not open) */}
      {!replyMode && (
        <div className="border-t px-6 py-3 shrink-0 flex items-center gap-2">
          <Button
            variant="outline"
            size="sm"
            onClick={() => { setReplyToEmail(lastEmail); setReplyMode("reply"); }}
          >
            <Reply className="h-3.5 w-3.5 mr-1.5" />
            Reply
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => { setReplyToEmail(lastEmail); setReplyMode("replyAll"); }}
          >
            <ReplyAll className="h-3.5 w-3.5 mr-1.5" />
            Reply All
          </Button>
          <Button
            variant="outline"
            size="sm"
            onClick={() => { setReplyToEmail(lastEmail); setReplyMode("forward"); }}
          >
            <Forward className="h-3.5 w-3.5 mr-1.5" />
            Forward
          </Button>
        </div>
      )}
    </div>
  );
}

// ─── Inline Reply Editor ──────────────────────────────────────────────

function InlineReplyEditor({
  thread,
  lastEmail,
  mode,
  domainId,
  domainName,
  onClose,
  onSent,
}: {
  thread: Thread;
  lastEmail: Email;
  mode: "reply" | "replyAll" | "forward";
  domainId: string;
  domainName: string;
  onClose: () => void;
  onSent: () => void;
}) {
  // Determine defaults
  const defaultFrom = getReplyFromAddress(thread.emails || [], domainName);
  const { defaultTo, defaultCc } = getReplyRecipients(lastEmail, mode, defaultFrom);
  const defaultSubject = getReplySubject(thread.subject, mode);
  const quotedHtml = getQuotedHtml(lastEmail, mode);

  const [fromAddress, setFromAddress] = useState(defaultFrom);
  const [to, setTo] = useState<string[]>(defaultTo);
  const [cc, setCc] = useState<string[]>(defaultCc);
  const [subject, setSubject] = useState(defaultSubject);
  const [bodyHtml, setBodyHtml] = useState("");
  const [bodyPlain, setBodyPlain] = useState("");
  const [showCc, setShowCc] = useState(defaultCc.length > 0);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");

  async function handleSend(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    if (to.length === 0) {
      setError("To is required");
      return;
    }

    setSending(true);
    try {
      // Combine user's reply with quoted text
      const fullHtml = bodyHtml + quotedHtml;
      const fullPlain = bodyPlain;

      // Build threading headers from the email being replied to
      const replyPayload: Record<string, unknown> = {
        from: fromAddress,
        to,
        cc,
        subject,
        html: fullHtml,
        text: fullPlain,
        domain_id: domainId,
        reply_to_thread_id: thread.id,
      };
      if (lastEmail.message_id) {
        replyPayload.in_reply_to = lastEmail.message_id;
        // Build references chain: existing references + the message we're replying to
        const existingRefs = lastEmail.references || [];
        replyPayload.references = [...existingRefs, lastEmail.message_id];
      }
      await api.post("/api/emails/send", replyPayload);
      onSent();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to send");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="rounded-lg border shadow-sm">
      <div className="flex items-center justify-between px-4 py-2 border-b bg-muted/30">
        <span className="text-sm font-medium">
          {mode === "forward" ? "Forward" : mode === "replyAll" ? "Reply All" : "Reply"}
        </span>
        <button onClick={onClose} className="p-0.5 hover:bg-muted rounded">
          <X className="h-4 w-4" />
        </button>
      </div>

      <form onSubmit={handleSend} className="p-4 space-y-3">
        {error && (
          <div className="text-xs text-destructive bg-destructive/10 p-2 rounded">
            {error}
          </div>
        )}

        <div className="space-y-2">
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">From</label>
            <Input
              value={fromAddress}
              onChange={(e) => setFromAddress(e.target.value)}
              className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1 bg-transparent"
            />
          </div>
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">To</label>
            <RecipientInput
              value={to}
              onChange={setTo}
              placeholder="recipient@example.com"
            />
            {!showCc && (
              <button
                type="button"
                onClick={() => setShowCc(true)}
                className="text-xs text-muted-foreground hover:text-foreground shrink-0"
              >
                Cc
              </button>
            )}
          </div>
          {showCc && (
            <div className="flex items-center gap-2">
              <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Cc</label>
              <RecipientInput
                value={cc}
                onChange={setCc}
                placeholder="cc@example.com"
              />
            </div>
          )}
          {mode !== "forward" && (
            <input type="hidden" value={subject} />
          )}
          {mode === "forward" && (
            <div className="flex items-center gap-2">
              <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Subj</label>
              <Input
                value={subject}
                onChange={(e) => setSubject(e.target.value)}
                className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1 bg-transparent"
              />
            </div>
          )}
        </div>

        <TipTapEditor
          onChange={(html, plain) => {
            setBodyHtml(html);
            setBodyPlain(plain);
          }}
          autofocus
          placeholder={mode === "forward" ? "Add a message..." : "Write your reply..."}
        />

        {/* Quoted text preview */}
        {quotedHtml && (
          <div
            className="text-xs text-muted-foreground border-l-2 border-muted pl-3 max-h-[150px] overflow-y-auto prose prose-xs max-w-none"
            dangerouslySetInnerHTML={{
              __html: DOMPurify.sanitize(quotedHtml, {
                ALLOWED_TAGS: ["p", "br", "strong", "em", "a", "div", "span", "blockquote"],
                ALLOWED_ATTR: ["href"],
              }),
            }}
          />
        )}

        <div className="flex items-center gap-2">
          <Button type="submit" size="sm" disabled={sending}>
            {sending ? <Spinner className="mr-1 h-3 w-3" /> : <Send className="mr-1 h-3 w-3" />}
            Send
          </Button>
          <Button type="button" variant="ghost" size="sm" onClick={onClose}>
            Cancel
          </Button>
        </div>
      </form>
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

  return (
    <div className="rounded-lg border">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex items-center gap-3 w-full px-4 py-3 text-left"
      >
        <div className="h-8 w-8 rounded-full bg-muted flex items-center justify-center text-xs font-medium shrink-0">
          {email.from_address.charAt(0).toUpperCase()}
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <ContactCard email={email.from_address}>
              <span className="text-sm font-medium truncate">
                {email.from_address}
              </span>
            </ContactCard>
            <span className="text-xs text-muted-foreground shrink-0">
              {formatRelativeTime(email.created_at)}
            </span>
          </div>
          {!expanded && (
            <p className="text-xs text-muted-foreground truncate">
              {email.body_plain?.slice(0, 100)}
            </p>
          )}
        </div>
      </button>

      {expanded && (
        <div className="px-4 pb-4">
          <div className="text-xs text-muted-foreground mb-3 space-y-0.5">
            <p>To: {parseAddresses(email.to_addresses).join(", ")}</p>
            {parseAddresses(email.cc_addresses).length > 0 && (
              <p>Cc: {parseAddresses(email.cc_addresses).join(", ")}</p>
            )}
          </div>
          {sanitizedHtml ? (
            <div
              className="prose prose-sm max-w-none"
              dangerouslySetInnerHTML={{ __html: sanitizedHtml }}
            />
          ) : (
            <pre className="text-sm whitespace-pre-wrap font-sans">
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
      )}
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
  return `me@${domainName}`;
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
