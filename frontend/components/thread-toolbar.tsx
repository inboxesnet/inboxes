"use client";

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
  Minus,
} from "lucide-react";
import type { Folder } from "@/lib/types";

interface ThreadToolbarProps {
  folder: Folder;
  allSelected: boolean;
  someSelected: boolean;
  hasSelection: boolean;
  onToggleSelectAll: () => void;
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
  allSelected,
  someSelected,
  hasSelection,
  onToggleSelectAll,
  onBulkAction,
  onRefresh,
  page,
  total,
  limit,
  onPageChange,
  loading,
}: ThreadToolbarProps) {
  const start = total === 0 ? 0 : (page - 1) * limit + 1;
  const end = Math.min(page * limit, total);
  const totalPages = Math.ceil(total / limit);

  return (
    <div className="flex items-center gap-2 h-10 px-3 border-b shrink-0">
      {/* Select all checkbox */}
      <div className="relative flex items-center">
        <input
          type="checkbox"
          checked={allSelected}
          ref={(el) => {
            if (el) el.indeterminate = someSelected;
          }}
          onChange={onToggleSelectAll}
          className="h-3.5 w-3.5 rounded border-muted-foreground/40 cursor-pointer accent-primary"
        />
      </div>

      {/* Bulk actions — visible when selection exists */}
      {hasSelection && (
        <div className="flex items-center gap-0.5 ml-1">
          {(folder === "inbox" || folder === "sent" || folder === "spam") && (
            <button
              title="Archive"
              onClick={() => onBulkAction("archive")}
              className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
            >
              <Archive className="h-4 w-4" />
            </button>
          )}
          {folder !== "trash" && (
            <button
              title="Trash"
              onClick={() => onBulkAction("trash")}
              className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
            >
              <Trash2 className="h-4 w-4" />
            </button>
          )}
          {folder === "trash" && (
            <>
              <button
                title="Move to Inbox"
                onClick={() => onBulkAction("move:inbox")}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
              >
                <Inbox className="h-4 w-4" />
              </button>
              <button
                title="Delete permanently"
                onClick={() => onBulkAction("delete")}
                className="p-1.5 rounded hover:bg-muted text-destructive hover:text-destructive"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            </>
          )}
          {folder === "spam" && (
            <button
              title="Not spam"
              onClick={() => onBulkAction("move:inbox")}
              className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
            >
              <Inbox className="h-4 w-4" />
            </button>
          )}
          <button
            title="Mark as read"
            onClick={() => onBulkAction("read")}
            className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
          >
            <MailOpen className="h-4 w-4" />
          </button>
          <button
            title="Mark as unread"
            onClick={() => onBulkAction("unread")}
            className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
          >
            <Mail className="h-4 w-4" />
          </button>
          {folder !== "spam" && folder !== "trash" && (
            <button
              title="Spam"
              onClick={() => onBulkAction("spam")}
              className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
            >
              <AlertTriangle className="h-4 w-4" />
            </button>
          )}
        </div>
      )}

      {/* Spacer */}
      <div className="flex-1" />

      {/* Refresh */}
      <button
        title="Refresh"
        onClick={onRefresh}
        className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
      >
        <RefreshCw className={cn("h-4 w-4", loading && "animate-spin")} />
      </button>

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
