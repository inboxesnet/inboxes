"use client";

import * as React from "react";
import { Bold, Italic, Link, List, ListOrdered, Send } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/components/ui/toast";

interface ComposeModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function ComposeModal({ open, onOpenChange }: ComposeModalProps) {
  const { addToast } = useToast();
  const [to, setTo] = React.useState("");
  const [subject, setSubject] = React.useState("");
  const [showCcBcc, setShowCcBcc] = React.useState(false);
  const [cc, setCc] = React.useState("");
  const [bcc, setBcc] = React.useState("");
  const [sending, setSending] = React.useState(false);
  const [error, setError] = React.useState("");
  const editorRef = React.useRef<HTMLDivElement>(null);

  function resetForm() {
    setTo("");
    setSubject("");
    setCc("");
    setBcc("");
    setShowCcBcc(false);
    setError("");
    if (editorRef.current) {
      editorRef.current.innerHTML = "";
    }
  }

  function execCommand(command: string, value?: string) {
    document.execCommand(command, false, value);
    editorRef.current?.focus();
  }

  function handleBold() {
    execCommand("bold");
  }

  function handleItalic() {
    execCommand("italic");
  }

  function handleLink() {
    const url = prompt("Enter URL:");
    if (url) {
      execCommand("createLink", url);
    }
  }

  function handleUnorderedList() {
    execCommand("insertUnorderedList");
  }

  function handleOrderedList() {
    execCommand("insertOrderedList");
  }

  async function handleSend() {
    setError("");

    if (!to.trim()) {
      setError("Recipient (To) is required");
      return;
    }

    if (!subject.trim()) {
      setError("Subject is required");
      return;
    }

    const bodyHtml = editorRef.current?.innerHTML || "";
    const bodyPlain = editorRef.current?.innerText || "";

    if (!bodyPlain.trim()) {
      setError("Email body is required");
      return;
    }

    // Parse email addresses (comma separated)
    const toAddresses = to.split(",").map((e) => e.trim()).filter(Boolean);
    const ccAddresses = cc ? cc.split(",").map((e) => e.trim()).filter(Boolean) : [];
    const bccAddresses = bcc ? bcc.split(",").map((e) => e.trim()).filter(Boolean) : [];

    setSending(true);

    try {
      const res = await fetch("/api/emails/send", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          to: toAddresses,
          cc: ccAddresses.length > 0 ? ccAddresses : undefined,
          bcc: bccAddresses.length > 0 ? bccAddresses : undefined,
          subject,
          body_html: bodyHtml,
          body_plain: bodyPlain,
        }),
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        setError((data as { error?: string }).error || "Failed to send email");
        setSending(false);
        return;
      }

      addToast("Email sent successfully", "success");
      resetForm();
      onOpenChange(false);
    } catch {
      setError("Failed to send email. Please try again.");
    } finally {
      setSending(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-2xl">
        <DialogHeader>
          <DialogTitle>New Message</DialogTitle>
        </DialogHeader>

        <div className="mt-4 space-y-3">
          {error && (
            <div className="rounded-md border border-destructive bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="space-y-1">
            <div className="flex items-center gap-2">
              <Label htmlFor="compose-to" className="w-12 text-sm text-muted-foreground">
                To
              </Label>
              <Input
                id="compose-to"
                type="text"
                placeholder="recipient@example.com"
                value={to}
                onChange={(e) => setTo(e.target.value)}
                className="flex-1"
              />
              {!showCcBcc && (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => setShowCcBcc(true)}
                  className="text-xs text-muted-foreground"
                >
                  CC/BCC
                </Button>
              )}
            </div>
          </div>

          {showCcBcc && (
            <>
              <div className="flex items-center gap-2">
                <Label htmlFor="compose-cc" className="w-12 text-sm text-muted-foreground">
                  CC
                </Label>
                <Input
                  id="compose-cc"
                  type="text"
                  placeholder="cc@example.com"
                  value={cc}
                  onChange={(e) => setCc(e.target.value)}
                  className="flex-1"
                />
              </div>
              <div className="flex items-center gap-2">
                <Label htmlFor="compose-bcc" className="w-12 text-sm text-muted-foreground">
                  BCC
                </Label>
                <Input
                  id="compose-bcc"
                  type="text"
                  placeholder="bcc@example.com"
                  value={bcc}
                  onChange={(e) => setBcc(e.target.value)}
                  className="flex-1"
                />
              </div>
            </>
          )}

          <div className="flex items-center gap-2">
            <Label htmlFor="compose-subject" className="w-12 text-sm text-muted-foreground">
              Subject
            </Label>
            <Input
              id="compose-subject"
              type="text"
              placeholder="Subject"
              value={subject}
              onChange={(e) => setSubject(e.target.value)}
              className="flex-1"
            />
          </div>

          {/* Rich text toolbar */}
          <div className="flex items-center gap-1 border-b pb-2">
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={handleBold}
              title="Bold"
            >
              <Bold className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={handleItalic}
              title="Italic"
            >
              <Italic className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={handleLink}
              title="Insert Link"
            >
              <Link className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={handleUnorderedList}
              title="Bullet List"
            >
              <List className="h-4 w-4" />
            </Button>
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={handleOrderedList}
              title="Numbered List"
            >
              <ListOrdered className="h-4 w-4" />
            </Button>
          </div>

          {/* ContentEditable editor */}
          <div
            ref={editorRef}
            contentEditable
            className="min-h-[200px] w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
            role="textbox"
            aria-label="Email body"
            aria-multiline="true"
          />

          {/* Actions */}
          <div className="flex items-center justify-end gap-2 pt-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                resetForm();
                onOpenChange(false);
              }}
            >
              Discard
            </Button>
            <Button
              type="button"
              onClick={handleSend}
              disabled={sending}
            >
              {sending ? (
                "Sending..."
              ) : (
                <>
                  <Send className="mr-2 h-4 w-4" />
                  Send
                </>
              )}
            </Button>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
