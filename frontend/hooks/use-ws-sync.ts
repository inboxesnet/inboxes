"use client";

import { useEffect } from "react";
import { useQueryClient, QueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { useNotifications } from "@/contexts/notification-context";
import { queryKeys } from "@/lib/query-keys";
import { hasLabel, threadBelongsInView } from "@/lib/types";
import type { WSMessage, Thread, Email, ThreadListResponse, UnreadCounts } from "@/lib/types";

// --- Helpers for surgical cache updates ---

/** Look up a thread from any cached thread list */
function findCachedThread(
  qc: QueryClient,
  threadId: string
): Thread | undefined {
  const allQueries = qc
    .getQueryCache()
    .findAll({ queryKey: queryKeys.threads.lists() });
  for (const query of allQueries) {
    const data = query.state.data as ThreadListResponse | undefined;
    if (data?.threads) {
      const found = data.threads.find((t) => t.id === threadId);
      if (found) return found;
    }
  }
  return undefined;
}

/** Whether a thread contributes to inbox unread counts */
function isInboxEligible(thread: { labels?: string[] }): boolean {
  return (
    hasLabel(thread, "inbox") &&
    !hasLabel(thread, "trash") &&
    !hasLabel(thread, "spam")
  );
}

/** Compute unread count contribution of a thread (0 if not inbox-eligible) */
function unreadContribution(thread: Thread | undefined): number {
  if (!thread || !isInboxEligible(thread)) return 0;
  return thread.unread_count;
}

/** Surgically adjust domain unread count by delta */
function adjustUnreadCount(
  qc: QueryClient,
  domainId: string | undefined,
  delta: number
): void {
  if (!domainId || delta === 0) return;
  qc.setQueryData<UnreadCounts>(queryKeys.domains.unreadCounts(), (old) => {
    if (!old) return old;
    return { ...old, [domainId]: Math.max(0, (old[domainId] || 0) + delta) };
  });
}

/** Compute new labels after a bulk action */
function applyBulkLabels(
  labels: string[],
  action: string,
  label: string
): string[] {
  switch (action) {
    case "archive":
      return labels.filter((l) => l !== "inbox");
    case "trash":
      return [...labels.filter((l) => l !== "trash"), "trash"];
    case "spam":
      return [...labels.filter((l) => l !== "spam"), "spam"];
    case "move":
      switch (label) {
        case "inbox":
          return [
            ...labels.filter(
              (l) => l !== "inbox" && l !== "trash" && l !== "spam"
            ),
            "inbox",
          ];
        case "trash":
          return [...labels.filter((l) => l !== "trash"), "trash"];
        case "spam":
          return [...labels.filter((l) => l !== "spam"), "spam"];
        case "archive":
          return labels.filter((l) => l !== "inbox");
        default:
          return labels.includes(label) ? labels : [...labels, label];
      }
    case "mute":
      return labels.includes("muted") ? labels : [...labels, "muted"];
    case "unmute":
      return labels.filter((l) => l !== "muted");
    case "label":
      return labels.includes(label) ? labels : [...labels, label];
    case "unlabel":
      return labels.filter((l) => l !== label);
    default:
      return labels;
  }
}

/**
 * Central WebSocket-to-React-Query cache sync.
 * Subscribes to all event types and maps them to cache operations.
 * Rendered once in the app layout.
 *
 * Uses setQueryData / setQueriesData for surgical zero-network updates.
 * Only sync.completed and rare fallbacks trigger invalidation.
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

          // Capture old cached state BEFORE updating thread lists
          const oldCached =
            msg.thread_id ? findCachedThread(qc, msg.thread_id) : undefined;

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

            // Surgical unread count for email.received only
            if (msg.event === "email.received") {
              adjustUnreadCount(
                qc,
                emailThread.domain_id,
                unreadContribution(emailThread) - unreadContribution(oldCached)
              );
            }
            // email.sent: no unread count change
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
              // Fallback: also invalidate unread counts
              qc.invalidateQueries({
                queryKey: queryKeys.domains.unreadCounts(),
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
                const hadThread = old.threads.some(
                  (t) => t.id === msg.thread_id
                );

                // If this is the starred view and thread was unstarred, remove it
                if (
                  queryLabel === "starred" &&
                  msg.event === "thread.unstarred" &&
                  hadThread
                ) {
                  return {
                    ...old,
                    total: Math.max(0, old.total - 1),
                    threads: old.threads.filter(
                      (t) => t.id !== msg.thread_id
                    ),
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
                    t.id === msg.thread_id
                      ? { ...t, labels: muteThread.labels || t.labels }
                      : t
                  ),
                };
              }
            );
            qc.setQueryData<Thread>(
              queryKeys.threads.detail(msg.thread_id!),
              (old) =>
                old
                  ? { ...old, labels: muteThread.labels || old.labels }
                  : old
            );
          }
          break;
        }

        case "thread.read":
        case "thread.unread": {
          if (msg.thread_id) {
            const cached = findCachedThread(qc, msg.thread_id);
            const unreadCount = msg.event === "thread.read" ? 0 : 1;

            // Compute delta BEFORE updating cache
            const domainId =
              msg.domain_id || cached?.domain_id;
            if (msg.event === "thread.read") {
              adjustUnreadCount(qc, domainId, -unreadContribution(cached));
            } else {
              adjustUnreadCount(
                qc,
                domainId,
                cached && isInboxEligible(cached)
                  ? 1 - cached.unread_count
                  : 0
              );
            }

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
          break;
        }

        case "thread.archived":
        case "thread.trashed":
        case "thread.spammed":
        case "thread.moved": {
          const thread = (msg.payload?.thread as Thread) || null;
          if (msg.thread_id) {
            // Capture old state BEFORE cache update
            const cached = findCachedThread(qc, msg.thread_id);

            // Surgical unread count adjustment
            adjustUnreadCount(
              qc,
              msg.domain_id || thread?.domain_id || cached?.domain_id,
              unreadContribution(thread) - unreadContribution(cached)
            );

            // For each cached list, check if thread belongs in that view
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

                const hadThread = old.threads.some(
                  (t) => t.id === msg.thread_id
                );

                if (
                  thread &&
                  queryLabel &&
                  threadBelongsInView(thread, queryLabel)
                ) {
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
                      (a, b) =>
                        new Date(b.last_message_at).getTime() -
                        new Date(a.last_message_at).getTime()
                    ),
                  };
                }

                // Thread doesn't belong in this view — remove it
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
          }
          break;
        }

        case "thread.deleted":
          if (msg.thread_id) {
            // Capture old state BEFORE removing from cache
            const cached = findCachedThread(qc, msg.thread_id);
            adjustUnreadCount(
              qc,
              msg.domain_id || cached?.domain_id,
              -unreadContribution(cached)
            );

            qc.setQueriesData<ThreadListResponse>(
              { queryKey: queryKeys.threads.lists() },
              (old) => {
                if (!old) return old;
                const hadThread = old.threads.some(
                  (t) => t.id === msg.thread_id
                );
                if (!hadThread) return old;
                return {
                  ...old,
                  total: Math.max(0, old.total - 1),
                  threads: old.threads.filter(
                    (t) => t.id !== msg.thread_id
                  ),
                };
              }
            );
          }
          break;

        case "thread.bulk_action": {
          const action = msg.payload?.action as string;
          const threadIds = msg.payload?.thread_ids as string[];
          const label = (msg.payload?.label as string) || "";

          if (!action || !threadIds?.length) break;

          // Collect cached threads for delta calculation
          const cachedThreads = new Map<string, Thread>();
          for (const id of threadIds) {
            const cached = findCachedThread(qc, id);
            if (cached) cachedThreads.set(id, cached);
          }

          // Compute per-domain unread deltas
          const deltaByDomain = new Map<string, number>();
          for (const cached of cachedThreads.values()) {
            let newUnread = cached.unread_count;
            let newLabels = cached.labels;

            if (action === "delete") {
              // Thread removed entirely — contribution drops to 0
              const d = -unreadContribution(cached);
              if (d !== 0) {
                deltaByDomain.set(
                  cached.domain_id,
                  (deltaByDomain.get(cached.domain_id) || 0) + d
                );
              }
              continue;
            }

            if (action === "read") newUnread = 0;
            else if (action === "unread") newUnread = 1;

            if (
              action !== "read" &&
              action !== "unread"
            ) {
              newLabels = applyBulkLabels(cached.labels, action, label);
            }

            const newThread = {
              ...cached,
              labels: newLabels,
              unread_count: newUnread,
            };
            const d =
              unreadContribution(newThread) - unreadContribution(cached);
            if (d !== 0) {
              deltaByDomain.set(
                cached.domain_id,
                (deltaByDomain.get(cached.domain_id) || 0) + d
              );
            }
          }

          // Apply unread count deltas
          for (const [domainId, delta] of deltaByDomain) {
            adjustUnreadCount(qc, domainId, delta);
          }

          // Update thread lists
          qc.setQueriesData<ThreadListResponse>(
            { queryKey: queryKeys.threads.lists() },
            (old) => {
              if (!old) return old;

              const cacheEntry = qc
                .getQueryCache()
                .findAll({ queryKey: queryKeys.threads.lists() })
                .find((q) => q.state.data === old);
              const queryLabel = cacheEntry?.queryKey?.[3] as
                | string
                | undefined;
              const queryDomainId = cacheEntry?.queryKey?.[2] as
                | string
                | undefined;

              if (!queryLabel) return old;

              const threadIdSet = new Set(threadIds);

              // Delete: remove from all lists
              if (action === "delete") {
                const filtered = old.threads.filter(
                  (t) => !threadIdSet.has(t.id)
                );
                if (filtered.length === old.threads.length) return old;
                return {
                  ...old,
                  total: Math.max(
                    0,
                    old.total - (old.threads.length - filtered.length)
                  ),
                  threads: filtered,
                };
              }

              // Read/unread: update unread_count in place, no list changes
              if (action === "read" || action === "unread") {
                const hasAffected = old.threads.some((t) =>
                  threadIdSet.has(t.id)
                );
                if (!hasAffected) return old;
                const newUnread = action === "read" ? 0 : 1;
                return {
                  ...old,
                  threads: old.threads.map((t) =>
                    threadIdSet.has(t.id)
                      ? { ...t, unread_count: newUnread }
                      : t
                  ),
                };
              }

              // Label-changing actions: archive, trash, spam, move, mute, unmute, label, unlabel
              const result: Thread[] = [];

              for (const t of old.threads) {
                if (!threadIdSet.has(t.id)) {
                  result.push(t);
                  continue;
                }
                const updated = {
                  ...t,
                  labels: applyBulkLabels(t.labels, action, label),
                };
                if (threadBelongsInView(updated, queryLabel)) {
                  result.push(updated);
                }
              }

              // Add threads from other cached lists that might now belong in this view
              for (const id of threadIds) {
                if (old.threads.some((t) => t.id === id)) continue;
                const cached = cachedThreads.get(id);
                if (!cached) continue;
                if (
                  queryDomainId &&
                  cached.domain_id &&
                  queryDomainId !== cached.domain_id
                )
                  continue;
                const updated = {
                  ...cached,
                  labels: applyBulkLabels(cached.labels, action, label),
                };
                if (threadBelongsInView(updated, queryLabel)) {
                  result.push(updated);
                }
              }

              const totalDiff = result.length - old.threads.length;
              if (totalDiff === 0 && result.every((t, i) => t === old.threads[i])) {
                return old;
              }

              return {
                ...old,
                total: Math.max(0, old.total + totalDiff),
                threads:
                  totalDiff !== 0
                    ? result.sort(
                        (a, b) =>
                          new Date(b.last_message_at).getTime() -
                          new Date(a.last_message_at).getTime()
                      )
                    : result,
              };
            }
          );
          break;
        }

        case "email.status_updated": {
          const emailId = msg.payload?.email_id as string;
          const status = msg.payload?.status as Email["status"];

          // Surgical update: patch the email status in the cached thread detail
          if (msg.thread_id && emailId && status) {
            qc.setQueryData<Thread>(
              queryKeys.threads.detail(msg.thread_id),
              (old) => {
                if (!old?.emails) return old;
                return {
                  ...old,
                  emails: old.emails.map((e) =>
                    e.id === emailId ? { ...e, status } : e
                  ),
                };
              }
            );
          }

          // Show notification on send failure
          if (status === "failed" || status === "bounced" || status === "complained") {
            const subject = msg.payload?.subject as string | undefined;
            const draftId = msg.payload?.draft_id as string | undefined;
            const title =
              status === "bounced" || status === "complained"
                ? "Email bounced"
                : "Email failed";
            const description = subject || "An email failed to deliver";

            if (draftId && msg.domain_id) {
              const domainId = msg.domain_id;
              toast.error(title, {
                description,
                action: {
                  label: "Open drafts",
                  onClick: () => {
                    window.location.href = `/d/${domainId}/drafts`;
                  },
                },
              });
            } else {
              toast.error(title, { description });
            }

            if (
              typeof Notification !== "undefined" &&
              Notification.permission === "granted"
            ) {
              new Notification(title, {
                body: description,
              });
            }
          }
          break;
        }

        case "sync.completed":
          qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
          qc.invalidateQueries({
            queryKey: queryKeys.domains.unreadCounts(),
          });
          break;

        case "plan.changed": {
          const plan = msg.payload?.plan as string | undefined;
          if (plan === "cancelled") {
            toast.warning(
              "Your subscription has been cancelled. Check Settings > Billing for details."
            );
          } else if (plan === "pro") {
            toast.success("Your subscription is now active!");
          }
          break;
        }

        case "domain.disconnected": {
          const domain = msg.payload?.domain as string | undefined;
          const reason = msg.payload?.reason as string | undefined;
          if (reason === "api_key_revoked") {
            toast.error(
              "API key revoked - all domains disconnected. Check Settings."
            );
          } else {
            toast.error(
              `Domain ${domain || ""} has been disconnected from Resend.`
            );
          }
          qc.invalidateQueries({ queryKey: queryKeys.domains.list() });
          break;
        }

        case "domain.reconnected": {
          const domain = msg.payload?.domain as string | undefined;
          toast.success(`Domain ${domain || ""} has reconnected.`);
          qc.invalidateQueries({ queryKey: queryKeys.domains.list() });
          break;
        }

        case "domain.dns_degraded": {
          const domain = msg.payload?.domain as string | undefined;
          const degraded = msg.payload?.degraded as string[] | undefined;
          toast.warning(
            `DNS records changed for ${domain || ""}: ${degraded?.join(", ") || "verification failed"}`
          );
          break;
        }
      }
    });

    return unsub;
  }, [subscribe, setLastEventId, qc]);

  return null;
}
