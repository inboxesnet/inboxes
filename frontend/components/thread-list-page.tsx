"use client";

import { useState, useCallback, useMemo, useEffect, useRef } from "react";
import { useParams } from "next/navigation";
import { useQueryClient, useQuery } from "@tanstack/react-query";
import { useThreadList, useStarThread, useThreadAction, useBulkAction } from "@/hooks/use-threads";
import { useThreadSelection } from "@/hooks/use-thread-selection";
import { ThreadList } from "@/components/thread-list";
import { ThreadToolbar } from "@/components/thread-toolbar";
import { ThreadView } from "@/components/thread-view";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import { ThreadListProvider } from "@/contexts/thread-list-context";
import { queryKeys } from "@/lib/query-keys";
import { api } from "@/lib/api";
import { Search, X } from "lucide-react";
import type { Label, Thread } from "@/lib/types";

const LIMIT = 100;

const EMPTY_MESSAGES: Record<Label, string> = {
  inbox: "Your inbox is empty",
  sent: "No sent messages",
  drafts: "No drafts",
  archive: "No archived messages",
  starred: "No starred messages",
  trash: "Trash is empty",
  spam: "No spam messages",
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

export function ThreadListPage({ label, title, subtitle }: ThreadListPageProps) {
  const params = useParams();
  const domainId = params.domainId as string;
  const qc = useQueryClient();

  const [page, setPage] = useState(1);
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const [searchInput, setSearchInput] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const searchInputRef = useRef<HTMLInputElement>(null);
  const listScrollRef = useRef<HTMLDivElement>(null);
  const scrollPositionRef = useRef(0);
  const [selectedThreadId, setSelectedThreadId] = useState<string | null>(null);
  const [selectAllPages, setSelectAllPages] = useState(false);

  const { data, isLoading, isFetching } = useThreadList(domainId, label, page);
  const threads = data?.threads ?? [];
  const total = data?.total ?? 0;

  const { data: searchData, isFetching: searchFetching, isError: searchError, refetch: searchRefetch } = useQuery({
    queryKey: queryKeys.search.results(domainId, searchQuery),
    queryFn: () =>
      api.get<{ threads: Thread[] }>(
        `/api/emails/search?q=${encodeURIComponent(searchQuery)}&domain_id=${domainId}`
      ),
    enabled: searchQuery.length > 0,
  });
  const searchResults = searchData?.threads ?? [];

  const isSearching = searchQuery.length > 0;
  const activeThreads = isSearching ? searchResults : threads;

  const threadIds = useMemo(() => activeThreads.map((t) => t.id), [activeThreads]);
  const selection = useThreadSelection(threadIds);
  const clearSelectionRef = useRef(selection.clearSelection);
  clearSelectionRef.current = selection.clearSelection;

  const starMutation = useStarThread();
  const actionMutation = useThreadAction();
  const bulkMutation = useBulkAction();

  // Update page title
  useEffect(() => {
    document.title = `${title} - Inboxes`;
  }, [title]);

  // Reset selection, focus, and reading pane when domain/label changes
  useEffect(() => {
    setPage(1);
    clearSelectionRef.current();
    setFocusedIndex(-1);
    setSelectedThreadId(null);
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

  const handleRefresh = useCallback(() => {
    qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
    qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
    if (isSearching) {
      qc.invalidateQueries({ queryKey: queryKeys.search.all });
    }
  }, [qc, isSearching]);

  const handlePageChange = useCallback(
    (newPage: number) => {
      selection.clearSelection();
      setFocusedIndex(-1);
      setSelectAllPages(false);
      setPage(newPage);
    },
    [selection.clearSelection]
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
        setSelectedThreadId(null);
      }
    },
    [actionMutation, selectedThreadId]
  );

  const handleThreadClick = useCallback(
    (threadId: string) => {
      // Save scroll position before opening thread
      if (listScrollRef.current) {
        scrollPositionRef.current = listScrollRef.current.scrollTop;
      }
      setSelectedThreadId(threadId);
    },
    []
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

      // Clear selection only on success — preserve on error so user can retry
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
            selection.clearSelection();
            setSelectAllPages(false);
          },
        }
      );
    },
    [selection.selectedIds, selection.clearSelection, bulkMutation, selectAllPages, label, domainId]
  );

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  const refreshing = isFetching && !isLoading;

  const listPane = (
    <div className="h-full flex flex-col">
      {/* Header */}
      <div className="h-14 flex items-center pl-14 md:pl-4 pr-4 border-b shrink-0 gap-2">
        <form
          className="flex-1 flex items-center gap-2 max-w-md"
          onSubmit={(e) => {
            e.preventDefault();
            setSearchQuery(searchInput.trim());
          }}
        >
          <div className="relative flex-1">
            <Search className="absolute left-2.5 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
            <Input
              ref={searchInputRef}
              value={searchInput}
              onChange={(e) => setSearchInput(e.target.value)}
              placeholder={`Search ${title.toLowerCase()} by subject, sender, or content...`}
              className="h-8 pl-8 bg-muted"
            />
          </div>
          {searchQuery && (
            <button
              type="button"
              onClick={() => {
                setSearchInput("");
                setSearchQuery("");
              }}
              className="text-muted-foreground hover:text-foreground"
            >
              <X className="h-4 w-4" />
            </button>
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
        page={isSearching ? 1 : page}
        total={isSearching ? searchResults.length : total}
        limit={isSearching ? searchResults.length : LIMIT}
        onPageChange={handlePageChange}
        loading={isSearching ? searchFetching : refreshing}
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
                onClick={() => { setSearchInput(""); setSearchQuery(""); }}
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
                {EMPTY_MESSAGES[label]}
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
        <div className={`${selectedThreadId ? "hidden md:block w-full md:w-[350px] md:shrink-0 md:border-r" : "flex-1"} h-full`}>
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
              onBack={() => setSelectedThreadId(null)}
            />
          </div>
        )}
      </div>
    </ThreadListProvider>
  );
}
