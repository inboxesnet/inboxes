"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams, useSearchParams, useRouter } from "next/navigation";
import {
  DndContext,
  DragOverlay,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DragStartEvent,
  type DragEndEvent,
} from "@dnd-kit/core";
import { useDomains } from "@/contexts/domain-context";
import { EmailWindowProvider, useEmailWindow } from "@/contexts/email-window-context";
import { toast } from "sonner";
import { api } from "@/lib/api";
import dynamic from "next/dynamic";
import { DomainSidebar } from "@/components/domain-sidebar";
import { FloatingComposeWindow } from "@/components/floating-compose-window";
import type { Tab as SettingsTab } from "@/components/settings-modal";
import { NotificationListener } from "@/components/notification-listener";
import { KeyboardShortcuts } from "@/components/keyboard-shortcuts";
import { DragPreview } from "@/components/drag-preview";
import { Spinner } from "@/components/ui/spinner";
import { useBroadcastSync } from "@/hooks/use-broadcast-sync";
import { useTheme } from "next-themes";
import { Menu, Settings, Keyboard, Sun, Moon, LogOut } from "lucide-react";
import type { Thread } from "@/lib/types";

const SettingsModal = dynamic(
  () => import("@/components/settings-modal").then((m) => m.SettingsModal),
  { ssr: false }
);

function DomainLayoutInner({ children }: { children: React.ReactNode }) {
  const params = useParams();
  const searchParams = useSearchParams();
  const router = useRouter();
  const domainId = params.domainId as string;
  const { setActiveDomainId, loading: domainsLoading } = useDomains();
  // Always show loading on first render (both SSR and client) to avoid hydration mismatch.
  // useEffect fires client-only after hydration, switching to the real loading state.
  const [mounted, setMounted] = useState(false);
  useEffect(() => { setMounted(true); }, []);
  const loading = !mounted || domainsLoading;
  const { openCompose } = useEmailWindow();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [settingsTab, setSettingsTab] = useState<SettingsTab | undefined>(undefined);
  const [sidebarOpen, setSidebarOpen] = useState(false);
  const [draggedThread, setDraggedThread] = useState<Thread | null>(null);
  const [billingSuccess, setBillingSuccess] = useState(false);
  const { theme, setTheme } = useTheme();
  useBroadcastSync();

  async function handleLogout() {
    try { await api.post("/api/auth/logout"); } catch { /* redirect regardless */ }
    window.location.href = "/login";
  }

  // Handle Stripe checkout success redirect
  useEffect(() => {
    if (searchParams.get("billing") === "success") {
      setBillingSuccess(true);
      router.replace(`/d/${domainId}/inbox`);
      const timer = setTimeout(() => setBillingSuccess(false), 5000);
      return () => clearTimeout(timer);
    }
  }, [searchParams, domainId, router]);

  const mouseSensor = useSensor(MouseSensor, {
    activationConstraint: { distance: 8 },
  });
  const touchSensor = useSensor(TouchSensor, {
    activationConstraint: { delay: 250, tolerance: 5 },
  });
  const sensors = useSensors(mouseSensor, touchSensor);

  useEffect(() => {
    setActiveDomainId(domainId);
  }, [domainId, setActiveDomainId]);

  function handleCompose() {
    openCompose();
  }

  const handleDragStart = useCallback((event: DragStartEvent) => {
    const thread = event.active.data.current?.thread as Thread | undefined;
    if (thread) setDraggedThread(thread);
  }, []);

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      setDraggedThread(null);
      const { active, over } = event;
      if (!over) return;

      const targetLabel = over.id as string;
      const threadIds = (active.data.current?.threadIds as string[]) || [];
      const thread = active.data.current?.thread as Thread | undefined;

      if (threadIds.length > 1) {
        // Multi-thread drag — bulk move
        try {
          await api.patch("/api/threads/bulk", {
            thread_ids: threadIds,
            action: "move",
            label: targetLabel,
          });
        } catch {
          toast.error("Failed to move thread(s)");
        }
      } else if (thread) {
        try {
          await api.patch(`/api/threads/${thread.id}/move`, {
            label: targetLabel,
          });
        } catch {
          toast.error("Failed to move thread(s)");
        }
      }
    },
    []
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center h-dvh">
        <Spinner className="h-8 w-8" />
      </div>
    );
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      accessibility={{ screenReaderInstructions: { draggable: "Drag to move between folders, or press V to open the move dialog" } }}
    >
      <div className="flex h-dvh">
        {/* Skip to main content — accessibility */}
        <a
          href="#main-content"
          className="sr-only focus:not-sr-only focus:absolute focus:z-50 focus:top-2 focus:left-2 focus:bg-background focus:text-foreground focus:px-4 focus:py-2 focus:rounded-md focus:border focus:shadow-md"
        >
          Skip to main content
        </a>
        {/* Mobile sidebar overlay */}
        {sidebarOpen && (
          <div className="fixed inset-0 z-40 md:hidden">
            <div
              className="absolute inset-0 bg-black/50"
              onClick={() => setSidebarOpen(false)}
            />
            <div className="relative z-10 h-full shadow-xl">
              <DomainSidebar
                onCompose={handleCompose}
                onOpenSettings={(tab?: string) => { setSettingsTab(tab as SettingsTab | undefined); setSettingsOpen(true); }}
                onCloseSidebar={() => setSidebarOpen(false)}
              />
            </div>
          </div>
        )}

        {/* Desktop sidebar */}
        <div className="hidden md:flex">
          <DomainSidebar onCompose={handleCompose} onOpenSettings={(tab?: string) => { setSettingsTab(tab as SettingsTab | undefined); setSettingsOpen(true); }} />
        </div>

        <main id="main-content" className="flex-1 overflow-hidden relative" tabIndex={-1}>
          {/* Mobile hamburger */}
          <button
            onClick={() => setSidebarOpen(true)}
            className="absolute top-3.5 left-3 z-30 p-1.5 rounded-md hover:bg-muted md:hidden"
            aria-label="Open sidebar"
          >
            <Menu className="h-5 w-5" />
          </button>

          {/* Top-right toolbar (desktop only) */}
          <div className="hidden md:flex absolute top-0 right-0 z-20 h-14 items-center gap-1 pr-3">
            <button
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
              className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              title={theme === "dark" ? "Light mode" : "Dark mode"}
            >
              {theme === "dark" ? <Sun className="h-4 w-4" /> : <Moon className="h-4 w-4" />}
            </button>
            <button
              onClick={() => window.dispatchEvent(new CustomEvent("open-shortcuts-dialog"))}
              className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              title="Keyboard shortcuts (?)"
            >
              <Keyboard className="h-4 w-4" />
            </button>
            <button
              onClick={() => { setSettingsTab(undefined); setSettingsOpen(true); }}
              className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
              title="Settings"
            >
              <Settings className="h-4 w-4" />
            </button>
            <button
              onClick={handleLogout}
              className="p-2 rounded-md text-muted-foreground hover:text-destructive hover:bg-muted transition-colors"
              title="Log out"
            >
              <LogOut className="h-4 w-4" />
            </button>
          </div>
          {billingSuccess && (
            <div className="bg-green-500/10 text-green-700 dark:text-green-400 text-sm text-center py-2 px-4 border-b border-green-500/20">
              Subscription activated! You now have full access.
            </div>
          )}
          {children}
        </main>
        <FloatingComposeWindow />
        <SettingsModal open={settingsOpen} onOpenChange={(v) => { setSettingsOpen(v); if (!v) setSettingsTab(undefined); }} defaultTab={settingsTab} />
        <NotificationListener />
        <KeyboardShortcuts onCompose={handleCompose} />
      </div>
      <DragOverlay dropAnimation={null}>
        {draggedThread && <DragPreview thread={draggedThread} />}
      </DragOverlay>
    </DndContext>
  );
}

export default function DomainLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <EmailWindowProvider>
      <DomainLayoutInner>{children}</DomainLayoutInner>
    </EmailWindowProvider>
  );
}
