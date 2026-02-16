"use client";

import { usePathname, useRouter } from "next/navigation";
import { useDroppable } from "@dnd-kit/core";
import { useDomains } from "@/contexts/domain-context";
import { DomainIcon } from "@/components/domain-icon";
import { cn } from "@/lib/utils";
import {
  Inbox,
  Send,
  FileText,
  Archive,
  Trash2,
  AlertTriangle,
  PenSquare,
  Settings,
  Plus,
} from "lucide-react";
import type { Folder } from "@/lib/types";

const FOLDERS: { key: Folder; label: string; icon: React.ReactNode }[] = [
  { key: "inbox", label: "Inbox", icon: <Inbox className="h-4 w-4" /> },
  { key: "sent", label: "Sent", icon: <Send className="h-4 w-4" /> },
  { key: "drafts", label: "Drafts", icon: <FileText className="h-4 w-4" /> },
  { key: "archive", label: "Archive", icon: <Archive className="h-4 w-4" /> },
  { key: "spam", label: "Spam", icon: <AlertTriangle className="h-4 w-4" /> },
  { key: "trash", label: "Trash", icon: <Trash2 className="h-4 w-4" /> },
];

interface DomainSidebarProps {
  onCompose: () => void;
  onOpenSettings: () => void;
}

function DroppableFolderButton({
  folderKey,
  label,
  icon,
  isActive,
  count,
  onClick,
}: {
  folderKey: string;
  label: string;
  icon: React.ReactNode;
  isActive: boolean;
  count: number;
  onClick: () => void;
}) {
  const { isOver, setNodeRef } = useDroppable({ id: folderKey });

  return (
    <button
      ref={setNodeRef}
      onClick={onClick}
      className={cn(
        "flex items-center gap-3 w-full rounded-md px-3 py-2 text-sm transition-colors",
        isActive
          ? "bg-accent/80 text-accent-foreground font-medium"
          : "text-muted-foreground hover:bg-accent/50 hover:text-foreground",
        isOver && "bg-primary/10 ring-2 ring-primary/30"
      )}
    >
      {icon}
      <span className="flex-1 text-left">{label}</span>
      {count > 0 && (
        <span className="text-xs bg-primary text-primary-foreground rounded-full px-1.5 py-0.5 min-w-[20px] text-center">
          {count}
        </span>
      )}
    </button>
  );
}

export function DomainSidebar({ onCompose, onOpenSettings }: DomainSidebarProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { domains, activeDomain, setActiveDomainId, unreadCounts } =
    useDomains();

  // Extract folder from path: /d/{domainId}/{folder}/... → folder is segments[3]
  const segments = pathname.split("/");
  const folderSegment = segments[3] || "inbox";
  const currentFolder = FOLDERS.some((f) => f.key === folderSegment) ? folderSegment : "inbox";

  function navigateToDomain(domainId: string) {
    setActiveDomainId(domainId);
    router.push(`/d/${domainId}/inbox`);
  }

  function navigateToFolder(folder: string) {
    if (!activeDomain) return;
    router.push(`/d/${activeDomain.id}/${folder}`);
  }

  return (
    <div className="flex h-screen">
      {/* Left strip: domain icons */}
      <div className="flex flex-col items-center w-[72px] bg-muted/50 py-3 gap-2 border-r overflow-y-auto">
        {domains.map((d) => (
          <DomainIcon
            key={d.id}
            domain={d.domain}
            active={activeDomain?.id === d.id}
            hasUnread={(unreadCounts[d.id] || 0) > 0}
            onClick={() => navigateToDomain(d.id)}
          />
        ))}

        {/* Separator */}
        <div className="w-8 h-px bg-border my-1" />

        {/* Add domain */}
        <button
          onClick={onOpenSettings}
          className="flex items-center justify-center h-12 w-12 rounded-[24px] hover:rounded-2xl bg-muted text-muted-foreground hover:bg-green-500 hover:text-white transition-all duration-200"
          title="Add domain"
        >
          <Plus className="h-5 w-5" />
        </button>

        {/* Spacer */}
        <div className="flex-1" />

        {/* Settings */}
        <button
          onClick={onOpenSettings}
          className="flex items-center justify-center h-12 w-12 rounded-[24px] hover:rounded-2xl bg-muted text-muted-foreground hover:bg-accent transition-all duration-200"
          title="Settings"
        >
          <Settings className="h-5 w-5" />
        </button>
      </div>

      {/* Right panel: folder navigation */}
      <div className="flex flex-col w-[240px] bg-background border-r">
        {/* Domain name header */}
        <div className="h-14 flex items-center px-4 border-b">
          <h2 className="font-semibold text-sm truncate">
            {activeDomain?.domain || "Select a domain"}
          </h2>
        </div>

        {/* Compose button */}
        <div className="p-3">
          <button
            onClick={onCompose}
            className="flex items-center gap-2 w-full rounded-md bg-primary text-primary-foreground px-3 py-2 text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            <PenSquare className="h-4 w-4" />
            Compose
          </button>
        </div>

        {/* Folder list */}
        <nav className="flex-1 px-2 space-y-0.5">
          {FOLDERS.map((f) => {
            const isActive = currentFolder === f.key;
            const count =
              f.key === "inbox" && activeDomain
                ? unreadCounts[activeDomain.id] || 0
                : 0;

            return (
              <DroppableFolderButton
                key={f.key}
                folderKey={f.key}
                label={f.label}
                icon={f.icon}
                isActive={isActive}
                count={count}
                onClick={() => navigateToFolder(f.key)}
              />
            );
          })}
        </nav>
      </div>
    </div>
  );
}
