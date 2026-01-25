"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";
import { useToast } from "@/components/ui/toast";

interface ThreadSummary {
  id: string;
  subject: string;
  unread_count: number;
  starred: boolean;
  message_count: number;
  last_message_at: string;
  from_address: string;
  to_addresses: string[];
  preview: string;
}

interface ThreadsResponse {
  threads: ThreadSummary[];
  total: number;
  page: number;
  totalPages: number;
}

function formatTime(dateStr: string): string {
  const date = new Date(dateStr);
  const now = new Date();
  const diff = now.getTime() - date.getTime();
  const oneDay = 24 * 60 * 60 * 1000;

  if (diff < oneDay && date.getDate() === now.getDate()) {
    return date.toLocaleTimeString([], { hour: "numeric", minute: "2-digit" });
  }

  if (diff < 7 * oneDay) {
    return date.toLocaleDateString([], { weekday: "short" });
  }

  return date.toLocaleDateString([], { month: "short", day: "numeric" });
}

function extractName(address: string): string {
  const match = address.match(/^"?([^"<]+)"?\s*</);
  if (match) return match[1].trim();
  return address.split("@")[0];
}

function getRecipientDisplay(toAddresses: unknown): string {
  const addresses = Array.isArray(toAddresses) ? toAddresses : [];
  if (addresses.length === 0) return "Unknown";
  const firstName = extractName(addresses[0]);
  if (addresses.length === 1) return firstName;
  return `${firstName} +${addresses.length - 1}`;
}

function getInitial(toAddresses: unknown): string {
  const addresses = Array.isArray(toAddresses) ? toAddresses : [];
  if (addresses.length === 0) return "?";
  const name = extractName(addresses[0]);
  return name.charAt(0).toUpperCase();
}

export default function SentPage() {
  const router = useRouter();
  const { addToast } = useToast();
  const [threads, setThreads] = useState<ThreadSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);

  const fetchThreads = useCallback(async (pageNum: number) => {
    setLoading(true);
    try {
      const res = await fetch(`/api/threads?folder=sent&page=${pageNum}`);
      if (res.ok) {
        const data: ThreadsResponse = await res.json();
        setThreads(data.threads);
        setTotalPages(data.totalPages);
      } else {
        addToast("Failed to load sent messages", "destructive");
      }
    } catch {
      addToast("Network error. Please check your connection.", "destructive");
    } finally {
      setLoading(false);
    }
  }, [addToast]);

  useEffect(() => {
    fetchThreads(page);
  }, [page, fetchThreads]);

  if (loading && threads.length === 0) {
    return (
      <div className="flex flex-col gap-2 p-4">
        {Array.from({ length: 6 }).map((_, i) => (
          <div key={i} className="flex items-center gap-3 rounded-lg border p-4">
            <div className="h-10 w-10 animate-pulse rounded-full bg-muted" />
            <div className="flex-1 space-y-2">
              <div className="h-4 w-1/3 animate-pulse rounded bg-muted" />
              <div className="h-3 w-2/3 animate-pulse rounded bg-muted" />
            </div>
          </div>
        ))}
      </div>
    );
  }

  if (!loading && threads.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-20 text-center">
        <div className="text-4xl mb-4">📤</div>
        <h2 className="text-xl font-semibold">No sent messages</h2>
        <p className="mt-2 text-muted-foreground">
          Messages you send will appear here.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <div className="border-b px-3 py-3 sm:px-4">
        <h1 className="text-lg font-semibold">Sent</h1>
      </div>

      <div className="divide-y">
        {threads.map((thread) => (
          <button
            key={thread.id}
            onClick={() => router.push(`/inbox/${thread.id}`)}
            className="flex min-h-[60px] w-full items-start gap-3 px-3 py-3 text-left transition-colors hover:bg-muted/50 sm:px-4"
          >
            <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
              {getInitial(thread.to_addresses)}
            </div>

            <div className="min-w-0 flex-1">
              <div className="flex items-center gap-2">
                <span className="truncate text-sm font-normal">
                  To: {getRecipientDisplay(thread.to_addresses)}
                </span>
                {thread.message_count > 1 && (
                  <span className="shrink-0 text-xs text-muted-foreground">
                    ({thread.message_count})
                  </span>
                )}
                <span className="ml-auto shrink-0 text-xs text-muted-foreground">
                  {formatTime(thread.last_message_at)}
                </span>
              </div>

              <p className={cn("truncate text-sm text-muted-foreground")}>
                {thread.subject}
              </p>

              <p className="mt-0.5 truncate text-xs text-muted-foreground">
                {thread.preview}
              </p>
            </div>
          </button>
        ))}
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t px-3 py-3 sm:px-4">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page <= 1}
            className="h-10 min-w-[80px] sm:h-9"
          >
            Previous
          </Button>
          <span className="text-xs text-muted-foreground sm:text-sm">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            disabled={page >= totalPages}
            className="h-10 min-w-[80px] sm:h-9"
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
