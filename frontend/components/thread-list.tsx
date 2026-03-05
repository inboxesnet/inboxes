"use client";

import { useRouter } from "next/navigation";
import { cn, formatThreadTime } from "@/lib/utils";
import { Star, Archive, Trash2, Mail, MailOpen, BellOff } from "lucide-react";
import { useDraggable } from "@dnd-kit/core";
import { hasLabel } from "@/lib/types";
import type { Thread, Label } from "@/lib/types";
import { extractSender, extractDisplayName, cleanSnippet, parseParticipants } from "@/lib/thread-helpers";

interface ThreadListProps {
  threads: Thread[];
  domainId: string;
  label: Label;
  selectedId?: string;
  selectedIds: Set<string>;
  focusedIndex: number;
  onToggleSelect: (id: string) => void;
  onToggleSelectAll: () => void;
  onStar: (id: string) => void;
  onAction: (id: string, action: string) => void;
  onThreadClick?: (threadId: string) => void;
  resolveLabel?: (thread: Thread) => Label;
}

function ThreadRow({
  thread,
  index,
  domainId,
  label,
  selectedId,
  selectedIds,
  focusedIndex,
  onToggleSelect,
  onStar,
  onAction,
  onThreadClick,
  resolveLabel,
}: {
  thread: Thread;
  index: number;
  domainId: string;
  label: Label;
  selectedId?: string;
  selectedIds: Set<string>;
  focusedIndex: number;
  onToggleSelect: (id: string) => void;
  onStar: (id: string) => void;
  onAction: (id: string, action: string) => void;
  onThreadClick?: (threadId: string) => void;
  resolveLabel?: (thread: Thread) => Label;
}) {
  const router = useRouter();
  const effectiveLabel = resolveLabel ? resolveLabel(thread) : label;
  // Include all selected thread IDs in drag data for multi-thread drag
  const dragThreadIds = selectedIds.has(thread.id) && selectedIds.size > 1
    ? Array.from(selectedIds)
    : [thread.id];

  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: thread.id,
    data: { thread, threadIds: dragThreadIds },
  });

  const isSelected = selectedIds.has(thread.id);
  const isFocused = index === focusedIndex;
  const isActive = thread.id === selectedId;
  const isUnread = thread.unread_count > 0;
  const participants = parseParticipants(thread.participant_emails);
  const displayName =
    effectiveLabel === "sent" && thread.original_to
      ? extractSender([thread.original_to])
      : thread.last_sender
        ? extractDisplayName(thread.last_sender)
        : extractSender(participants);

  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      role="listitem"
      tabIndex={0}
      onClick={() => onThreadClick ? onThreadClick(thread.id) : router.push(`/d/${domainId}/${effectiveLabel}/${thread.id}`)}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          onThreadClick ? onThreadClick(thread.id) : router.push(`/d/${domainId}/${label}/${thread.id}`);
        }
      }}
      className={cn(
        "group grid grid-cols-[14px_16px_1fr_auto] md:grid-cols-[14px_16px_200px_1fr_100px] items-center gap-1.5 w-full text-left px-3 h-14 md:h-10 transition-colors cursor-pointer select-none overflow-hidden",
        isDragging && "opacity-50",
        isActive && "bg-accent",
        isSelected && !isActive && "bg-accent/60",
        isUnread && !isSelected && !isActive && "bg-blue-50/50 dark:bg-accent/60",
        isFocused && !isSelected && !isActive && "ring-1 ring-inset ring-primary/30",
        !isActive && !isSelected && !isUnread && "hover:bg-muted/50 text-muted-foreground dark:text-muted-foreground"
      )}
    >
      {/* Checkbox — expanded click radius via label pseudo-element */}
      <label
        className="relative flex items-center justify-center cursor-pointer before:absolute before:inset-[-8px] before:content-['']"
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        <input
          type="checkbox"
          checked={isSelected}
          onChange={() => onToggleSelect(thread.id)}
          className="h-3.5 w-3.5 rounded border-muted-foreground/40 cursor-pointer accent-primary"
        />
      </label>

      {/* Star */}
      <button
        onClick={(e) => {
          e.stopPropagation();
          onStar(thread.id);
        }}
        onPointerDown={(e) => e.stopPropagation()}
        className="p-0.5 -m-0.5"
        aria-label={hasLabel(thread, "starred") ? "Unstar" : "Star"}
      >
        <Star
          className={cn(
            "h-3.5 w-3.5",
            hasLabel(thread, "starred")
              ? "text-yellow-500 fill-yellow-500"
              : "text-muted-foreground/30 hover:text-yellow-500"
          )}
        />
      </button>

      {/* Mobile: two-line layout */}
      <div className="min-w-0 md:hidden flex flex-col justify-center gap-0.5">
        <div className="flex items-baseline min-w-0">
          <span className={cn("text-sm truncate", isUnread ? "font-semibold text-foreground" : "font-normal text-foreground dark:text-muted-foreground")}>
            {label === "sent" && <span className="text-muted-foreground font-normal">To </span>}
            {displayName}
          </span>
          {thread.message_count > 1 && (
            <span className="text-xs text-muted-foreground ml-1 shrink-0">
              ({thread.message_count})
            </span>
          )}
        </div>
        <div className="text-xs truncate">
          {hasLabel(thread, "muted") && <BellOff className="inline h-3 w-3 text-muted-foreground/50 mr-1" />}
          <span className={isUnread ? "font-medium text-foreground" : "text-muted-foreground dark:text-muted-foreground/80"}>{thread.subject}</span>
          {thread.snippet && (
            <span className="text-muted-foreground/80">
              {" — "}{cleanSnippet(thread.snippet)}
            </span>
          )}
        </div>
      </div>

      {/* Desktop: Sender / Recipient */}
      <span
        className={cn(
          "hidden md:inline text-sm truncate",
          isUnread ? "font-semibold text-foreground" : "font-normal dark:text-muted-foreground"
        )}
      >
        {label === "sent" && <span className="text-muted-foreground font-normal">To </span>}
        {displayName}
        {thread.message_count > 1 && (
          <span className="text-xs text-muted-foreground ml-1">
            ({thread.message_count})
          </span>
        )}
      </span>

      {/* Desktop: Subject — snippet */}
      <div className="hidden md:block min-w-0 text-sm truncate">
        {hasLabel(thread, "muted") && <BellOff className="inline h-3 w-3 text-muted-foreground/50 mr-1" />}
        <span className={isUnread ? "font-semibold text-foreground" : "dark:text-muted-foreground"}>{thread.subject}</span>
        {thread.snippet && (
          <span className="text-muted-foreground/70">
            {" — "}{cleanSnippet(thread.snippet)}
          </span>
        )}
      </div>

      {/* Time / Hover actions */}
      <div className="flex items-center justify-end">
        {/* Time — always visible on mobile, hidden on desktop hover */}
        <span className="text-xs text-muted-foreground md:group-hover:hidden whitespace-nowrap">
          {resolveLabel && (
            <span className="text-[10px] text-muted-foreground/80 mr-1.5 capitalize">
              {effectiveLabel}
            </span>
          )}
          {formatThreadTime(thread.last_message_at)}
        </span>

        {/* Actions — desktop hover only */}
        <div className="hidden md:group-hover:flex items-center gap-1">
          {effectiveLabel !== "archive" && (
            <button
              title="Archive"
              aria-label="Archive"
              onClick={(e) => {
                e.stopPropagation();
                onAction(thread.id, "archive");
              }}
              onPointerDown={(e) => e.stopPropagation()}
              className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
            >
              <Archive className="h-3.5 w-3.5" />
            </button>
          )}
          {effectiveLabel !== "trash" && (
            <button
              title="Trash"
              aria-label="Trash"
              onClick={(e) => {
                e.stopPropagation();
                onAction(thread.id, "trash");
              }}
              onPointerDown={(e) => e.stopPropagation()}
              className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          )}
          <button
            title={isUnread ? "Mark read" : "Mark unread"}
            aria-label={isUnread ? "Mark as read" : "Mark as unread"}
            onClick={(e) => {
              e.stopPropagation();
              onAction(thread.id, isUnread ? "read" : "unread");
            }}
            onPointerDown={(e) => e.stopPropagation()}
            className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-foreground"
          >
            {isUnread ? (
              <MailOpen className="h-3.5 w-3.5" />
            ) : (
              <Mail className="h-3.5 w-3.5" />
            )}
          </button>
        </div>
      </div>
    </div>
  );
}

export function ThreadList({
  threads,
  domainId,
  label,
  selectedId,
  selectedIds,
  focusedIndex,
  onToggleSelect,
  onToggleSelectAll,
  onStar,
  onAction,
  onThreadClick,
  resolveLabel,
}: ThreadListProps) {
  if (threads.length === 0) {
    return null;
  }

  return (
    <div className="divide-y" role="list" aria-label="Email threads">
      {threads.map((thread, index) => (
        <ThreadRow
          key={thread.id}
          thread={thread}
          index={index}
          domainId={domainId}
          label={label}
          selectedId={selectedId}
          selectedIds={selectedIds}
          focusedIndex={focusedIndex}
          onToggleSelect={onToggleSelect}
          onStar={onStar}
          onAction={onAction}
          onThreadClick={onThreadClick}
          resolveLabel={resolveLabel}
        />
      ))}
    </div>
  );
}
