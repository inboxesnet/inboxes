"use client";

import { Suspense, useState, useCallback, useMemo, useEffect, useRef } from "react";
import { useParams, usePathname, useRouter, useSearchParams } from "next/navigation";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { useThreadList, useStarThread, useThreadAction, useBulkAction } from "@/hooks/use-threads";
import { useThreadSelection } from "@/hooks/use-thread-selection";
import { useNotifications } from "@/contexts/notification-context";
import { useSyncJob } from "@/hooks/use-sync-job";
import { ThreadList } from "@/components/thread-list";
import { ThreadToolbar } from "@/components/thread-toolbar";
import { ThreadView } from "@/components/thread-view";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { Skeleton } from "@/components/ui/skeleton";
import { ThreadListProvider } from "@/contexts/thread-list-context";
import { useUnreadBadge } from "@/hooks/use-unread-badge";
import { queryKeys } from "@/lib/query-keys";
import { api } from "@/lib/api";
import { toast } from "sonner";
import { Search, X } from "lucide-react";
import type { Label, Thread } from "@/lib/types";

const LIMIT = 100;

const EMPTY_MESSAGES: Record<Label, string> = {
  inbox: "Your inbox is empty",
  sent: "No sent messages",
  drafts: "No drafts",
  archive: "No archived messages",
  starred: "No starred messages",
  snoozed: "No snoozed conversations",
  trash: "Trash is empty",
  spam: "No spam messages",
  failed: "No failed sends",
  deleted_forever: "No deleted messages",
};

function getThreadLabel(thread: Thread): Label {
  const labels = thread.labels || [];
  if (labels.includes("trash")) return "trash";
  if (labels.includes("spam")) return "spam";
  if (labels.includes("archive")) return "archive";
  if (labels.includes("sent")) return "sent";
  return "inbox";
}

interface ThreadListPageProps {
  label: Label;
  title: string;
  subtitle?: string;
}

// The URL owns the view state: ?thread= (open thread), ?q= (search),
// ?page=, ?scope=folder (search scope). Refresh and Back now work.
export function ThreadListPage(props: ThreadListPageProps) {
  return (
    <Suspense
      fallback={
        <div className="flex items-center justify-center h-full">
          <Spinner className="h-6 w-6" />
        </div>
      }
    >
      <ThreadListPageInner {...props} />
    </Suspense>
  );
}

function ThreadListPageInner({ label, title, subtitle }: ThreadListPageProps) {
  const params = useParams();
  const domainId = params.domainId as string;
  const qc = useQueryClient();
  const router = useRouter();
  const pathname = usePathname();
  const searchParams = useSearchParams();

  const page = Math.max(1, parseInt(searchParams.get("page") || "1", 10) || 1);
  const searchQuery = searchParams.get("q") || "";
  const scopeFolder = searchParams.get("scope") === "folder";
  const selectedThreadId = searchParams.get("thread");

  const updateParams = useCallback(
    (updates: Record<string, string | null>, push = false) => {
      const sp = new URLSearchParams(searchParams.toString());
      for (const [key, value] of Object.entries(updates)) {
        if (value === null || value === "") sp.delete(key);
        else sp.set(key, value);
      }
      const qs = sp.toString();
      const url = qs ? `${pathname}?${qs}` : pathname;
      if (push) router.push(url, { scroll: false });
      else router.replace(url, { scroll: false });
    },
    [searchParams, pathname, router]
  );

  const [focusedIndex, setFocusedIndex] = useState(-1);
  const [searchInput, setSearchInput] = useState(searchQuery);
  const searchInputRef = useRef<HTMLInputElement>(null);
  const listScrollRef = useRef<HTMLDivElement>(null);
  const scrollPositionRef = useRef(0);
  const [selectAllPages, setSelectAllPages] = useState(false);

  // Keep the input in sync when the query changes via URL navigation
  useEffect(() => {
    setSearchInput(searchQuery);
  }, [searchQuery]);

  const { data, isLoading, isFetching } = useThreadList(domainId, label, page);
  const threads = data?.threads ?? [];
  const total = data?.total ?? 0;

  const searchScope = scopeFolder ? label : "";
  const { data: searchData, isFetching: searchFetching, isError: searchError, refetch: searchRefetch } = useQuery({
    queryKey: queryKeys.search.results(domainId, searchQuery, page, searchScope),
    queryFn: () =>
      api.get<{ threads: Thread[]; total: number; limit: number }>(
        `/api/emails/search?q=${encodeURIComponent(searchQuery)}&domain_id=${domainId}&page=${page}${searchScope ? `&label=${encodeURIComponent(searchScope)}` : ""}`
      ),
    enabled: searchQuery.length > 0,
  });
  const searchResults = searchData?.threads ?? [];
  const searchTotal = searchData?.total ?? 0;
  const searchLimit = searchData?.limit ?? 50;

  const isSearching = searchQuery.length > 0;
  const activeThreads = isSearching ? searchResults : threads;

  const threadIds = useMemo(() => activeThreads.map((t) => t.id), [activeThreads]);
  const selection = useThreadSelection(threadIds);
  const clearSelectionRef = useRef(selection.clearSelection);
  clearSelectionRef.current = selection.clearSelection;

  const starMutation = useStarThread();
  const actionMutation = useThreadAction();
  const bulkMutation = useBulkAction();

  const { connected: wsConnected } = useNotifications();
  const { startSync, isRunning: syncRunning, isComplete: syncComplete, result: syncResult } = useSyncJob();
  const { data: meData } = useQuery({
    queryKey: queryKeys.users.me(),
    queryFn: () => api.get<{ has_webhook: boolean }>("/api/users/me"),
    staleTime: Infinity,
  });
  const hasWebhook = meData?.has_webhook ?? false;

  // Update page title + favicon badge
  useUnreadBadge(title, domainId);

  // Reset selection and focus when domain/label changes. The URL params
  // (page/thread/search) already reset on navigation.
  useEffect(() => {
    clearSelectionRef.current();
    setFocusedIndex(-1);
    setSelectAllPages(false);
  }, [domainId, label]);

  // Reset selection and focus when search changes
  useEffect(() => {
    clearSelectionRef.current();
    setFocusedIndex(-1);
    setSelectAllPages(false);
  }, [searchQuery]);

  // Listen for focus-search custom event (from keyboard shortcuts)
  useEffect(() => {
    function handleFocusSearch() {
      searchInputRef.current?.focus();
    }
    window.addEventListener("focus-search", handleFocusSearch);
    return () => window.removeEventListener("focus-search", handleFocusSearch);
  }, []);

  const invalidateCaches = useCallback(() => {
    qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
    qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    if (isSearching) {
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
    }
  }, [qc, isSearching]);

  const handleRefresh = useCallback(() => {
    if (hasWebhook && wsConnected) {
      invalidateCaches();
    } else {
      startSync();
    }
  }, [hasWebhook, wsConnected, invalidateCaches, startSync]);

  // Invalidate caches and toast when sync completes
  const prevSyncComplete = useRef(false);
  useEffect(() => {
    if (syncComplete && !prevSyncComplete.current) {
      invalidateCaches();
      toast.success("Sync complete");
    }
    prevSyncComplete.current = syncComplete;
  }, [syncComplete, invalidateCaches]);

  const handlePageChange = useCallback(
    (newPage: number) => {
      selection.clearSelection();
      setFocusedIndex(-1);
      setSelectAllPages(false);
      updateParams({ page: newPage <= 1 ? null : String(newPage), thread: null }, true);
    },
    [selection.clearSelection, updateParams]
  );

  const handleStar = useCallback(
    (threadId: string) => {
      const thread = threads.find((t) => t.id === threadId);
      starMutation.mutate({ threadId, starred: !thread?.labels?.includes("starred") });
    },
    [starMutation, threads]
  );

  const handleAction = useCallback(
    (threadId: string, action: string) => {
      actionMutation.mutate({ threadId, action });
      // Close reading pane when thread is moved/archived/trashed
      const closingActions = ["archive", "trash", "spam", "delete"];
      if ((closingActions.includes(action) || action.startsWith("move:")) && threadId === selectedThreadId) {
        updateParams({ thread: null });
      }
    },
    [actionMutation, selectedThreadId, updateParams]
  );

  const handleThreadClick = useCallback(
    (threadId: string) => {
      // Save scroll position before opening thread
      if (listScrollRef.current) {
        scrollPositionRef.current = listScrollRef.current.scrollTop;
      }
      // Push, so browser Back closes the reading pane
      updateParams({ thread: threadId }, true);
    },
    [updateParams]
  );

  // Restore scroll position when returning from thread view
  useEffect(() => {
    if (!selectedThreadId && listScrollRef.current && scrollPositionRef.current > 0) {
      listScrollRef.current.scrollTop = scrollPositionRef.current;
    }
  }, [selectedThreadId]);

  const handleToggleSelectAllPages = useCallback(() => {
    if (selectAllPages) {
      // Clear select-all-pages mode
      setSelectAllPages(false);
      selection.clearSelection();
    } else {
      // Enable select-all-pages mode (select all visible first)
      selection.selectIds(activeThreads.map((t) => t.id));
      setSelectAllPages(true);
    }
  }, [selectAllPages, selection, activeThreads]);

  const handleBulkAction = useCallback(
    (actionStr: string) => {
      const ids = Array.from(selection.selectedIds);
      if (ids.length === 0 && !selectAllPages) return;

      let action = actionStr;
      let moveFolder: string | undefined;

      if (actionStr.startsWith("move:")) {
        action = "move";
        moveFolder = actionStr.split(":")[1];
      } else if (actionStr.startsWith("label:")) {
        action = "label";
        moveFolder = actionStr.split(":")[1];
      } else if (actionStr.startsWith("unlabel:")) {
        action = "unlabel";
        moveFolder = actionStr.split(":")[1];
      }

      // Clear selection only for actions that remove threads from the current view
      const removesFromView = ["archive", "trash", "spam", "delete", "move", "dismiss"].includes(action);
      bulkMutation.mutate(
        {
          threadIds: ids,
          action,
          label: moveFolder,
          selectAll: selectAllPages || undefined,
          filterLabel: selectAllPages ? label : undefined,
          filterDomainId: selectAllPages ? domainId : undefined,
        },
        {
          onSuccess: () => {
            if (removesFromView) {
              selection.clearSelection();
              setSelectAllPages(false);
            }
          },
        }
      );
    },
    [selection.selectedIds, selection.clearSelection, bulkMutation, selectAllPages, label, domainId]
  );

  const handleMarkAllRead = useCallback(() => {
    bulkMutation.mutate(
      {
        threadIds: [],
        action: "read",
        selectAll: true,
        filterLabel: label,
        filterDomainId: domainId,
      },
      { onSuccess: () => toast.success("Marked all as read") }
    );
  }, [bulkMutation, label, domainId]);

  const handleSnooze = useCallback(
    async (until: string | null) => {
      const ids = Array.from(selection.selectedIds);
      if (ids.length === 0) return;
      try {
        await Promise.all(
          ids.map((id) => api.patch(`/api/threads/${id}/snooze`, { until }))
        );
        toast.success(
          until
            ? `Snoozed ${ids.length} conversation${ids.length === 1 ? "" : "s"} until ${new Date(until).toLocaleString(undefined, { weekday: "short", hour: "numeric", minute: "2-digit" })}`
            : "Unsnoozed"
        );
        selection.clearSelection();
        qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
        qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
      } catch {
        toast.error("Failed to snooze");
      }
    },
    [selection.selectedIds, selection.clearSelection, qc]
  );

  const handleEmptyFolder = useCallback(() => {
    bulkMutation.mutate(
      {
        threadIds: [],
        action: "delete",
        selectAll: true,
        filterLabel: label,
        filterDomainId: domainId,
      },
      {
        onSuccess: () => {
          toast.success(label === "trash" ? "Trash emptied" : "Spam emptied");
          qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
        },
      }
    );
  }, [bulkMutation, label, domainId, qc]);

  if (isLoading) {
    return (
      <div className="h-full flex flex-col">
        <div className="h-14 flex items-center pl-14 md:pl-4 pr-4 border-b shrink-0">
          <Skeleton className="h-8 w-full max-w-md" />
        </div>
        <div className="h-10 flex items-center gap-3 px-4 border-b shrink-0">
          <Skeleton className="h-4 w-4" />
          <Skeleton className="h-4 w-28" />
        </div>
        <div className="flex-1 overflow-hidden divide-y">
          {Array.from({ length: 10 }).map((_, i) => (
            <div key={i} className="flex items-center gap-3 px-3 h-14 md:h-10">
              <Skeleton className="h-3.5 w-3.5 shrink-0" />
              <Skeleton className="h-3.5 w-3.5 shrink-0" />
              <Skeleton className="h-3.5 w-40 shrink-0" />
              <Skeleton className="h-3.5 flex-1" />
              <Skeleton className="h-3.5 w-12 shrink-0" />
            </div>
          ))}
        </div>
      </div>
    );
  }

  const refreshing = isFetching && !isLoading;

  const listPane = (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="h-14 flex print:hidden items-center pl-14 md:pl-4 pr-4 border-b shrink-0 gap-2">
        <form
          className="flex-1 flex items-center gap-2 max-w-md"
          onSubmit={(e) => {
            e.preventDefault();
            updateParams(
              { q: searchInput.trim() || null, page: null, thread: null },
              true
            );
          }}
        >
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              ref={searchInputRef}
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder="Search mail"
              title="Operators: from:  to:  has:attachment  before:2026-01-01  after:2026-01-01"
              className="h-8 pl-8 bg-muted border-transparent focus-visible:ring-1 focus-visible:ring-offset-0 focus-visible:bg-background"
            />
          </div>
          {searchQuery && (
            <>
              <button
                type="button"
                onClick={() =>
                  updateParams({ scope: scopeFolder ? null : "folder", page: null })
                }
                className={`text-xs whitespace-nowrap rounded-full border px-2 py-0.5 transition-colors ${scopeFolder ? "bg-primary text-primary-foreground border-primary" : "text-muted-foreground hover:text-foreground"}`}
                title={scopeFolder ? "Searching this folder only" : "Searching all mail"}
              >
                {scopeFolder ? `Only ${title}` : "All mail"}
              </button>
              <button
                type="button"
                onClick={() => {
                  setSearchInput("");
                  updateParams({ q: null, scope: null, page: null });
                }}
                className="text-muted-foreground hover:text-foreground"
              >
                <X className="h-4 w-4" />
              </button>
            </>
          )}
        </form>
      </div>

      {/* Toolbar — always shown */}
      <ThreadToolbar
        label={label}
        title={isSearching ? "Search results" : title}
        subtitle={isSearching ? undefined : subtitle}
        threads={activeThreads}
        selectedIds={selection.selectedIds}
        allSelected={selection.allSelected}
        someSelected={selection.someSelected}
        hasSelection={selection.selectedIds.size > 0}
        selectAllPages={selectAllPages}
        onToggleSelectAll={selection.toggleSelectAll}
        onSelectIds={selection.selectIds}
        onToggleSelectAllPages={handleToggleSelectAllPages}
        onBulkAction={handleBulkAction}
        onRefresh={handleRefresh}
        onMarkAllRead={!isSearching && label !== "trash" && label !== "spam" && label !== "drafts" ? handleMarkAllRead : undefined}
        onEmptyFolder={!isSearching && (label === "trash" || label === "spam") ? handleEmptyFolder : undefined}
        onSnooze={!isSearching && label !== "drafts" ? handleSnooze : undefined}
        page={page}
        total={isSearching ? searchTotal : total}
        limit={isSearching ? searchLimit : LIMIT}
        onPageChange={handlePageChange}
        loading={isSearching ? searchFetching : (refreshing || syncRunning)}
        isPending={bulkMutation.isPending}
      />

      {isSearching ? (
        /* Search results */
        <div ref={listScrollRef} className="flex-1 overflow-y-auto overflow-x-hidden relative">
          {searchFetching ? (
            <div className="flex items-center justify-center h-32">
              <Spinner className="h-6 w-6" />
            </div>
          ) : searchError ? (
            <div className="flex flex-col items-center justify-center h-32 gap-2 text-sm">
              <span className="text-destructive">Search failed</span>
              <button onClick={() => searchRefetch()} className="text-primary hover:underline text-xs">
                Try again
              </button>
            </div>
          ) : searchResults.length === 0 ? (
            <div className="flex flex-col items-center justify-center h-32 gap-2 text-sm">
              <span className="text-muted-foreground">No results found for &ldquo;{searchQuery}&rdquo;</span>
              <button
                onClick={() => { setSearchInput(""); updateParams({ q: null, scope: null, page: null }); }}
                className="text-primary hover:underline text-xs"
              >
                Clear search
              </button>
            </div>
          ) : (
            <ThreadList
              threads={searchResults}
              domainId={domainId}
              label={label}
              selectedId={selectedThreadId ?? undefined}
              selectedIds={selection.selectedIds}
              focusedIndex={focusedIndex}
              onToggleSelect={selection.toggleSelect}
              onSelectClick={selection.selectClick}
              onToggleSelectAll={selection.toggleSelectAll}
              onStar={handleStar}
              onAction={handleAction}
              onThreadClick={handleThreadClick}
              resolveLabel={getThreadLabel}
            />
          )}
        </div>
      ) : (
        <>
          {/* Thread list or empty state */}
          <div ref={listScrollRef} className={`flex-1 overflow-y-auto overflow-x-hidden relative ${refreshing ? "opacity-60" : ""}`}>
            {threads.length === 0 ? (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                {EMPTY_MESSAGES[label] ?? `No messages with label “${label}”`}
              </div>
            ) : (
              <ThreadList
                threads={threads}
                domainId={domainId}
                label={label}
                selectedId={selectedThreadId ?? undefined}
                selectedIds={selection.selectedIds}
                focusedIndex={focusedIndex}
                onToggleSelect={selection.toggleSelect}
              onSelectClick={selection.selectClick}
                onToggleSelectAll={selection.toggleSelectAll}
                onStar={handleStar}
                onAction={handleAction}
                onThreadClick={handleThreadClick}
              />
            )}
          </div>
        </>
      )}
    </div>
  );

  return (
    <ThreadListProvider
      value={{
        threads: activeThreads,
        selectedIds: selection.selectedIds,
        toggleSelect: selection.toggleSelect,
        handleBulkAction,
        handleRefresh,
        focusedIndex,
        setFocusedIndex,
        handleStar,
        handleAction,
        label,
        domainId,
        onThreadClick: handleThreadClick,
      }}
    >
      <div className="h-full flex">
        {/* List pane — hidden on mobile when a thread is selected */}
        <div className={`${selectedThreadId ? "hidden md:block print:hidden w-full md:w-[350px] md:shrink-0 md:border-r" : "flex-1"} h-full`}>
          {listPane}
        </div>

        {/* Detail pane — full-width on mobile */}
        {selectedThreadId && (
          <div className="flex-1 h-full overflow-hidden">
            <ThreadView
              key={selectedThreadId}
              threadId={selectedThreadId}
              domainId={domainId}
              label={label}
              onBack={() => updateParams({ thread: null })}
            />
          </div>
        )}
      </div>
    </ThreadListProvider>
  );
}
