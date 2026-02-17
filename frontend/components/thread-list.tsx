"use client";

import { useRouter } from "next/navigation";
import { cn, formatThreadTime } from "@/lib/utils";
import { Star, Archive, Trash2, Mail, MailOpen } from "lucide-react";
import { useDraggable } from "@dnd-kit/core";
import type { Thread, Folder } from "@/lib/types";

interface ThreadListProps {
  threads: Thread[];
  domainId: string;
  folder: Folder;
  selectedId?: string;
  selectedIds: Set<string>;
  focusedIndex: number;
  onToggleSelect: (id: string) => void;
  onToggleSelectAll: () => void;
  onStar: (id: string) => void;
  onAction: (id: string, action: string) => void;
  onThreadClick?: (threadId: string) => void;
}

function extractSender(emails: string[]): string {
  if (!emails || emails.length === 0) return "Unknown";
  const first = emails[0];
  const atIndex = first.indexOf("@");
  return atIndex > 0 ? first.substring(0, atIndex) : first;
}

function parseParticipants(raw: string[] | string): string[] {
  if (Array.isArray(raw)) return raw;
  if (typeof raw === "string") {
    try {
      return JSON.parse(raw);
    } catch {
      return [];
    }
  }
  return [];
}

function ThreadRow({
  thread,
  index,
  domainId,
  folder,
  selectedId,
  selectedIds,
  focusedIndex,
  onToggleSelect,
  onStar,
  onAction,
  onThreadClick,
}: {
  thread: Thread;
  index: number;
  domainId: string;
  folder: Folder;
  selectedId?: string;
  selectedIds: Set<string>;
  focusedIndex: number;
  onToggleSelect: (id: string) => void;
  onStar: (id: string) => void;
  onAction: (id: string, action: string) => void;
  onThreadClick?: (threadId: string) => void;
}) {
  const router = useRouter();
  const { attributes, listeners, setNodeRef, isDragging } = useDraggable({
    id: thread.id,
    data: { thread },
  });

  const isSelected = selectedIds.has(thread.id);
  const isFocused = index === focusedIndex;
  const isActive = thread.id === selectedId;
  const isUnread = thread.unread_count > 0;
  const participants = parseParticipants(thread.participant_emails);
  const displayName =
    folder === "sent" && thread.original_to
      ? extractSender([thread.original_to])
      : extractSender(participants);

  return (
    <div
      ref={setNodeRef}
      {...listeners}
      {...attributes}
      role="button"
      tabIndex={0}
      onClick={() => onThreadClick ? onThreadClick(thread.id) : router.push(`/d/${domainId}/${folder}/${thread.id}`)}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          onThreadClick ? onThreadClick(thread.id) : router.push(`/d/${domainId}/${folder}/${thread.id}`);
        }
      }}
      className={cn(
        "group flex items-center gap-2 w-full text-left px-3 h-10 transition-colors cursor-pointer select-none",
        isDragging && "opacity-50",
        isActive && "bg-accent",
        isSelected && !isActive && "bg-accent/60",
        isUnread && !isSelected && !isActive && "bg-blue-50/50",
        isFocused && !isSelected && !isActive && "ring-1 ring-inset ring-primary/30",
        !isActive && !isSelected && !isUnread && "hover:bg-muted/50"
      )}
    >
      {/* Checkbox */}
      <input
        type="checkbox"
        checked={isSelected}
        onChange={(e) => {
          e.stopPropagation();
          onToggleSelect(thread.id);
        }}
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
        className="h-3.5 w-3.5 rounded border-muted-foreground/40 shrink-0 cursor-pointer accent-primary"
      />

      {/* Star */}
      <button
        onClick={(e) => {
          e.stopPropagation();
          onStar(thread.id);
        }}
        onPointerDown={(e) => e.stopPropagation()}
        className="shrink-0 p-0.5 -m-0.5"
      >
        <Star
          className={cn(
            "h-3.5 w-3.5",
            thread.starred
              ? "text-yellow-500 fill-yellow-500"
              : "text-muted-foreground/30 hover:text-yellow-500"
          )}
        />
      </button>

      {/* Sender / Recipient */}
      <span
        className={cn(
          "w-[160px] md:w-[200px] shrink-0 text-sm truncate",
          isUnread ? "font-semibold" : "font-normal"
        )}
      >
        {folder === "sent" && <span className="text-muted-foreground font-normal">To </span>}
        {displayName}
        {thread.message_count > 1 && (
          <span className="text-xs text-muted-foreground ml-1">
            ({thread.message_count})
          </span>
        )}
      </span>

      {/* Subject — snippet */}
      <span className="flex-1 min-w-0 text-sm truncate">
        <span className={isUnread ? "font-medium" : ""}>
          {thread.subject}
        </span>
        {thread.snippet && (
          <span className="text-muted-foreground hidden md:inline">
            {" "}
            — {thread.snippet}
          </span>
        )}
      </span>

      {/* Time (default) / Hover actions */}
      <div className="shrink-0 w-[100px] flex items-center justify-end">
        {/* Time - visible by default, hidden on hover */}
        <span className="text-xs text-muted-foreground group-hover:hidden whitespace-nowrap">
          {formatThreadTime(thread.last_message_at)}
        </span>

        {/* Actions - hidden by default, visible on hover */}
        <div className="hidden group-hover:flex items-center gap-1">
          {folder !== "archive" && (
            <button
              title="Archive"
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
          {folder !== "trash" && (
            <button
              title="Trash"
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
  folder,
  selectedId,
  selectedIds,
  focusedIndex,
  onToggleSelect,
  onToggleSelectAll,
  onStar,
  onAction,
  onThreadClick,
}: ThreadListProps) {
  if (threads.length === 0) {
    return null;
  }

  return (
    <div className="divide-y">
      {threads.map((thread, index) => (
        <ThreadRow
          key={thread.id}
          thread={thread}
          index={index}
          domainId={domainId}
          folder={folder}
          selectedId={selectedId}
          selectedIds={selectedIds}
          focusedIndex={focusedIndex}
          onToggleSelect={onToggleSelect}
          onStar={onStar}
          onAction={onAction}
          onThreadClick={onThreadClick}
        />
      ))}
    </div>
  );
}
