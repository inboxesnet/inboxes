"use client";

import { useEffect, useState } from "react";
import { useRouter, usePathname } from "next/navigation";
import { useDomains } from "@/contexts/domain-context";
import { useThreadList } from "@/contexts/thread-list-context";
import { KeyboardShortcutsDialog } from "@/components/keyboard-shortcuts-dialog";

interface KeyboardShortcutsProps {
  onCompose: () => void;
}

export function KeyboardShortcuts({ onCompose }: KeyboardShortcutsProps) {
  const router = useRouter();
  const pathname = usePathname();
  const { domains, activeDomain } = useDomains();
  const threadList = useThreadList();
  const [shortcutsOpen, setShortcutsOpen] = useState(false);

  // Listen for custom event from sidebar button
  useEffect(() => {
    function handleOpen() { setShortcutsOpen(true); }
    window.addEventListener("open-shortcuts-dialog", handleOpen);
    return () => window.removeEventListener("open-shortcuts-dialog", handleOpen);
  }, []);

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

      // Cmd+K or / — focus inline search
      if ((isMeta && e.key === "k") || (e.key === "/" && !isMeta)) {
        e.preventDefault();
        window.dispatchEvent(new Event("focus-search"));
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

      // ? — open keyboard shortcuts dialog
      if (e.key === "?" && !isMeta) {
        e.preventDefault();
        setShortcutsOpen(true);
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
          label,
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
            // Open focused thread (split-pane if available, else navigate)
            e.preventDefault();
            if (focusedIndex >= 0 && focusedIndex < threads.length) {
              const tid = threads[focusedIndex].id;
              if (threadList.onThreadClick) {
                threadList.onThreadClick(tid);
              } else {
                router.push(`/d/${domainId}/${label}/${tid}`);
              }
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
          case "m": {
            // Mute/unmute focused or selected
            e.preventDefault();
            if (selectedIds.size > 0) {
              const selectedThreads = threads.filter((t) => selectedIds.has(t.id));
              const allMuted = selectedThreads.every((t) => t.labels?.includes("muted"));
              handleBulkAction(allMuted ? "unmute" : "mute");
            } else if (focusedIndex >= 0 && focusedIndex < threads.length) {
              handleAction(threads[focusedIndex].id, "mute");
            }
            return;
          }
          case "v": {
            // Move to — dispatch custom event to open move dialog in toolbar
            e.preventDefault();
            window.dispatchEvent(new CustomEvent("open-move-dialog"));
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

  return <KeyboardShortcutsDialog open={shortcutsOpen} onOpenChange={setShortcutsOpen} />;
}
