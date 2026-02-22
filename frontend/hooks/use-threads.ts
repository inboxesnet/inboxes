import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import type { Thread, ThreadListResponse, Folder } from "@/lib/types";

const LIMIT = 100;

export function useThreadList(domainId: string, folder: Folder, page: number) {
  return useQuery({
    queryKey: queryKeys.threads.list(domainId, folder, page),
    queryFn: () =>
      api.get<ThreadListResponse>(
        `/api/threads?domain_id=${domainId}&folder=${folder}&page=${page}&limit=${LIMIT}`
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

export function useStarThread() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: (threadId: string) =>
      api.patch(`/api/threads/${threadId}/star`),
    onMutate: async (threadId) => {
      // Cancel outgoing refetches
      await qc.cancelQueries({ queryKey: queryKeys.threads.all });

      // Optimistic update across all thread lists
      qc.setQueriesData<ThreadListResponse>(
        { queryKey: queryKeys.threads.lists() },
        (old) => {
          if (!old) return old;
          return {
            ...old,
            threads: old.threads.map((t) =>
              t.id === threadId ? { ...t, starred: !t.starred } : t
            ),
          };
        }
      );

      // Optimistic update for thread detail
      qc.setQueryData<Thread>(
        queryKeys.threads.detail(threadId),
        (old) => (old ? { ...old, starred: !old.starred } : old)
      );
    },
    onError: () => {
      qc.invalidateQueries({ queryKey: queryKeys.threads.all });
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
        const folder = action.split(":")[1];
        return api.patch(`/api/threads/${threadId}/move`, { folder });
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
          return old;
        }
      );
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    },
  });
}

export function useBulkAction() {
  const qc = useQueryClient();

  return useMutation({
    mutationFn: ({
      threadIds,
      action,
      folder,
    }: {
      threadIds: string[];
      action: string;
      folder?: string;
    }) =>
      api.patch("/api/threads/bulk", {
        thread_ids: threadIds,
        action,
        folder,
      }),
    onMutate: async ({ threadIds, action }) => {
      await qc.cancelQueries({ queryKey: queryKeys.threads.all });

      const movingActions = ["archive", "trash", "spam", "move", "delete"];

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
          return old;
        }
      );
    },
    onSettled: () => {
      qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
      qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    },
  });
}
