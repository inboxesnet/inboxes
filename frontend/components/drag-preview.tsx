"use client";

import type { Thread } from "@/lib/types";

interface DragPreviewProps {
  thread: Thread;
  count?: number;
}

function extractSender(emails: string[]): string {
  if (!emails || emails.length === 0) return "Unknown";
  const first = Array.isArray(emails) ? emails[0] : String(emails);
  const atIndex = first.indexOf("@");
  return atIndex > 0 ? first.substring(0, atIndex) : first;
}

export function DragPreview({ thread, count = 1 }: DragPreviewProps) {
  const sender = extractSender(thread.participant_emails);

  return (
    <div className="bg-background border rounded-md shadow-lg px-3 py-2 max-w-[300px] pointer-events-none">
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
