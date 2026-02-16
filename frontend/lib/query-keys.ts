export const queryKeys = {
  threads: {
    all: ["threads"] as const,
    lists: () => [...queryKeys.threads.all, "list"] as const,
    list: (domainId: string, folder: string, page: number) =>
      [...queryKeys.threads.lists(), domainId, folder, page] as const,
    details: () => [...queryKeys.threads.all, "detail"] as const,
    detail: (threadId: string) =>
      [...queryKeys.threads.details(), threadId] as const,
  },
  drafts: {
    all: ["drafts"] as const,
    list: (domainId: string) => [...queryKeys.drafts.all, domainId] as const,
  },
  domains: {
    all: ["domains"] as const,
    list: () => [...queryKeys.domains.all, "list"] as const,
    unreadCounts: () => [...queryKeys.domains.all, "unreadCounts"] as const,
  },
};
