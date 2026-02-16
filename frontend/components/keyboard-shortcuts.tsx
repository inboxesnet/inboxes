"use client";

import { useEffect } from "react";
import { useRouter, usePathname } from "next/navigation";
import { useDomains } from "@/contexts/domain-context";
import { useThreadList } from "@/contexts/thread-list-context";

interface KeyboardShortcutsProps {
  onCompose: () => void;
}

export function KeyboardShortcuts({ onCompose }: KeyboardShortcutsProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { domains, activeDomain } = useDomains();
  const threadList = useThreadList();

  useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      // Ignore if user is typing in an input
      const target = e.target as HTMLElement;
      if (
        target.tagName === "INPUT" ||
        target.tagName === "TEXTAREA" ||
        target.isContentEditable
      ) {
        return;
      }

      const isMeta = e.metaKey || e.ctrlKey;

      // Cmd+K or / — search
      if ((isMeta && e.key === "k") || (e.key === "/" && !isMeta)) {
        e.preventDefault();
        if (activeDomain) {
          router.push(`/d/${activeDomain.id}/search`);
        }
        return;
      }

      // Cmd+N — compose
      if (isMeta && e.key === "n") {
        e.preventDefault();
        onCompose();
        return;
      }

      // Cmd+1-9 — switch domains
      if (isMeta && e.key >= "1" && e.key <= "9") {
        e.preventDefault();
        const idx = parseInt(e.key) - 1;
        if (idx < domains.length) {
          router.push(`/d/${domains[idx].id}/inbox`);
        }
        return;
      }

      // Thread-list shortcuts (no modifier, only when context is available)
      if (!isMeta && threadList && activeDomain) {
        const {
          threads,
          selectedIds,
          toggleSelect,
          handleBulkAction,
          handleRefresh,
          focusedIndex,
          setFocusedIndex,
          handleStar,
          handleAction,
          folder,
          domainId,
        } = threadList;

        switch (e.key) {
          case "j": {
            // Move focus down
            e.preventDefault();
            const next = Math.min(focusedIndex + 1, threads.length - 1);
            setFocusedIndex(next);
            return;
          }
          case "k": {
            // Move focus up
            e.preventDefault();
            const prev = Math.max(focusedIndex - 1, 0);
            setFocusedIndex(prev);
            return;
          }
          case "x": {
            // Toggle select on focused
            e.preventDefault();
            if (focusedIndex >= 0 && focusedIndex < threads.length) {
              toggleSelect(threads[focusedIndex].id);
            }
            return;
          }
          case "Enter":
          case "o": {
            // Open focused thread
            e.preventDefault();
            if (focusedIndex >= 0 && focusedIndex < threads.length) {
              router.push(`/d/${domainId}/${folder}/${threads[focusedIndex].id}`);
            }
            return;
          }
          case "e": {
            // Archive selected or focused
            e.preventDefault();
            if (selectedIds.size > 0) {
              handleBulkAction("archive");
            } else if (focusedIndex >= 0 && focusedIndex < threads.length) {
              handleAction(threads[focusedIndex].id, "archive");
            }
            return;
          }
          case "#": {
            // Trash selected or focused
            e.preventDefault();
            if (selectedIds.size > 0) {
              handleBulkAction("trash");
            } else if (focusedIndex >= 0 && focusedIndex < threads.length) {
              handleAction(threads[focusedIndex].id, "trash");
            }
            return;
          }
          case "s": {
            // Star focused
            e.preventDefault();
            if (focusedIndex >= 0 && focusedIndex < threads.length) {
              handleStar(threads[focusedIndex].id);
            }
            return;
          }
          case "r": {
            // Refresh
            e.preventDefault();
            handleRefresh();
            return;
          }
          case "I": {
            // Shift+I — mark read
            if (e.shiftKey) {
              e.preventDefault();
              if (selectedIds.size > 0) {
                handleBulkAction("read");
              } else if (focusedIndex >= 0 && focusedIndex < threads.length) {
                handleAction(threads[focusedIndex].id, "read");
              }
            }
            return;
          }
          case "U": {
            // Shift+U — mark unread
            if (e.shiftKey) {
              e.preventDefault();
              if (selectedIds.size > 0) {
                handleBulkAction("unread");
              } else if (focusedIndex >= 0 && focusedIndex < threads.length) {
                handleAction(threads[focusedIndex].id, "unread");
              }
            }
            return;
          }
        }
      }
    }

    document.addEventListener("keydown", handleKeyDown);
    return () => document.removeEventListener("keydown", handleKeyDown);
  }, [router, pathname, domains, activeDomain, onCompose, threadList]);

  return null;
}
