export const queryKeys = {
  threads: {
    all: ["threads"] as const,
    lists: () => [...queryKeys.threads.all, "list"] as const,
    list: (domainId: string, label: string, page: number) =>
      [...queryKeys.threads.lists(), domainId, label, page] as const,
    details: () => [...queryKeys.threads.all, "detail"] as const,
    detail: (threadId: string) =>
      [...queryKeys.threads.details(), threadId] as const,
  },
  search: {
    all: ["search"] as const,
    results: (domainId: string, query: string, page = 1, scope = "") =>
      [...queryKeys.search.all, domainId, query, page, scope] as const,
  },
  drafts: {
    all: ["drafts"] as const,
    list: (domainId: string) => [...queryKeys.drafts.all, domainId] as const,
  },
  domains: {
    all: ["domains"] as const,
    list: () => [...queryKeys.domains.all, "list"] as const,
    unreadCounts: () => [...queryKeys.domains.all, "unreadCounts"] as const,
    // Prefixed under unreadCounts so every existing invalidation of the
    // unread counts also refreshes the per-label counts.
    labelCounts: (domainId: string) =>
      [...queryKeys.domains.unreadCounts(), "labels", domainId] as const,
  },
  billing: {
    all: ["billing"] as const,
    info: () => [...queryKeys.billing.all, "info"] as const,
  },
};
