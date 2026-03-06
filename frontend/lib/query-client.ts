import { QueryClient, dehydrate, hydrate } from "@tanstack/react-query";
import { ApiError } from "@/lib/api";

const CACHE_KEY = "inboxes-qc";
const PERSIST_PREFIXES = ["threads", "domains", "drafts"];

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000, // 5 min — safety net if WS is down
      gcTime: 30 * 60 * 1000, // 30 minutes
      refetchOnWindowFocus: true,
      retry: (failureCount, error) => {
        // 402 (payment required) is not transient — don't retry
        if (error instanceof ApiError && error.status === 402) return false;
        return failureCount < 1;
      },
    },
  },
});

// Restore cache from localStorage on init
if (typeof window !== "undefined") {
  try {
    const raw = localStorage.getItem(CACHE_KEY);
    if (raw) {
      hydrate(queryClient, JSON.parse(raw));
    }
  } catch {
    localStorage.removeItem(CACHE_KEY);
  }

  // Debounced save on cache changes
  let saveTimer: ReturnType<typeof setTimeout>;
  queryClient.getQueryCache().subscribe(() => {
    clearTimeout(saveTimer);
    saveTimer = setTimeout(() => {
      try {
        const state = dehydrate(queryClient, {
          shouldDehydrateQuery: (q) =>
            q.state.status === "success" &&
            PERSIST_PREFIXES.includes(q.queryKey[0] as string) &&
            q.queryKey[1] !== "unreadCounts",
        });
        localStorage.setItem(CACHE_KEY, JSON.stringify(state));
      } catch {
        // localStorage full or serialization error — ignore
      }
    }, 2000);
  });
}
