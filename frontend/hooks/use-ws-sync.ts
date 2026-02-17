"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNotifications } from "@/contexts/notification-context";
import { queryKeys } from "@/lib/query-keys";
import type { WSMessage, Thread, ThreadListResponse } from "@/lib/types";

/**
 * Central WebSocket-to-React-Query cache sync.
 * Subscribes to all event types and maps them to cache operations.
 * Rendered once in the app layout.
 *
 * Uses setQueriesData for direct cache updates where possible,
 * falling back to invalidateQueries for complex operations.
 */
export function WSSync() {
  const { subscribe, setLastEventId } = useNotifications();
  const qc = useQueryClient();

  useEffect(() => {
    const unsub = subscribe("*", (msg: WSMessage) => {
      // Track event ID for reconnection catchup
      if (msg.id) {
        setLastEventId(msg.id);
      }

      switch (msg.event) {
        case "email.received":
          // Invalidate thread list for that domain + inbox folder
          if (msg.domain_id) {
            qc.invalidateQueries({
              queryKey: queryKeys.threads.lists(),
              predicate: (query) => {
                const key = query.queryKey;
                return key[2] === msg.domain_id && key[3] === "inbox";
              },
            });
          }
          qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
          // Invalidate thread detail if currently viewing
          if (msg.thread_id) {
            qc.invalidateQueries({
              queryKey: queryKeys.threads.detail(msg.thread_id),
            });
          }
          break;

        case "email.sent":
          // Invalidate sent folder lists
          qc.invalidateQueries({
            queryKey: queryKeys.threads.lists(),
            predicate: (query) => query.queryKey[3] === "sent",
          });
          if (msg.thread_id) {
            qc.invalidateQueries({
              queryKey: queryKeys.threads.detail(msg.thread_id),
            });
          }
          break;

        case "thread.starred":
        case "thread.unstarred":
          if (msg.thread_id) {
            const starred = msg.event === "thread.starred";
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;
                return {
                  ...old,
                  threads: old.threads.map((t) =>
                    t.id === msg.thread_id ? { ...t, starred } : t
                  ),
                };
              }
            );
          }
          break;

        case "thread.read":
        case "thread.unread": {
          if (msg.thread_id) {
            const unreadCount = msg.event === "thread.read" ? 0 : 1;
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;
                return {
                  ...old,
                  threads: old.threads.map((t) =>
                    t.id === msg.thread_id
                      ? { ...t, unread_count: unreadCount }
                      : t
                  ),
                };
              }
            );
          }
          qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
          break;
        }

        case "thread.archived":
        case "thread.trashed":
        case "thread.spammed":
        case "thread.moved": {
          // Remove thread from current folder list, add to target folder list
          const thread = (msg.payload?.thread as Thread) || null;
          if (msg.thread_id) {
            // Remove from all lists that no longer contain this thread
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;
                const hadThread = old.threads.some((t) => t.id === msg.thread_id);
                if (!hadThread) return old;
                return {
                  ...old,
                  total: Math.max(0, old.total - 1),
                  threads: old.threads.filter((t) => t.id !== msg.thread_id),
                };
              }
            );
            // If we have full thread data, add it to the target folder's list
            if (thread) {
              qc.setQueriesData<ThreadListResponse>(
                { queryKey: queryKeys.threads.lists() },
                (old) => {
                  if (!old) return old;
                  // Only add to matching folder queries
                  const queryFolder = (qc.getQueryCache().findAll({ queryKey: queryKeys.threads.lists() })
                    .find((q) => q.state.data === old)?.queryKey?.[3]) as string | undefined;
                  if (queryFolder !== thread.folder) return old;
                  // Don't add duplicates
                  if (old.threads.some((t) => t.id === thread.id)) return old;
                  return {
                    ...old,
                    total: old.total + 1,
                    threads: [thread, ...old.threads].sort(
                      (a, b) => new Date(b.last_message_at).getTime() - new Date(a.last_message_at).getTime()
                    ),
                  };
                }
              );
            }
          }
          qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
          break;
        }

        case "thread.deleted":
          if (msg.thread_id) {
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;
                const hadThread = old.threads.some((t) => t.id === msg.thread_id);
                if (!hadThread) return old;
                return {
                  ...old,
                  total: Math.max(0, old.total - 1),
                  threads: old.threads.filter((t) => t.id !== msg.thread_id),
                };
              }
            );
          }
          qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
          break;

        case "thread.bulk_action":
          qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
          qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
          break;

        case "email.status_updated":
          if (msg.thread_id) {
            qc.invalidateQueries({
              queryKey: queryKeys.threads.detail(msg.thread_id),
            });
          }
          break;

        case "sync.completed":
          qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
          qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
          break;
      }
    });

    return unsub;
  }, [subscribe, setLastEventId, qc]);

  return null;
}
