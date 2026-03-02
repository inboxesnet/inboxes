"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { toast } from "sonner";
import { usePathname, useRouter } from "next/navigation";
import { DndContext, closestCenter, PointerSensor, useSensor, useSensors } from "@dnd-kit/core";
import type { DragEndEvent } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy, useSortable, arrayMove } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { useDroppable } from "@dnd-kit/core";
import { useTheme } from "next-themes";
import { useQueryClient } from "@tanstack/react-query";
import { useDomains } from "@/contexts/domain-context";
import { useNotifications } from "@/contexts/notification-context";
import { DomainIcon } from "@/components/domain-icon";
import { cn } from "@/lib/utils";
import { queryKeys } from "@/lib/query-keys";
import type { Domain } from "@/lib/types";
import {
  Inbox,
  Send,
  FileText,
  Archive,
  Star,
  Trash2,
  AlertTriangle,
  PenSquare,
  Settings,
  Plus,
  X,
  Sun,
  Moon,
  LogOut,
  Tag,
  WifiOff,
  Keyboard,
  Info,
} from "lucide-react";
import { api } from "@/lib/api";

interface CustomLabel {
  id: string;
  name: string;
}
import type { Label } from "@/lib/types";

const LABELS: { key: Label; label: string; icon: React.ReactNode }[] = [
  { key: "inbox", label: "Inbox", icon: <Inbox className="h-4 w-4" /> },
  { key: "sent", label: "Sent", icon: <Send className="h-4 w-4" /> },
  { key: "drafts", label: "Drafts", icon: <FileText className="h-4 w-4" /> },
  { key: "archive", label: "Archive", icon: <Archive className="h-4 w-4" /> },
  { key: "starred", label: "Starred", icon: <Star className="h-4 w-4" /> },
  { key: "spam", label: "Spam", icon: <AlertTriangle className="h-4 w-4" /> },
  { key: "trash", label: "Trash", icon: <Trash2 className="h-4 w-4" /> },
];

interface DomainSidebarProps {
  onCompose: () => void;
  onOpenSettings: (tab?: string) => void;
  onCloseSidebar?: () => void;
}

function DroppableLabelButton({
  labelKey,
  label,
  icon,
  isActive,
  count,
  onClick,
}: {
  labelKey: string;
  label: string;
  icon: React.ReactNode;
  isActive: boolean;
  count: number;
  onClick: () => void;
}) {
  const { isOver, setNodeRef } = useDroppable({ id: labelKey });

  return (
    <button
      ref={setNodeRef}
      onClick={onClick}
      className={cn(
        "flex items-center gap-2.5 w-full rounded-md px-3 py-1.5 text-[13px] transition-colors",
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

function OfflineBanner({ className }: { className?: string }) {
  const [showInfo, setShowInfo] = useState(false);
  const ref = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!showInfo) return;
    function onClickOutside(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setShowInfo(false);
    }
    document.addEventListener("mousedown", onClickOutside);
    return () => document.removeEventListener("mousedown", onClickOutside);
  }, [showInfo]);

  return (
    <div ref={ref} className={cn("relative mx-2 flex items-center gap-2 rounded-md bg-yellow-100 dark:bg-yellow-900/30 px-3 py-1.5 text-xs text-yellow-800 dark:text-yellow-300 shrink-0", className)}>
      <WifiOff className="h-3.5 w-3.5 shrink-0" />
      <span className="flex-1">Offline</span>
      <button
        onClick={() => setShowInfo((v) => !v)}
        className="h-4 w-4 rounded-full bg-yellow-300/60 dark:bg-yellow-700/60 flex items-center justify-center hover:bg-yellow-400/60 dark:hover:bg-yellow-600/60 transition-colors"
        aria-label="More info"
      >
        <Info className="h-3 w-3" />
      </button>
      {showInfo && (
        <div className="absolute bottom-full right-0 mb-1.5 w-56 rounded-md border bg-card p-2.5 text-xs text-card-foreground shadow-lg z-10">
          Connection to the server has been lost. If you&apos;re running locally, make sure the backend is still running.
        </div>
      )}
    </div>
  );
}

const restrictToYAxis: import("@dnd-kit/core").Modifier = ({ transform }) => ({
  ...transform,
  x: 0,
});

function SortableDomainIcon({
  id,
  domain,
  active,
  hasUnread,
  onClick,
}: {
  id: string;
  domain: string;
  active: boolean;
  hasUnread: boolean;
  onClick: () => void;
}) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useSortable({
    id,
    transition: null,
  });
  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    opacity: isDragging ? 0.5 : undefined,
    zIndex: isDragging ? 10 : undefined,
  };

  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners}>
      <DomainIcon
        domain={domain}
        active={active}
        hasUnread={hasUnread}
        onClick={onClick}
      />
    </div>
  );
}

export function DomainSidebar({ onCompose, onOpenSettings, onCloseSidebar }: DomainSidebarProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { theme, setTheme } = useTheme();
  const { domains, activeDomain, setActiveDomainId, unreadCounts } =
    useDomains();
  const { connected } = useNotifications();
  const qc = useQueryClient();

  const reorderSensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } })
  );

  const handleReorderDragEnd = useCallback(
    (event: DragEndEvent) => {
      const { active, over } = event;
      if (!over || active.id === over.id) return;

      const oldIndex = domains.findIndex((d) => d.id === active.id);
      const newIndex = domains.findIndex((d) => d.id === over.id);
      if (oldIndex === -1 || newIndex === -1) return;

      const reordered = arrayMove(domains, oldIndex, newIndex);

      // Optimistically update the cache
      qc.setQueryData<Domain[]>(queryKeys.domains.list(), reordered);

      // Fire API call in background
      const order = reordered.map((d, i) => ({ id: d.id, order: i }));
      api.patch("/api/domains/reorder", { order }).catch(() => {
        toast.error("Failed to save domain order");
        qc.invalidateQueries({ queryKey: queryKeys.domains.list() });
      });
    },
    [domains, qc]
  );

  // Show disconnected banner only after 3s of disconnection
  const [showDisconnected, setShowDisconnected] = useState(false);
  const disconnectTimerRef = useRef<NodeJS.Timeout | null>(null);
  useEffect(() => {
    if (!connected) {
      disconnectTimerRef.current = setTimeout(() => setShowDisconnected(true), 3000);
    } else {
      if (disconnectTimerRef.current) clearTimeout(disconnectTimerRef.current);
      setShowDisconnected(false);
    }
    return () => { if (disconnectTimerRef.current) clearTimeout(disconnectTimerRef.current); };
  }, [connected]);

  const [customLabels, setCustomLabels] = useState<CustomLabel[]>([]);

  useEffect(() => {
    api.get<CustomLabel[]>("/api/labels").then(setCustomLabels).catch(() => { toast.error("Failed to load labels"); });
  }, []);

  // Extract label from path: /d/{domainId}/{label}/... → label is segments[3]
  const segments = pathname.split("/");
  const labelSegment = segments[3] || "inbox";
  const currentLabel = LABELS.some((f) => f.key === labelSegment) ? labelSegment : labelSegment;

  function navigateToDomain(domainId: string) {
    setActiveDomainId(domainId);
    router.push(`/d/${domainId}/inbox`);
    onCloseSidebar?.();
  }

  function navigateToLabel(label: string) {
    if (!activeDomain) return;
    router.push(`/d/${activeDomain.id}/${label}`);
    onCloseSidebar?.();
  }

  async function handleLogout() {
    try {
      await api.post("/api/auth/logout");
    } catch {
      // Ignore errors — redirect regardless
    }
    window.location.href = "/login";
  }

  const labelList = LABELS.map((f) => {
    const isActive = currentLabel === f.key;
    const count =
      f.key === "inbox" && activeDomain
        ? unreadCounts[activeDomain.id] || 0
        : 0;

    return (
      <DroppableLabelButton
        key={f.key}
        labelKey={f.key}
        label={f.label}
        icon={f.icon}
        isActive={isActive}
        count={count}
        onClick={() => navigateToLabel(f.key)}
      />
    );
  });

  return (
    <>
      {/* ── Mobile layout ── */}
      <div className="flex flex-col h-full w-[85vw] max-w-[320px] bg-background md:hidden">
        {/* Header with close */}
        <div className="flex items-center justify-between px-4 h-14 border-b shrink-0">
          <h2 className="font-semibold text-sm truncate">
            {activeDomain?.domain || "Select a domain"}
          </h2>
          <button
            onClick={onCloseSidebar}
            className="p-1.5 -mr-1.5 rounded-md hover:bg-muted"
          >
            <X className="h-5 w-5" />
          </button>
        </div>

        {/* Domain icons — horizontal scroll */}
        <div className="flex items-center gap-2 px-4 py-3 border-b overflow-x-auto scrollbar-hide shrink-0">
          {domains.map((d) => (
            <DomainIcon
              key={d.id}
              domain={d.domain}
              active={activeDomain?.id === d.id}
              hasUnread={(unreadCounts[d.id] || 0) > 0}
              onClick={() => navigateToDomain(d.id)}
            />
          ))}
          <button
            onClick={() => onOpenSettings("domains")}
            className="flex items-center justify-center h-10 w-10 rounded-full bg-muted text-muted-foreground hover:bg-green-500 hover:text-white transition-colors shrink-0"
            title="Add domain"
          >
            <Plus className="h-4 w-4" />
          </button>
        </div>

        {/* Compose */}
        <div className="p-3 shrink-0">
          <button
            onClick={() => { onCompose(); onCloseSidebar?.(); }}
            className="flex items-center gap-2 w-full rounded-md bg-primary text-primary-foreground px-3 py-2.5 text-sm font-medium hover:bg-primary/90 transition-colors"
          >
            <PenSquare className="h-4 w-4" />
            Compose
          </button>
        </div>

        {/* Folders */}
        <nav className="flex-1 px-2 space-y-px overflow-y-auto scrollbar-hide">
          {labelList}
          {customLabels.length > 0 && (
            <>
              <div className="h-px bg-border my-2" />
              {customLabels.map((l) => (
                <DroppableLabelButton
                  key={`label-${l.name}`}
                  labelKey={l.name}
                  label={l.name}
                  icon={<Tag className="h-4 w-4" />}
                  isActive={currentLabel === l.name}
                  count={0}
                  onClick={() => navigateToLabel(l.name)}
                />
              ))}
            </>
          )}
        </nav>

        {/* Disconnected banner */}
        {showDisconnected && <OfflineBanner className="mb-1" />}

        {/* Theme toggle + Settings + Logout */}
        <div className="border-t p-2 shrink-0 space-y-px">
          <button
            onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
            className="flex items-center gap-3 w-full rounded-md px-3 py-2.5 text-sm text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-colors"
          >
            {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            {theme === "dark" ? "Light mode" : "Dark mode"}
          </button>
          <button
            onClick={() => { window.dispatchEvent(new CustomEvent("open-shortcuts-dialog")); onCloseSidebar?.(); }}
            className="flex items-center gap-3 w-full rounded-md px-3 py-2.5 text-sm text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-colors"
          >
            <Keyboard className="h-4 w-4" />
            Keyboard shortcuts
          </button>
          <button
            onClick={() => onOpenSettings()}
            className="flex items-center gap-3 w-full rounded-md px-3 py-2.5 text-sm text-muted-foreground hover:bg-accent/50 hover:text-foreground transition-colors"
          >
            <Settings className="h-4 w-4" />
            Settings
          </button>
          <button
            onClick={handleLogout}
            className="flex items-center gap-3 w-full rounded-md px-3 py-2.5 text-sm text-muted-foreground hover:bg-accent/50 hover:text-destructive transition-colors"
          >
            <LogOut className="h-4 w-4" />
            Log out
          </button>
        </div>
      </div>

      {/* ── Desktop layout (unchanged) ── */}
      <div className="hidden md:flex h-screen">
        {/* Left strip: domain icons */}
        <div className="flex flex-col items-center w-[72px] bg-muted/50 py-3 gap-2 border-r overflow-y-auto scrollbar-hide overflow-x-hidden">
          <DndContext
            sensors={reorderSensors}
            collisionDetection={closestCenter}
            modifiers={[restrictToYAxis]}
            onDragEnd={handleReorderDragEnd}
          >
            <SortableContext items={domains.map((d) => d.id)} strategy={verticalListSortingStrategy}>
              {domains.map((d) => (
                <SortableDomainIcon
                  key={d.id}
                  id={d.id}
                  domain={d.domain}
                  active={activeDomain?.id === d.id}
                  hasUnread={(unreadCounts[d.id] || 0) > 0}
                  onClick={() => navigateToDomain(d.id)}
                />
              ))}
            </SortableContext>
          </DndContext>

          {/* Separator */}
          <div className="w-8 h-px bg-border my-1" />

          {/* Add domain */}
          <button
            onClick={() => onOpenSettings("domains")}
            className="flex items-center justify-center h-12 w-12 rounded-[24px] hover:rounded-2xl bg-muted text-muted-foreground hover:bg-green-500 hover:text-white transition-all duration-200"
            title="Add domain"
          >
            <Plus className="h-5 w-5" />
          </button>
        </div>

        {/* Right panel: label navigation */}
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
              className="flex items-center gap-2 w-full rounded-md bg-primary text-primary-foreground px-3 py-1.5 text-sm font-medium hover:bg-primary/90 transition-colors"
            >
              <PenSquare className="h-4 w-4" />
              Compose
            </button>
          </div>

          {/* Label list */}
          <nav className="flex-1 px-2 space-y-px overflow-y-auto scrollbar-hide">
            {labelList}
            {customLabels.length > 0 && (
              <>
                <div className="h-px bg-border my-2" />
                {customLabels.map((l) => (
                  <DroppableLabelButton
                    key={`label-${l.name}`}
                    labelKey={l.name}
                    label={l.name}
                    icon={<Tag className="h-4 w-4" />}
                    isActive={currentLabel === l.name}
                    count={0}
                    onClick={() => navigateToLabel(l.name)}
                  />
                ))}
              </>
            )}
          </nav>
          {showDisconnected && <OfflineBanner className="mb-2" />}
        </div>
      </div>
    </>
  );
}
