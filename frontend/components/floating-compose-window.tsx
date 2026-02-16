"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import { useEmailWindow } from "@/contexts/email-window-context";
import { useDomains } from "@/contexts/domain-context";
import { api } from "@/lib/api";
import { TipTapEditor } from "@/components/tiptap-editor";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import {
  Minus,
  X,
  ChevronUp,
  Send,
} from "lucide-react";
import { cn } from "@/lib/utils";

export function FloatingComposeWindow() {
  const { composeState, composeData, minimizeCompose, restoreCompose, closeCompose } =
    useEmailWindow();
  const { activeDomain } = useDomains();

  const [fromAddress, setFromAddress] = useState("");
  const [to, setTo] = useState("");
  const [cc, setCc] = useState("");
  const [bcc, setBcc] = useState("");
  const [subject, setSubject] = useState("");
  const [bodyHtml, setBodyHtml] = useState("");
  const [bodyPlain, setBodyPlain] = useState("");
  const [showCcBcc, setShowCcBcc] = useState(false);
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [draftId, setDraftId] = useState<string | null>(null);
  const [saveStatus, setSaveStatus] = useState<"" | "saving" | "saved">("");
  const saveTimerRef = useRef<NodeJS.Timeout | null>(null);
  const dirtyRef = useRef(false);

  // Initialize form from composeData when window opens
  useEffect(() => {
    if (composeState === "open" && composeData) {
      setFromAddress(composeData.fromAddress || "");
      setTo((composeData.toAddresses || []).join(", "));
      setCc((composeData.ccAddresses || []).join(", "));
      setBcc((composeData.bccAddresses || []).join(", "));
      setSubject(composeData.subject || "");
      setBodyHtml(composeData.bodyHtml || "");
      setBodyPlain(composeData.bodyPlain || "");
      setDraftId(composeData.draftId || null);
      setShowCcBcc(
        (composeData.ccAddresses?.length || 0) > 0 ||
        (composeData.bccAddresses?.length || 0) > 0
      );
      setError("");
      setSaveStatus("");
      dirtyRef.current = false;
    }
  }, [composeState, composeData]);

  // Reset form on close
  useEffect(() => {
    if (composeState === "closed") {
      setFromAddress("");
      setTo("");
      setCc("");
      setBcc("");
      setSubject("");
      setBodyHtml("");
      setBodyPlain("");
      setDraftId(null);
      setShowCcBcc(false);
      setError("");
      setSaveStatus("");
      dirtyRef.current = false;
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    }
  }, [composeState]);

  const saveDraft = useCallback(async () => {
    if (!activeDomain) return;
    const toAddresses = to.split(",").map((s) => s.trim()).filter(Boolean);
    const ccAddresses = cc ? cc.split(",").map((s) => s.trim()).filter(Boolean) : [];
    const bccAddresses = bcc ? bcc.split(",").map((s) => s.trim()).filter(Boolean) : [];

    // Don't save empty drafts
    if (!subject && !bodyPlain && toAddresses.length === 0) return;

    setSaveStatus("saving");
    try {
      if (draftId) {
        await api.patch(`/api/drafts/${draftId}`, {
          subject,
          from_address: fromAddress || `me@${activeDomain.domain}`,
          to_addresses: toAddresses,
          cc_addresses: ccAddresses,
          bcc_addresses: bccAddresses,
          body_html: bodyHtml,
          body_plain: bodyPlain,
        });
      } else {
        const res = await api.post<{ id: string }>("/api/drafts", {
          domain_id: activeDomain.id,
          kind: "compose",
          subject,
          from_address: fromAddress || `me@${activeDomain.domain}`,
          to_addresses: toAddresses,
          cc_addresses: ccAddresses,
          bcc_addresses: bccAddresses,
          body_html: bodyHtml,
          body_plain: bodyPlain,
        });
        setDraftId(res.id);
      }
      setSaveStatus("saved");
      dirtyRef.current = false;
    } catch {
      setSaveStatus("");
    }
  }, [activeDomain, draftId, to, cc, bcc, subject, fromAddress, bodyHtml, bodyPlain]);

  // Debounced auto-save
  const scheduleSave = useCallback(() => {
    dirtyRef.current = true;
    setSaveStatus("");
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    saveTimerRef.current = setTimeout(() => {
      saveDraft();
    }, 3000);
  }, [saveDraft]);

  // Save on close if dirty
  const handleClose = useCallback(() => {
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    if (dirtyRef.current) {
      saveDraft();
    }
    closeCompose();
  }, [closeCompose, saveDraft]);

  async function handleSend(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const toAddresses = to.split(",").map((s) => s.trim()).filter(Boolean);
    if (toAddresses.length === 0 || !subject) {
      setError("To and Subject are required");
      return;
    }

    setSending(true);
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);

    try {
      if (draftId) {
        // Update draft then send it
        await api.patch(`/api/drafts/${draftId}`, {
          subject,
          from_address: fromAddress || `me@${activeDomain?.domain}`,
          to_addresses: toAddresses,
          cc_addresses: cc ? cc.split(",").map((s) => s.trim()).filter(Boolean) : [],
          bcc_addresses: bcc ? bcc.split(",").map((s) => s.trim()).filter(Boolean) : [],
          body_html: bodyHtml,
          body_plain: bodyPlain,
        });
        await api.post(`/api/drafts/${draftId}/send`);
      } else {
        await api.post("/api/emails/send", {
          from: fromAddress || `me@${activeDomain?.domain}`,
          to: toAddresses,
          cc: cc ? cc.split(",").map((s) => s.trim()).filter(Boolean) : [],
          bcc: bcc ? bcc.split(",").map((s) => s.trim()).filter(Boolean) : [],
          subject,
          html: bodyHtml,
          text: bodyPlain,
          domain_id: activeDomain?.id,
        });
      }
      closeCompose();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to send");
    } finally {
      setSending(false);
    }
  }

  if (composeState === "closed") return null;

  // Minimized state
  if (composeState === "minimized") {
    return (
      <div
        className="fixed bottom-0 right-6 z-50 w-[320px] bg-background border border-b-0 rounded-t-lg shadow-lg cursor-pointer"
        onClick={restoreCompose}
      >
        <div className="flex items-center justify-between px-3 py-2">
          <span className="text-sm font-medium truncate">
            {subject || "New Message"}
          </span>
          <div className="flex items-center gap-1">
            <button
              onClick={(e) => {
                e.stopPropagation();
                restoreCompose();
              }}
              className="p-0.5 hover:bg-muted rounded"
            >
              <ChevronUp className="h-4 w-4" />
            </button>
            <button
              onClick={(e) => {
                e.stopPropagation();
                handleClose();
              }}
              className="p-0.5 hover:bg-muted rounded"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>
    );
  }

  // Open state
  return (
    <div className="fixed bottom-0 right-6 z-50 w-[520px] bg-background border border-b-0 rounded-t-lg shadow-2xl flex flex-col max-h-[600px]">
      {/* Title bar */}
      <div className="flex items-center justify-between px-3 py-2 border-b bg-muted/30 rounded-t-lg shrink-0">
        <span className="text-sm font-medium">New Message</span>
        <div className="flex items-center gap-1">
          {saveStatus === "saving" && (
            <span className="text-xs text-muted-foreground mr-1">Saving...</span>
          )}
          {saveStatus === "saved" && (
            <span className="text-xs text-muted-foreground mr-1">Saved</span>
          )}
          <button
            onClick={minimizeCompose}
            className="p-0.5 hover:bg-muted rounded"
            title="Minimize"
          >
            <Minus className="h-4 w-4" />
          </button>
          <button
            onClick={handleClose}
            className="p-0.5 hover:bg-muted rounded"
            title="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Form */}
      <form onSubmit={handleSend} className="flex flex-col flex-1 min-h-0">
        <div className="px-3 py-1 space-y-1 shrink-0">
          {error && (
            <div className="text-xs text-destructive bg-destructive/10 p-2 rounded">
              {error}
            </div>
          )}
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">From</label>
            <Input
              value={fromAddress}
              onChange={(e) => { setFromAddress(e.target.value); scheduleSave(); }}
              placeholder={`me@${activeDomain?.domain || "example.com"}`}
              className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
            />
          </div>
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">To</label>
            <Input
              value={to}
              onChange={(e) => { setTo(e.target.value); scheduleSave(); }}
              placeholder="recipient@example.com"
              className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
            />
            {!showCcBcc && (
              <button
                type="button"
                onClick={() => setShowCcBcc(true)}
                className="text-xs text-muted-foreground hover:text-foreground shrink-0"
              >
                Cc Bcc
              </button>
            )}
          </div>
          {showCcBcc && (
            <>
              <div className="flex items-center gap-2">
                <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Cc</label>
                <Input
                  value={cc}
                  onChange={(e) => { setCc(e.target.value); scheduleSave(); }}
                  placeholder="cc@example.com"
                  className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
                />
              </div>
              <div className="flex items-center gap-2">
                <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Bcc</label>
                <Input
                  value={bcc}
                  onChange={(e) => { setBcc(e.target.value); scheduleSave(); }}
                  placeholder="bcc@example.com"
                  className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
                />
              </div>
            </>
          )}
          <div className="flex items-center gap-2">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Subj</label>
            <Input
              value={subject}
              onChange={(e) => { setSubject(e.target.value); scheduleSave(); }}
              placeholder="Subject"
              className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
            />
          </div>
        </div>

        <div className="border-t mx-3" />

        {/* Editor */}
        <div className="flex-1 overflow-y-auto min-h-0 px-3 py-1">
          <TipTapEditor
            content={bodyHtml}
            onChange={(html, plain) => {
              setBodyHtml(html);
              setBodyPlain(plain);
              scheduleSave();
            }}
            autofocus
            className="border-0"
          />
        </div>

        {/* Footer */}
        <div className="flex items-center justify-between px-3 py-2 border-t shrink-0">
          <Button type="submit" size="sm" disabled={sending}>
            {sending ? <Spinner className="mr-1 h-3 w-3" /> : <Send className="mr-1 h-3 w-3" />}
            Send
          </Button>
          <button
            type="button"
            onClick={handleClose}
            className="text-xs text-muted-foreground hover:text-foreground"
          >
            Discard
          </button>
        </div>
      </form>
    </div>
  );
}
