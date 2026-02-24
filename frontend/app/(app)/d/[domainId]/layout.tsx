"use client";

import { useEffect, useState, useCallback } from "react";
import { useParams } from "next/navigation";
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
import { api } from "@/lib/api";
import { DomainSidebar } from "@/components/domain-sidebar";
import { FloatingComposeWindow } from "@/components/floating-compose-window";
import { SettingsModal } from "@/components/settings-modal";
import { NotificationListener } from "@/components/notification-listener";
import { KeyboardShortcuts } from "@/components/keyboard-shortcuts";
import { DragPreview } from "@/components/drag-preview";
import { Spinner } from "@/components/ui/spinner";
import type { Thread } from "@/lib/types";

function DomainLayoutInner({ children }: { children: React.ReactNode }) {
  const params = useParams();
  const domainId = params.domainId as string;
  const { setActiveDomainId, loading } = useDomains();
  const { openCompose } = useEmailWindow();
  const [settingsOpen, setSettingsOpen] = useState(false);
  const [draggedThread, setDraggedThread] = useState<Thread | null>(null);

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

      const targetFolder = over.id as string;
      const thread = active.data.current?.thread as Thread | undefined;
      if (!thread || thread.folder === targetFolder) return;

      try {
        await api.patch(`/api/threads/${thread.id}/move`, {
          folder: targetFolder,
        });
      } catch {
        // Move failed silently
      }
    },
    []
  );

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <Spinner className="h-8 w-8" />
      </div>
    );
  }

  return (
    <DndContext
      sensors={sensors}
      onDragStart={handleDragStart}
      onDragEnd={handleDragEnd}
      accessibility={{ screenReaderInstructions: { draggable: "" } }}
    >
      <div className="flex h-screen">
        <DomainSidebar onCompose={handleCompose} onOpenSettings={() => setSettingsOpen(true)} />
        <main className="flex-1 overflow-hidden">{children}</main>
        <FloatingComposeWindow />
        <SettingsModal open={settingsOpen} onOpenChange={setSettingsOpen} />
        <NotificationListener />
        <KeyboardShortcuts onCompose={handleCompose} />
      </div>
      <DragOverlay>
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
