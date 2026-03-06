"use client";

import type { Thread } from "@/lib/types";
import { extractSender, extractDisplayName } from "@/lib/thread-helpers";

interface DragPreviewProps {
  thread: Thread;
  count?: number;
}

export function DragPreview({ thread, count = 1 }: DragPreviewProps) {
  const sender = thread.last_sender ? extractDisplayName(thread.last_sender) : extractSender(thread.participant_emails);

  return (
    <div className="bg-background border rounded-md shadow-lg px-3 py-2 w-[200px] pointer-events-none opacity-80">
      <p className="text-sm font-medium truncate">{thread.subject}</p>
      <p className="text-xs text-muted-foreground truncate">{sender}</p>
      {count > 1 && (
        <p className="text-xs text-muted-foreground mt-0.5">
          +{count - 1} more
        </p>
      )}
    </div>
  );
}
