"use client";

import { useState, useCallback, useMemo, useRef } from "react";

export function useThreadSelection(threadIds: string[]) {
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set());
  const anchorIndexRef = useRef<number | null>(null);

  const toggleSelect = useCallback((id: string) => {
    anchorIndexRef.current = null;
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

  // selectClick handles a checkbox click with range support: a shift-click
  // selects everything between the last clicked row and this one.
  const selectClick = useCallback(
    (id: string, index: number, shiftKey: boolean) => {
      if (shiftKey && anchorIndexRef.current !== null) {
        const start = Math.min(anchorIndexRef.current, index);
        const end = Math.max(anchorIndexRef.current, index);
        const range = threadIds.slice(start, end + 1);
        setSelectedIds((prev) => {
          const next = new Set(prev);
          for (const rangeId of range) next.add(rangeId);
          return next;
        });
      } else {
        setSelectedIds((prev) => {
          const next = new Set(prev);
          if (next.has(id)) {
            next.delete(id);
          } else {
            next.add(id);
          }
          return next;
        });
      }
      anchorIndexRef.current = index;
    },
    [threadIds]
  );

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

  const selectIds = useCallback((ids: string[]) => {
    setSelectedIds(new Set(ids));
  }, []);

  return useMemo(
    () => ({
      selectedIds,
      toggleSelect,
      selectClick,
      toggleSelectAll,
      clearSelection,
      selectIds,
      allSelected,
      someSelected,
    }),
    [selectedIds, toggleSelect, selectClick, toggleSelectAll, clearSelection, selectIds, allSelected, someSelected]
  );
}
