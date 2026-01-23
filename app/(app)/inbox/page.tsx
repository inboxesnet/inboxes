"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { useWebSocket } from "@/hooks/use-websocket";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

interface ThreadSummary {
  id: string;
  subject: string;
  unread_count: number;
  starred: boolean;
  message_count: number;
  last_message_at: string;
  from_address: string;
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

function getInitial(address: string): string {
  const name = extractName(address);
  return name.charAt(0).toUpperCase();
}

export default function InboxPage() {
  const router = useRouter();
  const [threads, setThreads] = useState<ThreadSummary[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);

  const fetchThreads = useCallback(async (pageNum: number) => {
    setLoading(true);
    try {
      const res = await fetch(`/api/threads?folder=inbox&page=${pageNum}`);
      if (res.ok) {
        const data: ThreadsResponse = await res.json();
        setThreads(data.threads);
        setTotalPages(data.totalPages);
      }
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchThreads(page);
  }, [page, fetchThreads]);

  const { subscribe } = useWebSocket();

  useEffect(() => {
    const unsubscribe = subscribe((event) => {
      if (event.event === "new_email") {
        fetchThreads(page);
      }
    });
    return unsubscribe;
  }, [subscribe, page, fetchThreads]);

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
        <div className="text-4xl mb-4">📭</div>
        <h2 className="text-xl font-semibold">Your inbox is empty</h2>
        <p className="mt-2 text-muted-foreground">
          New emails will appear here when they arrive.
        </p>
      </div>
    );
  }

  return (
    <div className="flex flex-col">
      <div className="border-b px-4 py-3">
        <h1 className="text-lg font-semibold">Inbox</h1>
      </div>

      <div className="divide-y">
        {threads.map((thread) => {
          const isUnread = thread.unread_count > 0;
          return (
            <button
              key={thread.id}
              onClick={() => router.push(`/inbox/${thread.id}`)}
              className={cn(
                "flex w-full items-start gap-3 px-4 py-3 text-left transition-colors hover:bg-muted/50",
                isUnread && "bg-blue-50/50 dark:bg-blue-950/20"
              )}
            >
              <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10 text-sm font-medium text-primary">
                {getInitial(thread.from_address)}
              </div>

              <div className="min-w-0 flex-1">
                <div className="flex items-center gap-2">
                  <span
                    className={cn(
                      "truncate text-sm",
                      isUnread ? "font-semibold" : "font-normal"
                    )}
                  >
                    {extractName(thread.from_address)}
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

                <div className="flex items-center gap-2">
                  {isUnread && (
                    <span className="mt-0.5 h-2 w-2 shrink-0 rounded-full bg-blue-500" />
                  )}
                  <p
                    className={cn(
                      "truncate text-sm",
                      isUnread
                        ? "font-medium text-foreground"
                        : "text-muted-foreground"
                    )}
                  >
                    {thread.subject}
                  </p>
                </div>

                <p className="mt-0.5 truncate text-xs text-muted-foreground">
                  {thread.preview}
                </p>
              </div>
            </button>
          );
        })}
      </div>

      {totalPages > 1 && (
        <div className="flex items-center justify-between border-t px-4 py-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => Math.max(1, p - 1))}
            disabled={page <= 1}
          >
            Previous
          </Button>
          <span className="text-sm text-muted-foreground">
            Page {page} of {totalPages}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
            disabled={page >= totalPages}
          >
            Next
          </Button>
        </div>
      )}
    </div>
  );
}
