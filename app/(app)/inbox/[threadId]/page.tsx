"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import DOMPurify from "isomorphic-dompurify";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

interface EmailMessage {
  id: string;
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

function EmailMessageItem({
  email,
  isLast,
  defaultExpanded,
}: {
  email: EmailMessage;
  isLast: boolean;
  defaultExpanded: boolean;
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
        </div>
      )}
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
          />
        ))}
      </div>
    </div>
  );
}
