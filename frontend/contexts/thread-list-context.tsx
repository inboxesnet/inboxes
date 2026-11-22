"use client";

import { createContext, useContext, useEffect, useSyncExternalStore } from "react";
import type { Thread, Label } from "@/lib/types";

interface ThreadListContextType {
  threads: Thread[];
  selectedIds: Set<string>;
  toggleSelect: (id: string) => void;
  handleBulkAction: (action: string) => void;
  handleRefresh: () => void;
  focusedIndex: number;
  setFocusedIndex: (index: number) => void;
  handleStar: (id: string) => void;
  handleAction: (id: string, action: string) => void;
  label: Label;
  domainId: string;
  onThreadClick?: (id: string) => void;
}

const ThreadListContext = createContext<ThreadListContextType | null>(null);

// The keyboard shortcuts live in the layout, outside the provider that the
// page renders. The provider publishes its value here so they can read it.
let published: ThreadListContextType | null = null;
const listeners = new Set<() => void>();

function publish(value: ThreadListContextType | null) {
  published = value;
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

const getPublished = () => published;
const getServerPublished = () => null;

export function ThreadListProvider({
  children,
  value,
}: {
  children: React.ReactNode;
  value: ThreadListContextType;
}) {
  useEffect(() => {
    publish(value);
  }, [value]);
  useEffect(() => () => publish(null), []);

  return (
    <ThreadListContext.Provider value={value}>
      {children}
    </ThreadListContext.Provider>
  );
}

export function useThreadList() {
  const fromContext = useContext(ThreadListContext);
  const fromStore = useSyncExternalStore(subscribe, getPublished, getServerPublished);
  return fromContext ?? fromStore;
}
