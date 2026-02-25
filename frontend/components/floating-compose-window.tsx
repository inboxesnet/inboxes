"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import DOMPurify from "dompurify";
import { useEmailWindow } from "@/contexts/email-window-context";
import { useDomains } from "@/contexts/domain-context";
import { api } from "@/lib/api";
import { TipTapEditor } from "@/components/tiptap-editor";
import { RecipientInput } from "@/components/recipient-input";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Spinner } from "@/components/ui/spinner";
import {
  Minus,
  X,
  ChevronUp,
  Send,
  Trash2,
} from "lucide-react";
import { cn } from "@/lib/utils";

interface MyAlias {
  id: string;
  address: string;
  name: string;
  domain_id: string;
  can_send_as: boolean;
  is_default: boolean;
}

export function FloatingComposeWindow() {
  const { composeState, composeData, minimizeCompose, restoreCompose, closeCompose } =
    useEmailWindow();
  const { activeDomain } = useDomains();

  const [fromAddress, setFromAddress] = useState("");
  const [to, setTo] = useState<string[]>([]);
  const [cc, setCc] = useState<string[]>([]);
  const [bcc, setBcc] = useState<string[]>([]);
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

  // Alias state
  const [aliases, setAliases] = useState<MyAlias[]>([]);

  // Fetch aliases when compose opens
  useEffect(() => {
    if (composeState !== "open" || !activeDomain) return;
    api.get<MyAlias[]>("/api/users/me/aliases").then((data) => {
      const domainAliases = data.filter(
        (a) => a.domain_id === activeDomain.id && a.can_send_as
      );
      setAliases(domainAliases);
    }).catch(() => {});
  }, [composeState, activeDomain]);

  // Pick best default from address
  function pickDefaultFrom(presetFrom?: string): string {
    if (presetFrom) return presetFrom;
    const domain = activeDomain?.domain || "example.com";
    if (aliases.length > 0) {
      const defaultAlias = aliases.find((a) => a.is_default);
      if (defaultAlias) return defaultAlias.address;
      const hello = aliases.find((a) => a.address.startsWith("hello@"));
      return hello ? hello.address : aliases[0].address;
    }
    return `hello@${domain}`;
  }

  // Initialize form from composeData when window opens
  useEffect(() => {
    if (composeState === "open" && composeData) {
      setTo(composeData.toAddresses || []);
      setCc(composeData.ccAddresses || []);
      setBcc(composeData.bccAddresses || []);
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

  // Set from address once aliases are loaded
  useEffect(() => {
    if (composeState === "open") {
      setFromAddress(pickDefaultFrom(composeData?.fromAddress));
    }
  }, [composeState, aliases, composeData?.fromAddress, activeDomain]);

  // Reset form on close
  useEffect(() => {
    if (composeState === "closed") {
      setFromAddress("");
      setTo([]);
      setCc([]);
      setBcc([]);
      setSubject("");
      setBodyHtml("");
      setBodyPlain("");
      setDraftId(null);
      setShowCcBcc(false);
      setError("");
      setSaveStatus("");
      setAliases([]);
      dirtyRef.current = false;
      if (saveTimerRef.current) clearTimeout(saveTimerRef.current);
    }
  }, [composeState]);

  const effectiveFrom = fromAddress || `hello@${activeDomain?.domain || "example.com"}`;

  const saveDraft = useCallback(async () => {
    if (!activeDomain) return;

    // Don't save empty drafts
    if (!subject && !bodyPlain && to.length === 0) return;

    setSaveStatus("saving");
    try {
      if (draftId) {
        await api.patch(`/api/drafts/${draftId}`, {
          subject,
          from_address: effectiveFrom,
          to_addresses: to,
          cc_addresses: cc,
          bcc_addresses: bcc,
          body_html: bodyHtml,
          body_plain: bodyPlain,
        });
      } else {
        const res = await api.post<{ id: string }>("/api/drafts", {
          domain_id: activeDomain.id,
          kind: "compose",
          subject,
          from_address: effectiveFrom,
          to_addresses: to,
          cc_addresses: cc,
          bcc_addresses: bcc,
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
  }, [activeDomain, draftId, to, cc, bcc, subject, effectiveFrom, bodyHtml, bodyPlain]);

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

    if (to.length === 0 || !subject) {
      setError("To and Subject are required");
      return;
    }

    setSending(true);
    if (saveTimerRef.current) clearTimeout(saveTimerRef.current);

    // Combine user's text with quoted text for the actual send
    const fullHtml = composeData?.quotedHtml ? bodyHtml + composeData.quotedHtml : bodyHtml;

    try {
      if (draftId) {
        // Update draft then send it
        await api.patch(`/api/drafts/${draftId}`, {
          subject,
          from_address: effectiveFrom,
          to_addresses: to,
          cc_addresses: cc,
          bcc_addresses: bcc,
          body_html: fullHtml,
          body_plain: bodyPlain,
        });
        await api.post(`/api/drafts/${draftId}/send`);
      } else {
        const payload: Record<string, unknown> = {
          from: effectiveFrom,
          to,
          cc,
          bcc,
          subject,
          html: fullHtml,
          text: bodyPlain,
          domain_id: activeDomain?.id,
        };
        // Include reply threading headers when replying
        if (composeData?.replyToThreadId) {
          payload.reply_to_thread_id = composeData.replyToThreadId;
        }
        if (composeData?.inReplyTo) {
          payload.in_reply_to = composeData.inReplyTo;
        }
        if (composeData?.references?.length) {
          payload.references = composeData.references;
        }
        await api.post("/api/emails/send", payload);
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
        className="fixed bottom-0 right-0 md:right-6 z-50 w-full md:w-[320px] bg-foreground text-background border border-b-0 md:rounded-t-lg shadow-lg cursor-pointer"
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
              className="p-0.5 hover:bg-white/10 rounded"
            >
              <ChevronUp className="h-4 w-4" />
            </button>
            <button
              onClick={(e) => {
                e.stopPropagation();
                handleClose();
              }}
              className="p-0.5 hover:bg-white/10 rounded"
            >
              <X className="h-4 w-4" />
            </button>
          </div>
        </div>
      </div>
    );
  }

  // ── Mobile: full-screen compose ──
  const mobileCompose = (
    <div className="fixed inset-0 z-50 bg-background flex flex-col md:hidden">
      {/* Mobile title bar */}
      <div className="flex items-center justify-between px-3 py-2 border-b shrink-0">
        <button onClick={handleClose} className="p-1 -ml-1 hover:bg-muted rounded">
          <X className="h-5 w-5" />
        </button>
        <span className="text-sm font-medium">
          {saveStatus === "saving" ? "Saving..." : saveStatus === "saved" ? "Saved" : "New Message"}
        </span>
        <Button type="submit" size="sm" form="compose-form" disabled={sending}>
          {sending ? <Spinner className="mr-1 h-3 w-3" /> : <Send className="mr-1 h-3 w-3" />}
          Send
        </Button>
      </div>

      <form id="compose-form" onSubmit={handleSend} className="flex flex-col flex-1 min-h-0">
        <div className="px-3 py-1 space-y-1 shrink-0">
          {error && (
            <div className="text-xs text-destructive bg-destructive/10 p-2 rounded">
              {error}
            </div>
          )}
          {aliases.length !== 1 && (
            <div className="flex items-center gap-2 border-b pb-1">
              <label className="text-xs text-muted-foreground w-10 text-right shrink-0">From</label>
              {aliases.length > 1 ? (
                <select
                  value={fromAddress}
                  onChange={(e) => { setFromAddress(e.target.value); scheduleSave(); }}
                  className="flex-1 h-7 text-sm border-0 bg-transparent shadow-none focus-visible:outline-none focus-visible:ring-0 px-1 cursor-pointer"
                >
                  {aliases.map((a) => (
                    <option key={a.id} value={a.address}>
                      {a.name ? `${a.name} <${a.address}>` : a.address}
                    </option>
                  ))}
                </select>
              ) : (
                <Input
                  value={fromAddress}
                  onChange={(e) => { setFromAddress(e.target.value); scheduleSave(); }}
                  placeholder={`hello@${activeDomain?.domain || "example.com"}`}
                  className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
                />
              )}
            </div>
          )}
          <div className="flex items-center gap-2 border-b pb-1">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">To</label>
            <RecipientInput
              value={to}
              onChange={(v) => { setTo(v); scheduleSave(); }}
              placeholder="recipient@example.com"
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
              <div className="flex items-center gap-2 border-b pb-1">
                <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Cc</label>
                <RecipientInput
                  value={cc}
                  onChange={(v) => { setCc(v); scheduleSave(); }}
                  placeholder="cc@example.com"
                />
              </div>
              <div className="flex items-center gap-2 border-b pb-1">
                <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Bcc</label>
                <RecipientInput
                  value={bcc}
                  onChange={(v) => { setBcc(v); scheduleSave(); }}
                  placeholder="bcc@example.com"
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

        {/* Editor — full toolbar on mobile */}
        <div className="flex-1 overflow-y-auto min-h-0 px-3 py-1">
          <TipTapEditor
            content={bodyHtml}
            onChange={(html, plain) => {
              setBodyHtml(html);
              setBodyPlain(plain);
              scheduleSave();
            }}
            autofocus
            className="border-0 rounded-none"
          />
          {composeData?.quotedHtml && (
            <div
              className="text-xs text-muted-foreground border-l-2 border-muted pl-3 mx-3 mt-2 max-h-[200px] overflow-y-auto prose prose-xs max-w-none"
              dangerouslySetInnerHTML={{
                __html: DOMPurify.sanitize(composeData.quotedHtml, {
                  ALLOWED_TAGS: ["p", "br", "strong", "em", "a", "div", "span", "blockquote"],
                  ALLOWED_ATTR: ["href"],
                }),
              }}
            />
          )}
        </div>
      </form>
    </div>
  );

  // ── Desktop: floating compose window ──
  const desktopCompose = (
    <div className="hidden md:flex fixed bottom-0 right-6 z-50 w-[520px] bg-background border border-b-0 rounded-t-lg shadow-2xl flex-col h-[500px] max-h-[70vh]">
      {/* Title bar — dark like Gmail */}
      <div className="flex items-center justify-between px-3 py-2 border-b bg-foreground text-background rounded-t-lg shrink-0">
        <span className="text-sm font-medium">New Message</span>
        <div className="flex items-center gap-1">
          {saveStatus === "saving" && (
            <span className="text-xs opacity-60 mr-1">Saving...</span>
          )}
          {saveStatus === "saved" && (
            <span className="text-xs opacity-60 mr-1">Saved</span>
          )}
          <button
            onClick={minimizeCompose}
            className="p-0.5 hover:bg-white/10 rounded"
            title="Minimize"
          >
            <Minus className="h-4 w-4" />
          </button>
          <button
            onClick={handleClose}
            className="p-0.5 hover:bg-white/10 rounded"
            title="Close"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {/* Form */}
      <form onSubmit={handleSend} className="flex flex-col flex-1 min-h-0">
        {/* Fields — clean rows with border-b separators */}
        <div className="shrink-0">
          {error && (
            <div className="text-xs text-destructive bg-destructive/10 p-2 mx-3 mt-2 rounded">
              {error}
            </div>
          )}
          {aliases.length !== 1 && (
            <div className="flex items-center gap-2 px-3 border-b min-h-[36px]">
              <label className="text-xs text-muted-foreground w-10 text-right shrink-0">From</label>
              {aliases.length > 1 ? (
                <select
                  value={fromAddress}
                  onChange={(e) => { setFromAddress(e.target.value); scheduleSave(); }}
                  className="flex-1 h-7 text-sm border-0 bg-transparent shadow-none focus-visible:outline-none focus-visible:ring-0 px-1 cursor-pointer"
                >
                  {aliases.map((a) => (
                    <option key={a.id} value={a.address}>
                      {a.name ? `${a.name} <${a.address}>` : a.address}
                    </option>
                  ))}
                </select>
              ) : (
                <Input
                  value={fromAddress}
                  onChange={(e) => { setFromAddress(e.target.value); scheduleSave(); }}
                  placeholder={`hello@${activeDomain?.domain || "example.com"}`}
                  className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
                />
              )}
            </div>
          )}
          <div className="flex items-center gap-2 px-3 border-b min-h-[36px]">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">To</label>
            <RecipientInput
              value={to}
              onChange={(v) => { setTo(v); scheduleSave(); }}
              placeholder="recipient@example.com"
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
              <div className="flex items-center gap-2 px-3 border-b min-h-[36px]">
                <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Cc</label>
                <RecipientInput
                  value={cc}
                  onChange={(v) => { setCc(v); scheduleSave(); }}
                  placeholder="cc@example.com"
                />
              </div>
              <div className="flex items-center gap-2 px-3 border-b min-h-[36px]">
                <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Bcc</label>
                <RecipientInput
                  value={bcc}
                  onChange={(v) => { setBcc(v); scheduleSave(); }}
                  placeholder="bcc@example.com"
                />
              </div>
            </>
          )}
          <div className="flex items-center gap-2 px-3 border-b min-h-[36px]">
            <label className="text-xs text-muted-foreground w-10 text-right shrink-0">Subj</label>
            <Input
              value={subject}
              onChange={(e) => { setSubject(e.target.value); scheduleSave(); }}
              placeholder="Subject"
              className="h-7 text-sm border-0 shadow-none focus-visible:ring-0 px-1"
            />
          </div>
        </div>

        {/* Editor — toolbar merged with Send into one bottom bar */}
        <div className="flex-1 min-h-0 flex flex-col">
          <TipTapEditor
            content={bodyHtml}
            onChange={(html, plain) => {
              setBodyHtml(html);
              setBodyPlain(plain);
              scheduleSave();
            }}
            autofocus
            className="border-0 rounded-none flex-1"
            quotedHtml={composeData?.quotedHtml}
            toolbarLeft={
              <Button type="submit" size="sm" disabled={sending} className="mx-1 shrink-0">
                {sending ? <Spinner className="mr-1 h-3 w-3" /> : <Send className="mr-1 h-3 w-3" />}
                Send
              </Button>
            }
            toolbarRight={
              <button
                type="button"
                onClick={handleClose}
                className="p-1.5 rounded hover:bg-muted text-muted-foreground hover:text-foreground transition-colors mx-1"
                title="Discard"
              >
                <Trash2 className="h-3.5 w-3.5" />
              </button>
            }
          />
        </div>
      </form>
    </div>
  );

  return (
    <>
      {mobileCompose}
      {desktopCompose}
    </>
  );
}
