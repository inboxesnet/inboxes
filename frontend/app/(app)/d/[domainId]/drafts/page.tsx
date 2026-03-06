"use client";

import { useState, useEffect, useCallback } from "react";
import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { useEmailWindow } from "@/contexts/email-window-context";
import { Spinner } from "@/components/ui/spinner";
import { FileText, Trash2 } from "lucide-react";
import { formatRelativeTime } from "@/lib/utils";
import { cn } from "@/lib/utils";
import type { Draft } from "@/lib/types";

export default function DraftsPage() {
  const params = useParams();
  const domainId = params.domainId as string;
  const { openDraft, currentDraftId, closeCompose } = useEmailWindow();
  const qc = useQueryClient();
  const [focusedIndex, setFocusedIndex] = useState(-1);

  const { data, isLoading } = useQuery({
    queryKey: ["drafts", domainId],
    queryFn: () =>
      api.get<{ drafts: Draft[] }>(`/api/drafts?domain_id=${domainId}`),
  });

  const drafts = data?.drafts ?? [];

  // Reset focus when drafts change
  useEffect(() => {
    setFocusedIndex(-1);
  }, [domainId]);

  const handleDelete = useCallback(
    async (draftId: string, e?: React.MouseEvent) => {
      if (e) e.stopPropagation();
      if (currentDraftId === draftId) {
        closeCompose();
      }
      // Optimistically remove from list immediately
      qc.setQueryData<{ drafts: Draft[] }>(["drafts", domainId], (old) =>
        old ? { drafts: old.drafts.filter((d) => d.id !== draftId) } : old
      );
      try {
        await api.delete(`/api/drafts/${draftId}`);
      } catch {
        // 404 = already deleted (e.g. discarded from compose window) — ignore
      }
    },
    [currentDraftId, closeCompose, qc, domainId]
  );

  // Keyboard navigation for drafts
  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      const target = e.target as HTMLElement;
      if (target.tagName === "INPUT" || target.tagName === "TEXTAREA" || target.isContentEditable) return;
      if (e.metaKey || e.ctrlKey) return;

      switch (e.key) {
        case "j":
          e.preventDefault();
          setFocusedIndex((prev) => Math.min(prev + 1, drafts.length - 1));
          break;
        case "k":
          e.preventDefault();
          setFocusedIndex((prev) => Math.max(prev - 1, 0));
          break;
        case "Enter":
          e.preventDefault();
          if (focusedIndex >= 0 && focusedIndex < drafts.length) {
            openDraft(drafts[focusedIndex]);
          }
          break;
        case "#":
          e.preventDefault();
          if (focusedIndex >= 0 && focusedIndex < drafts.length) {
            handleDelete(drafts[focusedIndex].id);
          }
          break;
      }
    }
    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [drafts, focusedIndex, openDraft, handleDelete]);

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-full">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  return (
    <div className="h-full flex flex-col">
      <div className="h-14 flex items-center px-4 border-b shrink-0">
        <h1 className="font-semibold text-sm">Drafts</h1>
      </div>
      <div className="flex-1 overflow-y-auto">
        {drafts.length === 0 ? (
          <div className="flex items-center justify-center h-full text-muted-foreground text-sm">
            No drafts
          </div>
        ) : (
          <div className="divide-y">
            {drafts.map((draft, i) => (
              <div
                key={draft.id}
                role="button"
                tabIndex={0}
                onClick={() => openDraft(draft)}
                onKeyDown={(e) => { if (e.key === 'Enter' || e.key === ' ') openDraft(draft); }}
                className={cn(
                  "flex items-center gap-3 w-full text-left px-4 py-3 hover:bg-muted/50 transition-colors cursor-pointer",
                  i === focusedIndex && "ring-1 ring-inset ring-primary/30"
                )}
              >
                <FileText className="h-4 w-4 text-muted-foreground shrink-0" />
                <div className="flex-1 min-w-0">
                  <div className="flex items-center gap-2">
                    <span className="text-sm font-medium truncate">
                      {draft.subject || "(no subject)"}
                    </span>
                  </div>
                  <p className="text-xs text-muted-foreground truncate">
                    To: {(draft.to_addresses || []).join(", ") || "(no recipients)"}
                  </p>
                </div>
                <span className="text-xs text-muted-foreground shrink-0">
                  {formatRelativeTime(draft.updated_at)}
                </span>
                <button
                  onClick={(e) => handleDelete(draft.id, e)}
                  className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-destructive shrink-0"
                  title="Delete draft"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
