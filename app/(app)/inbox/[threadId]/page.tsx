"use client";

import { useEffect, useState, useRef } from "react";
import { useParams, useRouter } from "next/navigation";
import DOMPurify from "isomorphic-dompurify";
import { Reply, ReplyAll, Forward, Send, Bold, Italic, Link, List, ListOrdered } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast";
import { ComposeModal, ComposePreFill } from "@/components/compose-modal";
import { cn } from "@/lib/utils";

interface EmailMessage {
  id: string;
  message_id: string;
  from_address: string;
  to_addresses: string[];
  cc_addresses: string[];
  subject: string;
  body_html: string;
  body_plain: string;
  direction: string;
  read: boolean;
  received_at: string;
  attachments: unknown[];
}

interface ThreadDetail {
  id: string;
  subject: string;
  starred: boolean;
  folder: string;
  message_count: number;
  emails: EmailMessage[];
}

function extractName(address: string): string {
  const match = address.match(/^"?([^"<]+)"?\s*</);
  if (match) return match[1].trim();
  return address.split("@")[0];
}

function extractEmail(address: string): string {
  const match = address.match(/<([^>]+)>/);
  if (match) return match[1];
  return address;
}

function getInitial(address: string): string {
  const name = extractName(address);
  return name.charAt(0).toUpperCase();
}

function formatDateTime(dateStr: string): string {
  const date = new Date(dateStr);
  return date.toLocaleDateString([], {
    month: "short",
    day: "numeric",
    year: "numeric",
    hour: "numeric",
    minute: "2-digit",
  });
}

function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      "p", "br", "b", "i", "em", "strong", "a", "ul", "ol", "li",
      "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "code",
      "table", "thead", "tbody", "tr", "th", "td", "img", "div", "span",
      "hr", "sub", "sup",
    ],
    ALLOWED_ATTR: ["href", "src", "alt", "title", "width", "height", "style", "target"],
    FORBID_TAGS: ["script", "iframe", "object", "embed", "form"],
    FORBID_ATTR: ["onerror", "onclick", "onload", "onmouseover"],
  });
}

function formatRecipients(addresses: string[]): string {
  if (!addresses || addresses.length === 0) return "";
  return addresses.map((addr) => extractName(addr)).join(", ");
}

function buildQuotedHtml(email: EmailMessage): string {
  const date = formatDateTime(email.received_at);
  const from = email.from_address;
  return `<br/><br/><div style="border-left:2px solid #ccc;padding-left:12px;margin-left:0;color:#555;">` +
    `<p>On ${date}, ${from} wrote:</p>` +
    `${email.body_html || email.body_plain || ""}</div>`;
}

function EmailMessageItem({
  email,
  defaultExpanded,
  onReply,
  onReplyAll,
  onForward,
}: {
  email: EmailMessage;
  isLast: boolean;
  defaultExpanded: boolean;
  onReply: (email: EmailMessage) => void;
  onReplyAll: (email: EmailMessage) => void;
  onForward: (email: EmailMessage) => void;
}) {
  const [expanded, setExpanded] = useState(defaultExpanded);

  const sanitizedBody = sanitizeHtml(email.body_html || "");
  const hasHtmlBody = sanitizedBody.trim().length > 0;

  return (
    <div className="border-b last:border-b-0">
      <button
        onClick={() => setExpanded(!expanded)}
        className={cn(
          "flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/30",
          !expanded && "cursor-pointer"
        )}
      >
        <div className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
          {getInitial(email.from_address)}
        </div>

        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className="text-sm font-medium">
              {extractName(email.from_address)}
            </span>
            <span className="text-xs text-muted-foreground">
              &lt;{extractEmail(email.from_address)}&gt;
            </span>
            <span className="ml-auto shrink-0 text-xs text-muted-foreground">
              {formatDateTime(email.received_at)}
            </span>
          </div>

          {expanded && (
            <div className="mt-0.5 text-xs text-muted-foreground">
              <span>To: {formatRecipients(email.to_addresses)}</span>
              {email.cc_addresses && email.cc_addresses.length > 0 && (
                <span className="ml-2">
                  CC: {formatRecipients(email.cc_addresses)}
                </span>
              )}
            </div>
          )}

          {!expanded && (
            <p className="mt-0.5 truncate text-sm text-muted-foreground">
              {email.body_plain
                ? email.body_plain.substring(0, 100)
                : "No content"}
            </p>
          )}
        </div>
      </button>

      {expanded && (
        <div className="px-4 pb-4 pl-16">
          {hasHtmlBody ? (
            <div
              className="prose prose-sm max-w-none dark:prose-invert"
              dangerouslySetInnerHTML={{ __html: sanitizedBody }}
            />
          ) : (
            <pre className="whitespace-pre-wrap text-sm text-foreground">
              {email.body_plain || "No content"}
            </pre>
          )}

          <div className="mt-3 flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => { e.stopPropagation(); onReply(email); }}
            >
              <Reply className="mr-1.5 h-3.5 w-3.5" />
              Reply
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => { e.stopPropagation(); onReplyAll(email); }}
            >
              <ReplyAll className="mr-1.5 h-3.5 w-3.5" />
              Reply All
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={(e) => { e.stopPropagation(); onForward(email); }}
            >
              <Forward className="mr-1.5 h-3.5 w-3.5" />
              Forward
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

interface ReplyFormData {
  to: string;
  cc: string;
  subject: string;
  in_reply_to: string;
  references: string[];
}

function InlineReplyForm({
  replyData,
  onSent,
  onCancel,
}: {
  replyData: ReplyFormData;
  onSent: (email: EmailMessage) => void;
  onCancel: () => void;
}) {
  const { addToast } = useToast();
  const editorRef = useRef<HTMLDivElement>(null);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");

  function execCommand(command: string, value?: string) {
    document.execCommand(command, false, value);
    editorRef.current?.focus();
  }

  async function handleSend() {
    setError("");
    const bodyHtml = editorRef.current?.innerHTML || "";
    const bodyPlain = editorRef.current?.innerText || "";

    if (!bodyPlain.trim()) {
      setError("Reply body is required");
      return;
    }

    const toAddresses = replyData.to.split(",").map((e) => e.trim()).filter(Boolean);
    const ccAddresses = replyData.cc ? replyData.cc.split(",").map((e) => e.trim()).filter(Boolean) : [];

    setSending(true);
    try {
      const res = await fetch("/api/emails/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          to: toAddresses,
          cc: ccAddresses.length > 0 ? ccAddresses : undefined,
          subject: replyData.subject,
          body_html: bodyHtml,
          body_plain: bodyPlain,
          in_reply_to: replyData.in_reply_to,
          references: replyData.references,
        }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setError((data as { error?: string }).error || "Failed to send reply");
        setSending(false);
        return;
      }

      const result = await res.json();
      addToast("Reply sent", "success");

      // Construct the sent email to append to thread immediately
      const sentEmail: EmailMessage = {
        id: result.email.id,
        message_id: result.email.message_id,
        from_address: "You",
        to_addresses: toAddresses,
        cc_addresses: ccAddresses,
        subject: replyData.subject,
        body_html: bodyHtml,
        body_plain: bodyPlain,
        direction: "outbound",
        read: true,
        received_at: new Date().toISOString(),
        attachments: [],
      };
      onSent(sentEmail);
    } catch {
      setError("Failed to send reply. Please try again.");
    } finally {
      setSending(false);
    }
  }

  return (
    <div className="border-t bg-muted/20 p-4">
      <div className="mb-2 text-xs text-muted-foreground">
        To: {replyData.to}
        {replyData.cc && <span className="ml-2">CC: {replyData.cc}</span>}
      </div>

      {error && (
        <div className="mb-2 rounded-md border border-destructive bg-destructive/10 px-3 py-2 text-sm text-destructive">
          {error}
        </div>
      )}

      <div className="flex items-center gap-1 border-b pb-2 mb-2">
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7" onClick={() => execCommand("bold")} title="Bold">
          <Bold className="h-3.5 w-3.5" />
        </Button>
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7" onClick={() => execCommand("italic")} title="Italic">
          <Italic className="h-3.5 w-3.5" />
        </Button>
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7" onClick={() => { const url = prompt("Enter URL:"); if (url) execCommand("createLink", url); }} title="Insert Link">
          <Link className="h-3.5 w-3.5" />
        </Button>
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7" onClick={() => execCommand("insertUnorderedList")} title="Bullet List">
          <List className="h-3.5 w-3.5" />
        </Button>
        <Button type="button" variant="ghost" size="icon" className="h-7 w-7" onClick={() => execCommand("insertOrderedList")} title="Numbered List">
          <ListOrdered className="h-3.5 w-3.5" />
        </Button>
      </div>

      <div
        ref={editorRef}
        contentEditable
        className="min-h-[120px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
        role="textbox"
        aria-label="Reply body"
        aria-multiline="true"
      />

      <div className="mt-3 flex items-center gap-2">
        <Button type="button" onClick={handleSend} disabled={sending} size="sm">
          {sending ? "Sending..." : (
            <>
              <Send className="mr-1.5 h-3.5 w-3.5" />
              Send
            </>
          )}
        </Button>
        <Button type="button" variant="outline" size="sm" onClick={onCancel}>
          Cancel
        </Button>
      </div>
    </div>
  );
}

export default function ThreadViewPage() {
  const params = useParams();
  const router = useRouter();
  const threadId = params.threadId as string;

  const [thread, setThread] = useState<ThreadDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [replyData, setReplyData] = useState<ReplyFormData | null>(null);
  const [forwardPreFill, setForwardPreFill] = useState<ComposePreFill | undefined>(undefined);
  const [forwardOpen, setForwardOpen] = useState(false);

  useEffect(() => {
    async function fetchThread() {
      setLoading(true);
      try {
        const res = await fetch(`/api/threads/${threadId}`);
        if (res.ok) {
          const data: ThreadDetail = await res.json();
          setThread(data);
        } else if (res.status === 404) {
          setError("Thread not found");
        } else {
          setError("Failed to load thread");
        }
      } catch {
        setError("Failed to load thread");
      } finally {
        setLoading(false);
      }
    }
    fetchThread();
  }, [threadId]);

  function collectReferences(email: EmailMessage): string[] {
    // Collect all message_ids in thread up to and including this email
    if (!thread) return [];
    const refs: string[] = [];
    for (const e of thread.emails) {
      if (e.message_id) refs.push(e.message_id);
      if (e.id === email.id) break;
    }
    return refs;
  }

  function handleReply(email: EmailMessage) {
    const fromEmail = extractEmail(email.from_address);
    const subject = thread?.subject || email.subject;
    const reSubject = subject.startsWith("Re:") ? subject : `Re: ${subject}`;

    setReplyData({
      to: fromEmail,
      cc: "",
      subject: reSubject,
      in_reply_to: email.message_id,
      references: collectReferences(email),
    });
  }

  function handleReplyAll(email: EmailMessage) {
    const fromEmail = extractEmail(email.from_address);
    const subject = thread?.subject || email.subject;
    const reSubject = subject.startsWith("Re:") ? subject : `Re: ${subject}`;

    // To: original sender
    // CC: all other recipients (to + cc), excluding current user (we don't know current user email client-side, but include all)
    const toAddresses = Array.isArray(email.to_addresses) ? email.to_addresses : [];
    const ccAddresses = Array.isArray(email.cc_addresses) ? email.cc_addresses : [];

    // All participants except the original sender (who goes in To)
    const allOthers = [...toAddresses, ...ccAddresses]
      .map((addr) => extractEmail(addr))
      .filter((addr) => addr !== fromEmail);
    const uniqueOthers = Array.from(new Set(allOthers));

    setReplyData({
      to: fromEmail,
      cc: uniqueOthers.join(", "),
      subject: reSubject,
      in_reply_to: email.message_id,
      references: collectReferences(email),
    });
  }

  function handleForward(email: EmailMessage) {
    const subject = thread?.subject || email.subject;
    const fwdSubject = subject.startsWith("Fwd:") ? subject : `Fwd: ${subject}`;
    const quotedHtml = buildQuotedHtml(email);

    setForwardPreFill({
      to: "",
      subject: fwdSubject,
      bodyHtml: quotedHtml,
    });
    setForwardOpen(true);
  }

  function handleReplySent(sentEmail: EmailMessage) {
    if (thread) {
      setThread({
        ...thread,
        message_count: thread.message_count + 1,
        emails: [...thread.emails, sentEmail],
      });
    }
    setReplyData(null);
  }

  if (loading) {
    return (
      <div className="flex flex-col gap-4 p-4">
        <div className="h-6 w-48 animate-pulse rounded bg-muted" />
        <div className="space-y-3">
          {Array.from({ length: 3 }).map((_, i) => (
            <div key={i} className="flex items-start gap-3 rounded-lg border p-4">
              <div className="h-9 w-9 animate-pulse rounded-full bg-muted" />
              <div className="flex-1 space-y-2">
                <div className="h-4 w-1/4 animate-pulse rounded bg-muted" />
                <div className="h-3 w-3/4 animate-pulse rounded bg-muted" />
                <div className="h-3 w-1/2 animate-pulse rounded bg-muted" />
              </div>
            </div>
          ))}
        </div>
      </div>
    );
  }

  if (error || !thread) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-center">
        <h2 className="text-xl font-semibold">{error || "Thread not found"}</h2>
        <Button
          variant="outline"
          className="mt-4"
          onClick={() => router.push("/inbox")}
        >
          Back to Inbox
        </Button>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <div className="flex items-center gap-3 border-b px-4 py-3">
        <Button
          variant="ghost"
          size="sm"
          onClick={() => router.push("/inbox")}
          className="shrink-0"
        >
          <svg
            xmlns="http://www.w3.org/2000/svg"
            width="16"
            height="16"
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth="2"
            strokeLinecap="round"
            strokeLinejoin="round"
          >
            <path d="m15 18-6-6 6-6" />
          </svg>
          <span className="ml-1">Back</span>
        </Button>

        <h1 className="min-w-0 flex-1 truncate text-lg font-semibold">
          {thread.subject}
        </h1>

        <span className="shrink-0 text-sm text-muted-foreground">
          {thread.message_count} message{thread.message_count !== 1 ? "s" : ""}
        </span>
      </div>

      <div className="divide-y">
        {thread.emails.map((email, index) => (
          <EmailMessageItem
            key={email.id}
            email={email}
            isLast={index === thread.emails.length - 1}
            defaultExpanded={index === thread.emails.length - 1}
            onReply={handleReply}
            onReplyAll={handleReplyAll}
            onForward={handleForward}
          />
        ))}
      </div>

      {replyData && (
        <InlineReplyForm
          replyData={replyData}
          onSent={handleReplySent}
          onCancel={() => setReplyData(null)}
        />
      )}

      <ComposeModal
        open={forwardOpen}
        onOpenChange={setForwardOpen}
        preFill={forwardPreFill}
      />
    </div>
  );
}
