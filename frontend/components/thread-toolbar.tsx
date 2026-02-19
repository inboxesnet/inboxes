"use client";

import { useState, useRef, useEffect } from "react";
import { cn } from "@/lib/utils";
import {
  Archive,
  Trash2,
  Mail,
  MailOpen,
  AlertTriangle,
  Inbox,
  RefreshCw,
  ChevronLeft,
  ChevronRight,
  ChevronDown,
  Minus,
} from "lucide-react";
import type { Folder, Thread } from "@/lib/types";

interface ThreadToolbarProps {
  folder: Folder;
  threads: Thread[];
  selectedIds: Set<string>;
  allSelected: boolean;
  someSelected: boolean;
  hasSelection: boolean;
  onToggleSelectAll: () => void;
  onSelectIds: (ids: string[]) => void;
  onBulkAction: (action: string) => void;
  onRefresh: () => void;
  page: number;
  total: number;
  limit: number;
  onPageChange: (page: number) => void;
  loading?: boolean;
}

export function ThreadToolbar({
  folder,
  threads,
  selectedIds,
  allSelected,
  someSelected,
  hasSelection,
  onToggleSelectAll,
  onSelectIds,
  onBulkAction,
  onRefresh,
  page,
  total,
  limit,
  onPageChange,
  loading,
}: ThreadToolbarProps) {
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const dropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!dropdownOpen) return;
    function handleClick(e: MouseEvent) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDropdownOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [dropdownOpen]);
  const start = total === 0 ? 0 : (page - 1) * limit + 1;
  const end = Math.min(page * limit, total);
  const totalPages = Math.ceil(total / limit);

  return (
    <div className="flex items-center gap-2 h-10 px-3 border-b shrink-0">
      {/* Select all checkbox + dropdown */}
      <div className="relative flex items-center" ref={dropdownRef}>
        <input
          type="checkbox"
          checked={allSelected}
          ref={(el) => {
            if (el) el.indeterminate = someSelected;
          }}
          onChange={onToggleSelectAll}
          className="h-3.5 w-3.5 rounded border-muted-foreground/40 cursor-pointer accent-primary"
        />
        <button
          onClick={() => setDropdownOpen((v) => !v)}
          className="ml-0.5 p-0.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
        >
          <ChevronDown className="h-3 w-3" />
        </button>
        {dropdownOpen && (
          <div className="absolute top-full left-0 mt-1 z-50 bg-popover border rounded-md shadow-md py-1 min-w-[140px]">
            {[
              { label: "All", action: () => onSelectIds(threads.map((t) => t.id)) },
              { label: "None", action: () => onSelectIds([]) },
              { label: "Read", action: () => onSelectIds(threads.filter((t) => t.unread_count === 0).map((t) => t.id)) },
              { label: "Unread", action: () => onSelectIds(threads.filter((t) => t.unread_count > 0).map((t) => t.id)) },
              { label: "Starred", action: () => onSelectIds(threads.filter((t) => t.starred).map((t) => t.id)) },
              { label: "Unstarred", action: () => onSelectIds(threads.filter((t) => !t.starred).map((t) => t.id)) },
            ].map((item) => (
              <button
                key={item.label}
                onClick={() => { item.action(); setDropdownOpen(false); }}
                className="w-full text-left px-3 py-1.5 text-sm hover:bg-accent hover:text-accent-foreground"
              >
                {item.label}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Refresh — hidden when selection active */}
      {!hasSelection && (
        <button
          title="Refresh"
          onClick={onRefresh}
          className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
        >
          <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
        </button>
      )}

      {/* Bulk actions — visible when selection exists */}
      {hasSelection && (() => {
        const selected = threads.filter((t) => selectedIds.has(t.id));
        const hasUnread = selected.some((t) => t.unread_count > 0);
        const hasRead = selected.some((t) => t.unread_count === 0);
        const showArchive = folder === "inbox" || folder === "sent";
        const showMoveToInbox = folder === "archive" || folder === "trash" || folder === "spam";
        const showSpam = folder !== "sent" && folder !== "spam";
        const showTrash = folder !== "trash";
        const showDelete = folder === "trash";

        return (
          <div className="flex items-center gap-0.5 ml-1">
            {/* Primary: Archive or Move to Inbox */}
            {showArchive && (
              <button
                title="Archive"
                onClick={() => onBulkAction("archive")}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
              >
                <Archive className="h-4 w-4" />
              </button>
            )}
            {showMoveToInbox && (
              <button
                title="Move to Inbox"
                onClick={() => onBulkAction("move:inbox")}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
              >
                <Inbox className="h-4 w-4" />
              </button>
            )}

            {/* Read/Unread — contextual */}
            {hasUnread && (
              <button
                title="Mark as read"
                onClick={() => onBulkAction("read")}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
              >
                <MailOpen className="h-4 w-4" />
              </button>
            )}
            {hasRead && (
              <button
                title="Mark as unread"
                onClick={() => onBulkAction("unread")}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
              >
                <Mail className="h-4 w-4" />
              </button>
            )}

            {/* Report Spam */}
            {showSpam && (
              <button
                title="Report spam"
                onClick={() => onBulkAction("spam")}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
              >
                <AlertTriangle className="h-4 w-4" />
              </button>
            )}

            {/* Destructive: Trash or Delete permanently */}
            {showTrash && (
              <button
                title="Trash"
                onClick={() => onBulkAction("trash")}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            {showDelete && (
              <button
                title="Delete permanently"
                onClick={() => onBulkAction("delete")}
                className="p-1.5 rounded hover:bg-muted text-destructive hover:text-destructive"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
          </div>
        );
      })()}

      {/* Spacer */}
      <div className="flex-1" />

      {/* Pagination */}
      {total > 0 && (
        <div className="flex items-center gap-1 text-xs text-muted-foreground">
          <span>
            {start}–{end} of {total}
          </span>
          <button
            onClick={() => onPageChange(page - 1)}
            disabled={page <= 1}
            className="p-1 rounded hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronLeft className="h-4 w-4" />
          </button>
          <button
            onClick={() => onPageChange(page + 1)}
            disabled={page >= totalPages}
            className="p-1 rounded hover:bg-muted disabled:opacity-30 disabled:cursor-not-allowed"
          >
            <ChevronRight className="h-4 w-4" />
          </button>
        </div>
      )}
    </div>
  );
}
