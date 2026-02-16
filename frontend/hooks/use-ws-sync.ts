"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useNotifications } from "@/contexts/notification-context";
import { queryKeys } from "@/lib/query-keys";
import type { WSMessage, ThreadListResponse } from "@/lib/types";

/**
 * Central WebSocket-to-React-Query cache sync.
 * Subscribes to all event types and maps them to cache operations.
 * Rendered once in the app layout.
 */
export function WSSync() {
  const { subscribe } = useNotifications();
  const qc = useQueryClient();

  useEffect(() => {
    const unsub = subscribe("*", (msg: WSMessage) => {
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
        case "thread.moved":
        case "thread.deleted":
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
  }, [subscribe, qc]);

  return null;
}
