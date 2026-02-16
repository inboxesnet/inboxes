"use client";

import { createContext, useContext, useState, useCallback } from "react";
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
}

interface EmailWindowContextValue {
  composeState: WindowState;
  composeData: ComposeData | null;
  openCompose: (data?: ComposeData) => void;
  openDraft: (draft: Draft) => void;
  minimizeCompose: () => void;
  restoreCompose: () => void;
  closeCompose: () => void;
}

const EmailWindowContext = createContext<EmailWindowContextValue | null>(null);

export function EmailWindowProvider({ children }: { children: React.ReactNode }) {
  const [composeState, setComposeState] = useState<WindowState>("closed");
  const [composeData, setComposeData] = useState<ComposeData | null>(null);

  const openCompose = useCallback((data?: ComposeData) => {
    setComposeData(data || {});
    setComposeState("open");
  }, []);

  const openDraft = useCallback((draft: Draft) => {
    setComposeData({
      draftId: draft.id,
      subject: draft.subject,
      fromAddress: draft.from_address,
      toAddresses: draft.to_addresses,
      ccAddresses: draft.cc_addresses,
      bccAddresses: draft.bcc_addresses,
      bodyHtml: draft.body_html,
      bodyPlain: draft.body_plain,
    });
    setComposeState("open");
  }, []);

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
        openCompose,
        openDraft,
        minimizeCompose,
        restoreCompose,
        closeCompose,
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
