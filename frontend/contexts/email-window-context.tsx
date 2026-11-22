"use client";

import { createContext, useContext, useState, useCallback, useRef } from "react";
import { toast } from "sonner";
import type { Draft } from "@/lib/types";

type WindowState = "open" | "minimized" | "closed";

interface ComposeData {
  draftId?: string;
  subject?: string;
  fromAddress?: string;
  toAddresses?: string[];
  ccAddresses?: string[];
  bccAddresses?: string[];
  bodyHtml?: string;
  bodyPlain?: string;
  /** Quoted text shown as read-only preview below the editor (replies/forwards) */
  quotedHtml?: string;
  attachmentIds?: string[];
  kind?: string;
  // Reply threading
  replyToThreadId?: string;
  inReplyTo?: string;
  references?: string[];
}

interface EmailWindowContextValue {
  composeState: WindowState;
  composeData: ComposeData | null;
  currentDraftId: string | undefined;
  openCompose: (data?: ComposeData) => void;
  openDraft: (draft: Draft) => void;
  minimizeCompose: () => void;
  restoreCompose: () => void;
  closeCompose: () => void;
  /** The compose window registers its save-now function here, so an open
   *  window is saved as a draft before new compose data replaces it. */
  registerFlush: (flush: FlushFn | null) => void;
}

/** Resolves to false when the save failed. */
type FlushFn = () => Promise<boolean | void>;

const EmailWindowContext = createContext<EmailWindowContextValue | null>(null);

export function EmailWindowProvider({ children }: { children: React.ReactNode }) {
  const [composeState, setComposeState] = useState<WindowState>("closed");
  const [composeData, setComposeData] = useState<ComposeData | null>(null);
  const flushRef = useRef<FlushFn | null>(null);

  const registerFlush = useCallback((flush: FlushFn | null) => {
    flushRef.current = flush;
  }, []);

  // flushCurrent saves the currently open compose window as a draft before
  // its content is replaced. Without this, opening Reply over an unsaved
  // compose silently discarded the body. It resolves to false when the save
  // failed: the caller then keeps the current window, because a replace
  // would lose the unsaved text.
  const flushCurrent = useCallback(async (state: WindowState) => {
    if (state === "closed") return true;
    try {
      const saved = await flushRef.current?.();
      if (saved !== false) return true;
    } catch {
      // Fall through to the failure path.
    }
    setComposeState("open");
    toast.error("The open message could not be saved. Close or send it first.");
    return false;
  }, []);

  const openCompose = useCallback(
    (data?: ComposeData) => {
      if (composeState === "closed" || !flushRef.current) {
        setComposeData(data || {});
        setComposeState("open");
        return;
      }
      void flushCurrent(composeState).then((saved) => {
        if (!saved) return;
        setComposeData(data || {});
        setComposeState("open");
      });
    },
    [composeState, flushCurrent]
  );

  const openDraft = useCallback(
    (draft: Draft) => {
      const apply = () => {
        setComposeData({
          draftId: draft.id,
          subject: draft.subject,
          fromAddress: draft.from_address,
          toAddresses: draft.to_addresses,
          ccAddresses: draft.cc_addresses,
          bccAddresses: draft.bcc_addresses,
          bodyHtml: draft.body_html,
          bodyPlain: draft.body_plain,
          attachmentIds: draft.attachment_ids,
          kind: draft.kind,
          // Keep the reply identity of the draft
          replyToThreadId: draft.thread_id || undefined,
          inReplyTo: draft.in_reply_to || undefined,
          references: draft.references_header
            ? draft.references_header.split(/\s+/).filter(Boolean)
            : undefined,
        });
        setComposeState("open");
      };
      if (composeState === "closed" || !flushRef.current) {
        apply();
        return;
      }
      // This draft is in the window already. Its text there is newer than
      // the list copy, so only show the window again.
      if (composeData?.draftId === draft.id) {
        setComposeState("open");
        return;
      }
      void flushCurrent(composeState).then((saved) => {
        if (saved) apply();
      });
    },
    [composeState, composeData, flushCurrent]
  );

  const minimizeCompose = useCallback(() => {
    setComposeState("minimized");
  }, []);

  const restoreCompose = useCallback(() => {
    setComposeState("open");
  }, []);

  const closeCompose = useCallback(() => {
    setComposeState("closed");
    setComposeData(null);
  }, []);

  return (
    <EmailWindowContext.Provider
      value={{
        composeState,
        composeData,
        currentDraftId: composeData?.draftId,
        openCompose,
        openDraft,
        minimizeCompose,
        restoreCompose,
        closeCompose,
        registerFlush,
      }}
    >
      {children}
    </EmailWindowContext.Provider>
  );
}

export function useEmailWindow() {
  const ctx = useContext(EmailWindowContext);
  if (!ctx) throw new Error("useEmailWindow must be used within EmailWindowProvider");
  return ctx;
}
