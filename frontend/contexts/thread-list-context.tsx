"use client";

import { createContext, useContext } from "react";
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

export function ThreadListProvider({
  children,
  value,
}: {
  children: React.ReactNode;
  value: ThreadListContextType;
}) {
  return (
    <ThreadListContext.Provider value={value}>
      {children}
    </ThreadListContext.Provider>
  );
}

export function useThreadList() {
  return useContext(ThreadListContext);
}
