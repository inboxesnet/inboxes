"use client";

import { useState, useRef, useEffect } from "react";
import { cn, getInitials, getDomainColor, getDomainTextColor } from "@/lib/utils";
import { useDomains } from "@/contexts/domain-context";
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
  Tag,
  BellOff,
} from "lucide-react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { hasLabel } from "@/lib/types";
import type { Label, Thread } from "@/lib/types";

interface ThreadToolbarProps {
  label: Label;
  title?: string;
  subtitle?: string;
  threads: Thread[];
  selectedIds: Set<string>;
  allSelected: boolean;
  someSelected: boolean;
  hasSelection: boolean;
  selectAllPages: boolean;
  onToggleSelectAll: () => void;
  onSelectIds: (ids: string[]) => void;
  onToggleSelectAllPages: () => void;
  onBulkAction: (action: string) => void;
  onRefresh: () => void;
  page: number;
  total: number;
  limit: number;
  onPageChange: (page: number) => void;
  loading?: boolean;
  isPending?: boolean;
}

interface CustomLabel {
  id: string;
  name: string;
}

export function ThreadToolbar({
  label,
  title,
  subtitle,
  threads,
  selectedIds,
  allSelected,
  someSelected,
  hasSelection,
  selectAllPages,
  onToggleSelectAll,
  onSelectIds,
  onToggleSelectAllPages,
  onBulkAction,
  onRefresh,
  page,
  total,
  limit,
  onPageChange,
  loading,
  isPending,
}: ThreadToolbarProps) {
  const { activeDomain } = useDomains();
  const [dropdownOpen, setDropdownOpen] = useState(false);
  const [labelDropdownOpen, setLabelDropdownOpen] = useState(false);
  const [customLabels, setCustomLabels] = useState<CustomLabel[]>([]);
  const dropdownRef = useRef<HTMLDivElement>(null);
  const labelDropdownRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!dropdownOpen && !labelDropdownOpen) return;
    function handleClick(e: MouseEvent) {
      if (dropdownOpen && dropdownRef.current && !dropdownRef.current.contains(e.target as Node)) {
        setDropdownOpen(false);
      }
      if (labelDropdownOpen && labelDropdownRef.current && !labelDropdownRef.current.contains(e.target as Node)) {
        setLabelDropdownOpen(false);
      }
    }
    document.addEventListener("mousedown", handleClick);
    return () => document.removeEventListener("mousedown", handleClick);
  }, [dropdownOpen, labelDropdownOpen]);

  useEffect(() => {
    if (labelDropdownOpen && customLabels.length === 0) {
      api.get<CustomLabel[]>("/api/labels").then(setCustomLabels).catch(() => { toast.error("Failed to load labels"); });
    }
  }, [labelDropdownOpen]);
  const start = total === 0 ? 0 : (page - 1) * limit + 1;
  const end = Math.min(page * limit, total);
  const totalPages = Math.ceil(total / limit);

  const showSelectAllBanner = allSelected && total > threads.length && !selectAllPages;
  const showSelectedAllBanner = selectAllPages;

  return (
    <div>
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
              { label: "Starred", action: () => onSelectIds(threads.filter((t) => hasLabel(t, "starred")).map((t) => t.id)) },
              { label: "Unstarred", action: () => onSelectIds(threads.filter((t) => !hasLabel(t, "starred")).map((t) => t.id)) },
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
        const showArchive = label === "inbox" || label === "sent";
        const showMoveToInbox = label === "archive" || label === "trash" || label === "spam";
        const showSpam = label !== "sent" && label !== "spam";
        const showTrash = label !== "trash";
        const showDelete = label === "trash";

        return (
          <div className="flex items-center gap-0.5 ml-1">
            {/* Primary: Archive or Move to Inbox */}
            {showArchive && (
              <button
                title="Archive"
                onClick={() => onBulkAction("archive")}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
              >
                <Archive className="h-4 w-4" />
              </button>
            )}
            {showMoveToInbox && (
              <button
                title="Move to Inbox"
                onClick={() => onBulkAction("move:inbox")}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
              >
                <Inbox className="h-4 w-4" />
              </button>
            )}

            {/* Read/Unread — contextual */}
            {hasUnread && (
              <button
                title="Mark as read"
                onClick={() => onBulkAction("read")}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
              >
                <MailOpen className="h-4 w-4" />
              </button>
            )}
            {hasRead && (
              <button
                title="Mark as unread"
                onClick={() => onBulkAction("unread")}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
              >
                <Mail className="h-4 w-4" />
              </button>
            )}

            {/* Report Spam */}
            {showSpam && (
              <button
                title="Report spam"
                onClick={() => onBulkAction("spam")}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
              >
                <AlertTriangle className="h-4 w-4" />
              </button>
            )}

            {/* Mute/Unmute */}
            <button
              title={selected.every((t) => hasLabel(t, "muted")) ? "Unmute" : "Mute"}
              onClick={() => {
                const allMuted = selected.every((t) => hasLabel(t, "muted"));
                onBulkAction(allMuted ? "unmute" : "mute");
              }}
              disabled={isPending}
              className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
            >
              <BellOff className="h-4 w-4" />
            </button>

            {/* Label dropdown */}
            <div className="relative" ref={labelDropdownRef}>
              <button
                title="Label"
                onClick={() => setLabelDropdownOpen((v) => !v)}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
              >
                <Tag className="h-4 w-4" />
              </button>
              {labelDropdownOpen && (
                <div className="absolute top-full left-0 mt-1 z-50 bg-popover border rounded-md shadow-md py-1 min-w-[160px]">
                  {customLabels.length === 0 ? (
                    <div className="px-3 py-2 text-sm text-muted-foreground">No labels</div>
                  ) : (
                    customLabels.map((l) => {
                      const allHaveLabel = selected.every((t) => hasLabel(t, l.name));
                      return (
                        <button
                          key={l.id}
                          onClick={() => {
                            onBulkAction(allHaveLabel ? `unlabel:${l.name}` : `label:${l.name}`);
                            setLabelDropdownOpen(false);
                          }}
                          className="w-full text-left px-3 py-1.5 text-sm hover:bg-accent hover:text-accent-foreground flex items-center gap-2"
                        >
                          <span className={cn("h-3 w-3 rounded border flex items-center justify-center text-[10px]", allHaveLabel ? "bg-primary border-primary text-primary-foreground" : "border-muted-foreground/40")}>
                            {allHaveLabel && "✓"}
                          </span>
                          {l.name}
                        </button>
                      );
                    })
                  )}
                </div>
              )}
            </div>

            {/* Destructive: Trash or Delete permanently */}
            {showTrash && (
              <button
                title="Trash"
                onClick={() => onBulkAction("trash")}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground disabled:opacity-50 disabled:pointer-events-none"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
            {showDelete && (
              <button
                title="Delete permanently"
                onClick={() => { if (confirm("Permanently delete selected conversations?")) onBulkAction("delete"); }}
                disabled={isPending}
                className="p-1.5 rounded hover:bg-muted text-destructive hover:text-destructive disabled:opacity-50 disabled:pointer-events-none"
              >
                <Trash2 className="h-4 w-4" />
              </button>
            )}
          </div>
        );
      })()}

      {/* Spacer */}
      <div className="flex-1" />

      {/* Label title — mobile only, centered */}
      {title && !hasSelection && (
        <span className="flex items-center gap-1 text-sm md:hidden">
          {activeDomain && (
            <span className={cn("font-normal opacity-60", getDomainTextColor(activeDomain.domain))}>{activeDomain.domain}</span>
          )}
          {activeDomain && <span className="text-muted-foreground/50">/</span>}
          <span className="font-semibold">
            {title}
            {subtitle && (
              <span className="block text-xs font-normal text-muted-foreground">{subtitle}</span>
            )}
          </span>
        </span>
      )}

      {/* Spacer */}
      <div className="flex-1" />

      {/* Pagination */}
      {total > 0 && (
        <div className="flex items-center gap-1 text-xs text-muted-foreground">
          <span>
            {start}–{end} of {total}
            {totalPages > 1 && <span className="hidden md:inline ml-1">(Page {page}/{totalPages})</span>}
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
    {(showSelectAllBanner || showSelectedAllBanner) && (
      <div className="flex items-center justify-center gap-1 h-8 px-3 border-b bg-muted/30 text-xs text-muted-foreground">
        {showSelectAllBanner && (
          <>
            All {threads.length} conversations on this page are selected.
            <button onClick={onToggleSelectAllPages} className="text-primary hover:underline font-medium">
              Select all {total} conversations
            </button>
          </>
        )}
        {showSelectedAllBanner && (
          <>
            All {total} conversations are selected.
            <button onClick={onToggleSelectAllPages} className="text-primary hover:underline font-medium">
              Clear selection
            </button>
          </>
        )}
      </div>
    )}
    </div>
  );
}
