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
import type { Folder, Thread } from "@/lib/types";

const LIMIT = 100;

const EMPTY_MESSAGES: Record<Folder, string> = {
  inbox: "Your inbox is empty",
  sent: "No sent messages",
  drafts: "No drafts",
  archive: "No archived messages",
  trash: "Trash is empty",
  spam: "No spam messages",
  deleted_forever: "No deleted messages",
};

interface ThreadListPageProps {
  folder: Folder;
  title: string;
  subtitle?: string;
}

export function ThreadListPage({ folder, title, subtitle }: ThreadListPageProps) {
  const params = useParams();
  const domainId = params.domainId as string;
  const qc = useQueryClient();

  const [page, setPage] = useState(1);
  const [focusedIndex, setFocusedIndex] = useState(-1);
  const [searchInput, setSearchInput] = useState("");
  const [searchQuery, setSearchQuery] = useState("");
  const searchInputRef = useRef<HTMLInputElement>(null);
  const [selectedThreadId, setSelectedThreadId] = useState<string | null>(null);

  const { data, isLoading, isFetching } = useThreadList(domainId, folder, page);
  const threads = data?.threads ?? [];
  const total = data?.total ?? 0;

  const threadIds = useMemo(() => threads.map((t) => t.id), [threads]);
  const selection = useThreadSelection(threadIds);
  const clearSelectionRef = useRef(selection.clearSelection);
  clearSelectionRef.current = selection.clearSelection;

  const { data: searchData, isFetching: searchFetching } = useQuery({
    queryKey: ["search", domainId, searchQuery],
    queryFn: () =>
      api.get<{ threads: Thread[] }>(
        `/api/emails/search?q=${encodeURIComponent(searchQuery)}&domain_id=${domainId}`
      ),
    enabled: searchQuery.length > 0,
  });
  const searchResults = searchData?.threads ?? [];

  const starMutation = useStarThread();
  const actionMutation = useThreadAction();
  const bulkMutation = useBulkAction();

  // Reset selection, focus, and reading pane when domain/folder changes
  useEffect(() => {
    setPage(1);
    clearSelectionRef.current();
    setFocusedIndex(-1);
    setSelectedThreadId(null);
  }, [domainId, folder]);

  const handleRefresh = useCallback(() => {
    qc.invalidateQueries({ queryKey: queryKeys.threads.lists() });
    qc.invalidateQueries({ queryKey: queryKeys.domains.unreadCounts() });
  }, [qc]);

  const handlePageChange = useCallback(
    (newPage: number) => {
      selection.clearSelection();
      setFocusedIndex(-1);
      setPage(newPage);
    },
    [selection.clearSelection]
  );

  const handleStar = useCallback(
    (threadId: string) => {
      starMutation.mutate(threadId);
    },
    [starMutation]
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
      setSelectedThreadId(threadId);
    },
    []
  );

  const handleBulkAction = useCallback(
    (actionStr: string) => {
      const ids = Array.from(selection.selectedIds);
      if (ids.length === 0) return;

      let action = actionStr;
      let moveFolder: string | undefined;

      if (actionStr.startsWith("move:")) {
        action = "move";
        moveFolder = actionStr.split(":")[1];
      }

      selection.clearSelection();
      bulkMutation.mutate({ threadIds: ids, action, folder: moveFolder });
    },
    [selection.selectedIds, selection.clearSelection, bulkMutation]
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
      <div className="h-14 flex items-center px-4 border-b shrink-0 gap-2">
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
              placeholder="Search emails..."
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

      {searchQuery ? (
        /* Search results */
        <div className="flex-1 overflow-y-auto overflow-x-hidden relative">
          {searchFetching ? (
            <div className="flex items-center justify-center h-32">
              <Spinner className="h-6 w-6" />
            </div>
          ) : searchResults.length === 0 ? (
            <div className="flex items-center justify-center h-32 text-muted-foreground text-sm">
              No results found
            </div>
          ) : (
            <ThreadList
              threads={searchResults}
              domainId={domainId}
              folder={folder}
              selectedId={selectedThreadId ?? undefined}
              selectedIds={selection.selectedIds}
              focusedIndex={-1}
              onToggleSelect={selection.toggleSelect}
              onToggleSelectAll={selection.toggleSelectAll}
              onStar={handleStar}
              onAction={handleAction}
              onThreadClick={handleThreadClick}
            />
          )}
        </div>
      ) : (
        <>
          {/* Toolbar */}
          <ThreadToolbar
            folder={folder}
            threads={threads}
            selectedIds={selection.selectedIds}
            allSelected={selection.allSelected}
            someSelected={selection.someSelected}
            hasSelection={selection.selectedIds.size > 0}
            onToggleSelectAll={selection.toggleSelectAll}
            onSelectIds={selection.selectIds}
            onBulkAction={handleBulkAction}
            onRefresh={handleRefresh}
            page={page}
            total={total}
            limit={LIMIT}
            onPageChange={handlePageChange}
            loading={refreshing}
          />

          {/* Thread list or empty state */}
          <div className={`flex-1 overflow-y-auto overflow-x-hidden relative ${refreshing ? "opacity-60" : ""}`}>
            {threads.length === 0 ? (
              <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
                {EMPTY_MESSAGES[folder]}
              </div>
            ) : (
              <ThreadList
                threads={threads}
                domainId={domainId}
                folder={folder}
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
        threads,
        selectedIds: selection.selectedIds,
        toggleSelect: selection.toggleSelect,
        handleBulkAction,
        handleRefresh,
        focusedIndex,
        setFocusedIndex,
        handleStar,
        handleAction,
        folder,
        domainId,
      }}
    >
      <div className="h-full flex">
        {/* List pane */}
        <div className={`${selectedThreadId ? "w-[400px] shrink-0 border-r" : "flex-1"} h-full`}>
          {listPane}
        </div>

        {/* Detail pane */}
        {selectedThreadId && (
          <div className="flex-1 h-full overflow-hidden">
            <ThreadView
              key={selectedThreadId}
              threadId={selectedThreadId}
              domainId={domainId}
              folder={folder}
              onBack={() => setSelectedThreadId(null)}
            />
          </div>
        )}
      </div>
    </ThreadListProvider>
  );
}
