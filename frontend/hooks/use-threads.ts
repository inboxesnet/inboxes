import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import { hasLabel } from "@/lib/types";
import type { Thread, ThreadListResponse, Label, UnreadCounts } from "@/lib/types";

const LIMIT = 100;

const SYSTEM_MOVE_TARGETS = ["inbox", "archive", "trash", "spam"];

// findCachedThread returns the first cached copy of a thread from the list caches.
function findCachedThread(qc: ReturnType<typeof useQueryClient>, threadId: string): Thread | undefined {
  for (const query of qc.getQueryCache().findAll({ queryKey: queryKeys.threads.lists() })) {
    const data = query.state.data as ThreadListResponse | undefined;
    const found = data?.threads?.find((t) => t.id === threadId);
    if (found) return found;
  }
  return undefined;
}

// undoOrigin decides where an undo should put a thread back: inbox when it
// was in the inbox before the action, archive otherwise.
function undoOrigin(thread: Thread | undefined): string {
  if (!thread) return "inbox";
  return hasLabel(thread, "inbox") && !hasLabel(thread, "trash") && !hasLabel(thread, "spam")
    ? "inbox"
    : "archive";
}

export function useThreadList(domainId: string, label: Label, page: number) {
  return useQuery({
    queryKey: queryKeys.threads.list(domainId, label, page),
    queryFn: () =>
      api.get<ThreadListResponse>(
        `/api/threads?domain_id=${domainId}&label=${label}&page=${page}&limit=${LIMIT}`
      ),
  });
}

export function useThread(threadId: string) {
  return useQuery({
    queryKey: queryKeys.threads.detail(threadId),
    queryFn: () =>
      api.get<{ thread: Thread }>(`/api/threads/${threadId}`).then(
        (d) => d.thread
      ),
  });
}

export function toggleStarredLabel(labels: string[]): string[] {
  if (labels.includes("starred")) {
    return labels.filter((l) => l !== "starred");
  }
  return [...labels, "starred"];
}

export function toggleMutedLabel(labels: string[]): string[] {
  if (labels.includes("muted")) {
    return labels.filter((l) => l !== "muted");
  }
  return [...labels, "muted"];
}

export function useStarThread() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({ threadId, starred }: { threadId: string; starred: boolean }) =>
      api.patch(`/api/threads/${threadId}/star`, { starred }),
    onMutate: async ({ threadId, starred }) => {
      await qc.cancelQueries({ queryKey: queryKeys.threads.all });

      const setLabels = (labels: string[]) =>
        starred
          ? labels.includes("starred") ? labels : [...labels, "starred"]
          : labels.filter((l) => l !== "starred");

      // Optimistic: update all thread lists
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

          return {
            ...old,
            threads: old.threads
              .map((t) =>
                t.id === threadId
                  ? { ...t, labels: setLabels(t.labels || []) }
                  : t
              )
              .filter((t) => {
                if (queryLabel === "starred" && t.id === threadId && !starred) {
                  return false;
                }
                return true;
              }),
            total:
              queryLabel === "starred" && !starred &&
              old.threads.some((t) => t.id === threadId)
                ? old.total - 1
                : old.total,
          };
        }
      );

      // Optimistic: update search caches
      qc.setQueriesData<{ threads: Thread[] }>(
        { queryKey: queryKeys.search.all },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            threads: old.threads.map((t) =>
              t.id === threadId
                ? { ...t, labels: setLabels(t.labels || []) }
                : t
            ),
          };
        }
      );

      // Optimistic: update thread detail
      qc.setQueryData<Thread>(
        queryKeys.threads.detail(threadId),
        (old) => (old ? { ...old, labels: setLabels(old.labels || []) } : old)
      );
    },
    onError: () => {
      toast.error("Failed to update star");
      qc.invalidateQueries({ queryKey: queryKeys.threads.all });
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
    },
  });
}

export function useMuteThread() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (threadId: string) =>
      api.patch(`/api/threads/${threadId}/mute`),
    onMutate: async (threadId) => {
      await qc.cancelQueries({ queryKey: queryKeys.threads.all });

      qc.setQueriesData<ThreadListResponse>(
        { queryKey: queryKeys.threads.lists() },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            threads: old.threads.map((t) =>
              t.id === threadId ? { ...t, labels: toggleMutedLabel(t.labels || []) } : t
            ),
          };
        }
      );

      // Optimistic: update search caches
      qc.setQueriesData<{ threads: Thread[] }>(
        { queryKey: queryKeys.search.all },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            threads: old.threads.map((t) =>
              t.id === threadId
                ? { ...t, labels: toggleMutedLabel(t.labels || []) }
                : t
            ),
          };
        }
      );

      qc.setQueryData<Thread>(
        queryKeys.threads.detail(threadId),
        (old) => (old ? { ...old, labels: toggleMutedLabel(old.labels || []) } : old)
      );
    },
    onError: () => {
      toast.error("Failed to update mute");
      qc.invalidateQueries({ queryKey: queryKeys.threads.all });
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
    },
  });
}

export function useThreadAction() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      threadId,
      action,
    }: {
      threadId: string;
      action: string;
    }) => {
      if (action.startsWith("move:")) {
        const label = action.split(":")[1];
        return api.patch(`/api/threads/${threadId}/move`, { label });
      }
      if (action === "delete") {
        return api.delete(`/api/threads/${threadId}`);
      }
      if (action === "dismiss") {
        // Dismiss is email-level; the bulk route fans it out to every
        // failed email on the thread.
        return api.patch(`/api/threads/bulk`, { thread_ids: [threadId], action: "dismiss" });
      }
      return api.patch(`/api/threads/${threadId}/${action}`);
    },
    onMutate: async ({ threadId, action }) => {
      await qc.cancelQueries({ queryKey: queryKeys.threads.all });

      const movingActions = ["archive", "trash", "spam", "delete", "move:deleted_forever"];
      const isMoving = movingActions.includes(action) || action.startsWith("move:");

      // Capture where an undo should restore the thread, before caches change.
      const undoTarget = undoOrigin(findCachedThread(qc, threadId));

      // Adjust unread counts BEFORE modifying cached threads (WSSync will see delta=0)
      if (action === "read" || action === "unread" || isMoving) {
        const allQueries = qc.getQueryCache().findAll({ queryKey: queryKeys.threads.lists() });
        let thread: Thread | undefined;
        for (const query of allQueries) {
          const data = query.state.data as ThreadListResponse | undefined;
          const found = data?.threads?.find((t) => t.id === threadId);
          if (found) { thread = found; break; }
        }
        if (thread && hasLabel(thread, "inbox") && !hasLabel(thread, "trash") && !hasLabel(thread, "spam")) {
          let delta = 0;
          if (action === "read") delta = -thread.unread_count;
          else if (action === "unread") delta = 1 - thread.unread_count;
          else if (isMoving) delta = -thread.unread_count;
          if (delta !== 0) {
            qc.setQueryData<UnreadCounts>(queryKeys.domains.unreadCounts(), (old) => {
              if (!old) return old;
              return { ...old, [thread!.domain_id]: Math.max(0, (old[thread!.domain_id] || 0) + delta) };
            });
          }
        }
      }

      // Dismiss removes the thread from the Failed view only — every other
      // list keeps it.
      if (action === "dismiss") {
        for (const query of qc.getQueryCache().findAll({ queryKey: queryKeys.threads.lists() })) {
          if (query.queryKey[3] !== "failed") continue;
          qc.setQueryData<ThreadListResponse>(query.queryKey, (old) =>
            old
              ? {
                  ...old,
                  threads: old.threads.filter((t) => t.id !== threadId),
                  total: Math.max(0, old.total - 1),
                }
              : old
          );
        }
      }

      // Optimistic: update all thread lists
      qc.setQueriesData<ThreadListResponse>(
        { queryKey: queryKeys.threads.lists() },
        (old) => {
          if (!old) return old;
          if (isMoving) {
            return {
              ...old,
              threads: old.threads.filter((t) => t.id !== threadId),
              total: old.total - 1,
            };
          }
          if (action === "read") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                t.id === threadId ? { ...t, unread_count: 0 } : t
              ),
            };
          }
          if (action === "unread") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                t.id === threadId ? { ...t, unread_count: 1 } : t
              ),
            };
          }
          if (action === "mute") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                t.id === threadId ? { ...t, labels: toggleMutedLabel(t.labels || []) } : t
              ),
            };
          }
          return old;
        }
      );

      // Optimistic: update search caches
      qc.setQueriesData<{ threads: Thread[] }>(
        { queryKey: queryKeys.search.all },
        (old) => {
          if (!old) return old;
          if (isMoving) {
            return { ...old, threads: old.threads.filter((t) => t.id !== threadId) };
          }
          if (action === "read") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                t.id === threadId ? { ...t, unread_count: 0 } : t
              ),
            };
          }
          if (action === "unread") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                t.id === threadId ? { ...t, unread_count: 1 } : t
              ),
            };
          }
          if (action === "mute") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                t.id === threadId
                  ? { ...t, labels: toggleMutedLabel(t.labels || []) }
                  : t
              ),
            };
          }
          return old;
        }
      );

      // Optimistic: update thread detail cache
      if (action === "mute") {
        qc.setQueryData<Thread>(
          queryKeys.threads.detail(threadId),
          (old) => (old ? { ...old, labels: toggleMutedLabel(old.labels || []) } : old)
        );
      }
      if (action === "read") {
        qc.setQueryData<Thread>(
          queryKeys.threads.detail(threadId),
          (old) => (old ? { ...old, unread_count: 0 } : old)
        );
      } else if (action === "unread") {
        qc.setQueryData<Thread>(
          queryKeys.threads.detail(threadId),
          (old) => (old ? { ...old, unread_count: 1 } : old)
        );
      } else if (isMoving) {
        // Remove stale detail cache for moved/deleted threads
        qc.removeQueries({ queryKey: queryKeys.threads.detail(threadId) });
      }

      return { undoTarget };
    },
    onError: (_err, { action }) => {
      const label = action === "archive" ? "archive" : action === "trash" ? "trash" : action;
      toast.error(`Failed to ${label} thread`);
      qc.invalidateQueries({ queryKey: queryKeys.threads.all });
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    },
    onSuccess: (_data, { threadId, action }, context) => {
      const undoTarget = context?.undoTarget || "inbox";
      const refresh = () => {
        qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
        qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
        qc.invalidateQueries({ queryKey: queryKeys.search.all });
      };

      // Reconcile the sidebar badges with the server after every action.
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });

      let label: string | null = null;
      let undo: (() => Promise<unknown>) | null = null;

      if (action === "archive" || action === "trash" || action === "spam") {
        label =
          action === "archive" ? "Archived" : action === "trash" ? "Moved to trash" : "Marked as spam";
        undo = () => api.patch(`/api/threads/${threadId}/move`, { label: undoTarget });
      } else if (action.startsWith("move:")) {
        const target = action.split(":")[1];
        if (target === "deleted_forever") return;
        label = `Moved to ${target}`;
        undo = SYSTEM_MOVE_TARGETS.includes(target)
          ? () => api.patch(`/api/threads/${threadId}/move`, { label: undoTarget })
          : () =>
              api.patch("/api/threads/bulk", {
                thread_ids: [threadId],
                action: "unlabel",
                label: target,
              });
      }

      if (label && undo) {
        const doUndo = undo;
        toast(label, {
          action: {
            label: "Undo",
            onClick: () => {
              doUndo()
                .then(refresh)
                .catch(() => toast.error("Failed to undo"));
            },
          },
        });
      }
    },
  });
}

export function useBulkAction() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      threadIds,
      action,
      label,
      selectAll,
      filterLabel,
      filterDomainId,
    }: {
      threadIds: string[];
      action: string;
      label?: string;
      selectAll?: boolean;
      filterLabel?: string;
      filterDomainId?: string;
    }) =>
      api.patch("/api/threads/bulk", {
        thread_ids: threadIds,
        action,
        label,
        select_all: selectAll,
        filter_label: filterLabel,
        filter_domain_id: filterDomainId,
      }),
    onMutate: async ({ threadIds, action }) => {
      await qc.cancelQueries({ queryKey: queryKeys.threads.all });

      const movingActions = ["archive", "trash", "spam", "move", "delete"];

      // Dismiss removes the selected threads from the Failed view only.
      if (action === "dismiss") {
        for (const query of qc.getQueryCache().findAll({ queryKey: queryKeys.threads.lists() })) {
          if (query.queryKey[3] !== "failed") continue;
          qc.setQueryData<ThreadListResponse>(query.queryKey, (old) => {
            if (!old) return old;
            const remaining = old.threads.filter((t) => !threadIds.includes(t.id));
            return {
              ...old,
              threads: remaining,
              total: Math.max(0, old.total - (old.threads.length - remaining.length)),
            };
          });
        }
      }

      // Capture where an undo should restore the threads. When any selected
      // thread was in the inbox, undo restores the batch to the inbox.
      let undoTarget = "archive";
      for (const id of threadIds) {
        if (undoOrigin(findCachedThread(qc, id)) === "inbox") {
          undoTarget = "inbox";
          break;
        }
      }
      if (threadIds.length === 0) undoTarget = "inbox";

      // Adjust unread counts BEFORE modifying cached threads (WSSync will see delta=0)
      if (action === "read" || action === "unread" || movingActions.includes(action)) {
        const seen = new Set<string>();
        const deltaByDomain: Record<string, number> = {};
        for (const query of qc.getQueryCache().findAll({ queryKey: queryKeys.threads.lists() })) {
          const data = query.state.data as ThreadListResponse | undefined;
          if (!data?.threads) continue;
          for (const t of data.threads) {
            if (seen.has(t.id) || !threadIds.includes(t.id)) continue;
            seen.add(t.id);
            if (!hasLabel(t, "inbox") || hasLabel(t, "trash") || hasLabel(t, "spam")) continue;
            let delta = 0;
            if (action === "read") delta = -t.unread_count;
            else if (action === "unread") delta = 1 - t.unread_count;
            else delta = -t.unread_count;
            if (delta !== 0) deltaByDomain[t.domain_id] = (deltaByDomain[t.domain_id] || 0) + delta;
          }
        }
        if (Object.keys(deltaByDomain).length > 0) {
          qc.setQueryData<UnreadCounts>(queryKeys.domains.unreadCounts(), (old) => {
            if (!old) return old;
            const updated = { ...old };
            for (const [domainId, delta] of Object.entries(deltaByDomain)) {
              updated[domainId] = Math.max(0, (updated[domainId] || 0) + delta);
            }
            return updated;
          });
        }
      }

      // Optimistic: update all thread lists
      qc.setQueriesData<ThreadListResponse>(
        { queryKey: queryKeys.threads.lists() },
        (old) => {
          if (!old) return old;
          if (movingActions.includes(action)) {
            const filtered = old.threads.filter(
              (t) => !threadIds.includes(t.id)
            );
            return {
              ...old,
              threads: filtered,
              total: old.total - (old.threads.length - filtered.length),
            };
          }
          if (action === "read") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id) ? { ...t, unread_count: 0 } : t
              ),
            };
          }
          if (action === "unread") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id) ? { ...t, unread_count: 1 } : t
              ),
            };
          }
          if (action === "mute") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id)
                  ? { ...t, labels: [...(t.labels || []).filter((l) => l !== "muted"), "muted"] }
                  : t
              ),
            };
          }
          if (action === "unmute") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id)
                  ? { ...t, labels: (t.labels || []).filter((l) => l !== "muted") }
                  : t
              ),
            };
          }
          return old;
        }
      );

      // Optimistic: update search caches
      qc.setQueriesData<{ threads: Thread[] }>(
        { queryKey: queryKeys.search.all },
        (old) => {
          if (!old) return old;
          if (movingActions.includes(action)) {
            return {
              ...old,
              threads: old.threads.filter((t) => !threadIds.includes(t.id)),
            };
          }
          if (action === "read") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id) ? { ...t, unread_count: 0 } : t
              ),
            };
          }
          if (action === "unread") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id) ? { ...t, unread_count: 1 } : t
              ),
            };
          }
          if (action === "mute") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id)
                  ? { ...t, labels: [...(t.labels || []).filter((l) => l !== "muted"), "muted"] }
                  : t
              ),
            };
          }
          if (action === "unmute") {
            return {
              ...old,
              threads: old.threads.map((t) =>
                threadIds.includes(t.id)
                  ? { ...t, labels: (t.labels || []).filter((l) => l !== "muted") }
                  : t
              ),
            };
          }
          return old;
        }
      );

      // Optimistic: update each thread's detail cache
      for (const id of threadIds) {
        if (action === "read") {
          qc.setQueryData<Thread>(
            queryKeys.threads.detail(id),
            (old) => (old ? { ...old, unread_count: 0 } : old)
          );
        } else if (action === "unread") {
          qc.setQueryData<Thread>(
            queryKeys.threads.detail(id),
            (old) => (old ? { ...old, unread_count: 1 } : old)
          );
        } else if (movingActions.includes(action)) {
          qc.removeQueries({ queryKey: queryKeys.threads.detail(id) });
        }
      }

      return { undoTarget };
    },
    onError: (_err, { action }) => {
      toast.error(`Failed to ${action} threads`);
      qc.invalidateQueries({ queryKey: queryKeys.threads.all });
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    },
    onSuccess: (_data, { threadIds, action, label: moveLabel }, context) => {
      const undoTarget = context?.undoTarget || "inbox";
      const count = threadIds.length;
      const plural = `${count} conversation${count > 1 ? "s" : ""}`;
      const refresh = () => {
        qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
        qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
        qc.invalidateQueries({ queryKey: queryKeys.search.all });
      };

      // Always reconcile with the server. The optimistic deltas above only
      // cover threads present in the cache, so the sidebar badges drift when
      // the selection spans threads outside the cached pages.
      refresh();

      let text: string | null = null;
      let undo: (() => Promise<unknown>) | null = null;

      if (action === "archive" || action === "trash" || action === "spam") {
        const verb =
          action === "archive" ? "Archived" : action === "trash" ? "Moved to trash" : "Marked as spam";
        text = `${verb} ${plural}`;
        undo = () =>
          api.patch("/api/threads/bulk", {
            thread_ids: threadIds,
            action: "move",
            label: undoTarget,
          });
      } else if (action === "move" && moveLabel && moveLabel !== "deleted_forever") {
        text = `Moved ${plural} to ${moveLabel}`;
        undo = SYSTEM_MOVE_TARGETS.includes(moveLabel)
          ? () =>
              api.patch("/api/threads/bulk", {
                thread_ids: threadIds,
                action: "move",
                label: undoTarget,
              })
          : () =>
              api.patch("/api/threads/bulk", {
                thread_ids: threadIds,
                action: "unlabel",
                label: moveLabel,
              });
      }

      if (text && undo && count > 0) {
        const doUndo = undo;
        toast(text, {
          action: {
            label: "Undo",
            onClick: () => {
              doUndo()
                .then(refresh)
                .catch(() => toast.error("Failed to undo"));
            },
          },
        });
      }
    },
  });
}
