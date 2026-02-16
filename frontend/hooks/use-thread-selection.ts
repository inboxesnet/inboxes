"use client";

import { useState, useCallback, useMemo } from "react";

export function useThreadSelection(threadIds: string[]) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());

  const toggleSelect = useCallback((id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev);
      if (next.has(id)) {
        next.delete(id);
      } else {
        next.add(id);
      }
      return next;
    });
  }, []);

  const toggleSelectAll = useCallback(() => {
    setSelectedIds((prev) => {
      if (prev.size === threadIds.length && threadIds.length > 0) {
        return new Set();
      }
      return new Set(threadIds);
    });
  }, [threadIds]);

  const clearSelection = useCallback(() => {
    setSelectedIds(new Set());
  }, []);

  const allSelected = threadIds.length > 0 && selectedIds.size === threadIds.length;
  const someSelected = selectedIds.size > 0 && !allSelected;

  return useMemo(
    () => ({
      selectedIds,
      toggleSelect,
      toggleSelectAll,
      clearSelection,
      allSelected,
      someSelected,
    }),
    [selectedIds, toggleSelect, toggleSelectAll, clearSelection, allSelected, someSelected]
  );
}
