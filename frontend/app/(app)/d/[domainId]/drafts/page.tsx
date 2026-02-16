"use client";

import { useParams } from "next/navigation";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { useEmailWindow } from "@/contexts/email-window-context";
import { Spinner } from "@/components/ui/spinner";
import { FileText, Trash2 } from "lucide-react";
import { formatRelativeTime } from "@/lib/utils";
import type { Draft } from "@/lib/types";

export default function DraftsPage() {
  const params = useParams();
  const domainId = params.domainId as string;
  const { openDraft } = useEmailWindow();
  const qc = useQueryClient();

  const { data, isLoading } = useQuery({
    queryKey: ["drafts", domainId],
    queryFn: () =>
      api.get<{ drafts: Draft[] }>(`/api/drafts?domain_id=${domainId}`),
  });

  const drafts = data?.drafts ?? [];

  async function handleDelete(e: React.MouseEvent, draftId: string) {
    e.stopPropagation();
    try {
      await api.delete(`/api/drafts/${draftId}`);
      qc.invalidateQueries({ queryKey: ["drafts", domainId] });
    } catch {
      // silent
    }
  }

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
            {drafts.map((draft) => (
              <button
                key={draft.id}
                onClick={() => openDraft(draft)}
                className="flex items-center gap-3 w-full text-left px-4 py-3 hover:bg-muted/50 transition-colors"
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
                  onClick={(e) => handleDelete(e, draft.id)}
                  className="p-1 rounded hover:bg-muted text-muted-foreground hover:text-destructive shrink-0"
                  title="Delete draft"
                >
                  <Trash2 className="h-3.5 w-3.5" />
                </button>
              </button>
            ))}
          </div>
        )}
      </div>
    </div>
  );
}
