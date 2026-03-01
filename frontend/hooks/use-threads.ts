import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import { hasLabel } from "@/lib/types";
import type { Thread, ThreadListResponse, Label } from "@/lib/types";

const LIMIT = 100;

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
      return api.patch(`/api/threads/${threadId}/${action}`);
    },
    onMutate: async ({ threadId, action }) => {
      await qc.cancelQueries({ queryKey: queryKeys.threads.all });

      const movingActions = ["archive", "trash", "spam", "delete", "move:deleted_forever"];
      const isMoving = movingActions.includes(action) || action.startsWith("move:");

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
    },
    onError: (_err, { action }) => {
      const label = action === "archive" ? "archive" : action === "trash" ? "trash" : action;
      toast.error(`Failed to ${label} thread`);
      qc.invalidateQueries({ queryKey: queryKeys.threads.all });
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    },
    onSuccess: (_data, { threadId, action }) => {
      if (action === "archive" || action === "trash") {
        const label = action === "archive" ? "Archived" : "Moved to trash";
        toast(label, {
          action: {
            label: "Undo",
            onClick: () => {
              api.patch(`/api/threads/${threadId}/move`, { label: "inbox" }).then(() => {
                qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
                qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
              }).catch(() => toast.error("Failed to undo"));
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
    },
    onError: (_err, { action }) => {
      toast.error(`Failed to ${action} threads`);
      qc.invalidateQueries({ queryKey: queryKeys.threads.all });
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    },
    onSuccess: (_data, { threadIds, action }) => {
      if (action === "archive" || action === "trash") {
        const label = action === "archive" ? "Archived" : "Moved to trash";
        const count = threadIds.length;
        toast(`${label} ${count} conversation${count > 1 ? "s" : ""}`, {
          action: {
            label: "Undo",
            onClick: () => {
              api.patch("/api/threads/bulk", {
                thread_ids: threadIds,
                action: "move",
                label: "inbox",
              }).then(() => {
                qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
                qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
              }).catch(() => toast.error("Failed to undo"));
            },
          },
        });
      }
    },
  });
}
