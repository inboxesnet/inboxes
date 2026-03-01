"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useNotifications } from "@/contexts/notification-context";
import { queryKeys } from "@/lib/query-keys";
import { hasLabel, threadBelongsInView } from "@/lib/types";
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
        case "email.sent": {
          const emailThread = (msg.payload?.thread as Thread) || null;

          if (emailThread && msg.thread_id) {
            // Slot the thread into each cached list using setQueriesData (no refetch)
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;

                const cacheEntry = qc
                  .getQueryCache()
                  .findAll({ queryKey: queryKeys.threads.lists() })
                  .find((q) => q.state.data === old);
                const queryDomainId = cacheEntry?.queryKey?.[2] as
                  | string
                  | undefined;
                const queryLabel = cacheEntry?.queryKey?.[3] as
                  | string
                  | undefined;

                if (!queryLabel) return old;

                // Skip lists for other domains
                if (
                  queryDomainId &&
                  emailThread.domain_id &&
                  queryDomainId !== emailThread.domain_id
                ) {
                  return old;
                }

                const belongs = threadBelongsInView(emailThread, queryLabel);
                const hadThread = old.threads.some(
                  (t) => t.id === msg.thread_id
                );

                if (belongs) {
                  if (hadThread) {
                    // Update existing thread in place
                    return {
                      ...old,
                      threads: old.threads.map((t) =>
                        t.id === msg.thread_id ? emailThread : t
                      ),
                    };
                  }
                  // Insert new thread sorted by last_message_at
                  return {
                    ...old,
                    total: old.total + 1,
                    threads: [emailThread, ...old.threads].sort(
                      (a, b) =>
                        new Date(b.last_message_at).getTime() -
                        new Date(a.last_message_at).getTime()
                    ),
                  };
                }

                // Thread doesn't belong — remove if present
                if (hadThread) {
                  return {
                    ...old,
                    total: Math.max(0, old.total - 1),
                    threads: old.threads.filter(
                      (t) => t.id !== msg.thread_id
                    ),
                  };
                }
                return old;
              }
            );

            // Invalidate thread detail if currently viewing this thread
            qc.invalidateQueries({
              queryKey: queryKeys.threads.detail(msg.thread_id),
            });
          } else {
            // Fallback: no thread in payload — use old invalidation approach
            if (msg.event === "email.received" && msg.domain_id) {
              qc.invalidateQueries({
                queryKey: queryKeys.threads.lists(),
                predicate: (query) => {
                  const key = query.queryKey;
                  return key[2] === msg.domain_id && key[3] === "inbox";
                },
              });
            }
            if (msg.event === "email.sent") {
              qc.invalidateQueries({
                queryKey: queryKeys.threads.lists(),
                predicate: (query) => query.queryKey[3] === "sent",
              });
            }
            if (msg.thread_id) {
              qc.invalidateQueries({
                queryKey: queryKeys.threads.detail(msg.thread_id),
              });
            }
          }

          qc.invalidateQueries({
            queryKey: queryKeys.domains.unreadCounts(),
          });
          break;
        }

        case "thread.starred":
        case "thread.unstarred": {
          const starThread = (msg.payload?.thread as Thread) || null;
          if (msg.thread_id && starThread) {
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;

                const queryLabel = qc
                  .getQueryCache()
                  .findAll({ queryKey: queryKeys.threads.lists() })
                  .find((q) => q.state.data === old)?.queryKey?.[3] as
                  | string
                  | undefined;

                const updatedLabels = starThread.labels || [];
                const hadThread = old.threads.some((t) => t.id === msg.thread_id);

                // If this is the starred view and thread was unstarred, remove it
                if (
                  queryLabel === "starred" &&
                  msg.event === "thread.unstarred" &&
                  hadThread
                ) {
                  return {
                    ...old,
                    total: Math.max(0, old.total - 1),
                    threads: old.threads.filter((t) => t.id !== msg.thread_id),
                  };
                }

                // If this is the starred view and thread was starred, add it
                if (
                  queryLabel === "starred" &&
                  msg.event === "thread.starred" &&
                  !hadThread &&
                  starThread &&
                  threadBelongsInView(starThread, "starred")
                ) {
                  return {
                    ...old,
                    total: old.total + 1,
                    threads: [starThread, ...old.threads].sort(
                      (a, b) =>
                        new Date(b.last_message_at).getTime() -
                        new Date(a.last_message_at).getTime()
                    ),
                  };
                }

                // Otherwise just update labels in place
                return {
                  ...old,
                  threads: old.threads.map((t) =>
                    t.id === msg.thread_id
                      ? { ...t, labels: updatedLabels }
                      : t
                  ),
                };
              }
            );
          }
          break;
        }

        case "thread.muted":
        case "thread.unmuted": {
          const muteThread = (msg.payload?.thread as Thread) || null;
          if (msg.thread_id && muteThread) {
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;
                return {
                  ...old,
                  threads: old.threads.map((t) =>
                    t.id === msg.thread_id ? { ...t, labels: muteThread.labels || t.labels } : t
                  ),
                };
              }
            );
            qc.setQueryData<Thread>(
              queryKeys.threads.detail(msg.thread_id!),
              (old) => (old ? { ...old, labels: muteThread.labels || old.labels } : old)
            );
          }
          break;
        }

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
          const thread = (msg.payload?.thread as Thread) || null;
          if (msg.thread_id) {
            // For each cached list, check if thread belongs in that view
            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;
                const queryLabel = (qc.getQueryCache().findAll({ queryKey: queryKeys.threads.lists() })
                  .find((q) => q.state.data === old)?.queryKey?.[3]) as string | undefined;

                const hadThread = old.threads.some((t) => t.id === msg.thread_id);

                if (thread && queryLabel && threadBelongsInView(thread, queryLabel)) {
                  // Thread belongs in this view — add or update it
                  if (hadThread) {
                    return {
                      ...old,
                      threads: old.threads.map((t) =>
                        t.id === msg.thread_id ? thread : t
                      ),
                    };
                  }
                  return {
                    ...old,
                    total: old.total + 1,
                    threads: [thread, ...old.threads].sort(
                      (a, b) => new Date(b.last_message_at).getTime() - new Date(a.last_message_at).getTime()
                    ),
                  };
                }

                // Thread doesn't belong in this view — remove it
                if (hadThread) {
                  return {
                    ...old,
                    total: Math.max(0, old.total - 1),
                    threads: old.threads.filter((t) => t.id !== msg.thread_id),
                  };
                }
                return old;
              }
            );
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

        case "email.status_updated": {
          if (msg.thread_id) {
            qc.invalidateQueries({
              queryKey: queryKeys.threads.detail(msg.thread_id),
            });
          }
          // Show notification on send failure
          const status = msg.payload?.status as string | undefined;
          const subject = msg.payload?.subject as string | undefined;
          if (status === "failed" || status === "bounced") {
            const title = status === "bounced" ? "Email bounced" : "Email failed";
            if (typeof Notification !== "undefined" && Notification.permission === "granted") {
              new Notification(title, { body: subject || "An email failed to deliver" });
            }
          }
          break;
        }

        case "sync.completed":
          qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
          qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
          break;

        case "plan.changed": {
          const plan = msg.payload?.plan as string | undefined;
          if (plan === "cancelled") {
            toast.warning("Your subscription has been cancelled. Check Settings > Billing for details.");
          } else if (plan === "pro") {
            toast.success("Your subscription is now active!");
          }
          break;
        }

        case "domain.disconnected": {
          const domain = msg.payload?.domain as string | undefined;
          const reason = msg.payload?.reason as string | undefined;
          if (reason === "api_key_revoked") {
            toast.error("API key revoked — all domains disconnected. Check Settings.");
          } else {
            toast.error(`Domain ${domain || ""} has been disconnected from Resend.`);
          }
          qc.invalidateQueries({ queryKey: queryKeys.domains.all });
          break;
        }

        case "domain.reconnected": {
          const domain = msg.payload?.domain as string | undefined;
          toast.success(`Domain ${domain || ""} has reconnected.`);
          qc.invalidateQueries({ queryKey: queryKeys.domains.all });
          break;
        }

        case "domain.dns_degraded": {
          const domain = msg.payload?.domain as string | undefined;
          const degraded = msg.payload?.degraded as string[] | undefined;
          toast.warning(`DNS records changed for ${domain || ""}: ${degraded?.join(", ") || "verification failed"}`);
          break;
        }
      }
    });

    return unsub;
  }, [subscribe, setLastEventId, qc]);

  return null;
}
