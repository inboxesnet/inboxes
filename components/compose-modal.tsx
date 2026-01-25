"use client";

import * as React from "react";
import { Bold, Italic, Link, List, ListOrdered, Send, Paperclip, X, FileIcon, ChevronDown } from "lucide-react";
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

export interface ComposePreFill {
  to?: string;
  cc?: string;
  subject?: string;
  bodyHtml?: string;
  fromAliasId?: string;
}

interface SendableAlias {
  id: string;
  address: string;
  name: string;
}

interface UserAliasesResponse {
  personal_email: string;
  aliases: SendableAlias[];
}

interface AttachedFile {
  id: string;
  filename: string;
  content_type: string;
  size: number;
  url: string;
  file?: File; // Local file before upload
  uploading?: boolean;
  error?: string;
}

interface ComposeModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  preFill?: ComposePreFill;
}

const MAX_TOTAL_SIZE = 50 * 1024 * 1024; // 50MB

function formatFileSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

export function ComposeModal({ open, onOpenChange, preFill }: ComposeModalProps) {
  const { addToast } = useToast();
  const [to, setTo] = React.useState("");
  const [subject, setSubject] = React.useState("");
  const [showCcBcc, setShowCcBcc] = React.useState(false);
  const [cc, setCc] = React.useState("");
  const [bcc, setBcc] = React.useState("");
  const [sending, setSending] = React.useState(false);
  const [error, setError] = React.useState("");
  const [attachments, setAttachments] = React.useState<AttachedFile[]>([]);
  const [isDragging, setIsDragging] = React.useState(false);
  const editorRef = React.useRef<HTMLDivElement>(null);
  const fileInputRef = React.useRef<HTMLInputElement>(null);

  // Alias selection state
  const [personalEmail, setPersonalEmail] = React.useState<string>("");
  const [aliases, setAliases] = React.useState<SendableAlias[]>([]);
  const [selectedFromId, setSelectedFromId] = React.useState<string>(""); // empty string = personal email
  const [showFromDropdown, setShowFromDropdown] = React.useState(false);
  const fromDropdownRef = React.useRef<HTMLDivElement>(null);

  // Fetch user aliases when modal opens
  React.useEffect(() => {
    if (open) {
      (async () => {
        try {
          const res = await fetch("/api/users/me/aliases");
          if (res.ok) {
            const data: UserAliasesResponse = await res.json();
            setPersonalEmail(data.personal_email);
            setAliases(data.aliases);
          }
        } catch {
          // Silently fail - user can still send from personal email
        }
      })();
    }
  }, [open]);

  // Close dropdown when clicking outside
  React.useEffect(() => {
    function handleClickOutside(event: MouseEvent) {
      if (fromDropdownRef.current && !fromDropdownRef.current.contains(event.target as Node)) {
        setShowFromDropdown(false);
      }
    }
    document.addEventListener("mousedown", handleClickOutside);
    return () => document.removeEventListener("mousedown", handleClickOutside);
  }, []);

  React.useEffect(() => {
    if (open && preFill) {
      if (preFill.to) setTo(preFill.to);
      if (preFill.cc) {
        setCc(preFill.cc);
        setShowCcBcc(true);
      }
      if (preFill.subject) setSubject(preFill.subject);
      if (preFill.bodyHtml && editorRef.current) {
        editorRef.current.innerHTML = preFill.bodyHtml;
      }
      if (preFill.fromAliasId) {
        setSelectedFromId(preFill.fromAliasId);
      }
    }
  }, [open, preFill]);

  function resetForm() {
    setTo("");
    setSubject("");
    setCc("");
    setBcc("");
    setShowCcBcc(false);
    setError("");
    setAttachments([]);
    setSelectedFromId("");
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

  function getTotalAttachmentSize(): number {
    return attachments.reduce((sum, a) => sum + a.size, 0);
  }

  async function handleFiles(files: FileList | File[]) {
    const fileArray = Array.from(files);
    const currentTotal = getTotalAttachmentSize();
    let newTotal = currentTotal;

    // Validate total size
    for (const file of fileArray) {
      newTotal += file.size;
    }

    if (newTotal > MAX_TOTAL_SIZE) {
      setError(`Total attachment size cannot exceed ${formatFileSize(MAX_TOTAL_SIZE)}`);
      return;
    }

    // Create temporary entries for each file
    const newAttachments: AttachedFile[] = fileArray.map((file) => ({
      id: `temp-${Date.now()}-${Math.random().toString(36).slice(2)}`,
      filename: file.name,
      content_type: file.type || "application/octet-stream",
      size: file.size,
      url: "",
      file,
      uploading: true,
    }));

    setAttachments((prev) => [...prev, ...newAttachments]);

    // Upload files
    const formData = new FormData();
    for (const file of fileArray) {
      formData.append("files", file);
    }

    try {
      const res = await fetch("/api/attachments/upload", {
        method: "POST",
        body: formData,
      });

      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        const errorMsg = (data as { error?: string }).error || "Failed to upload files";

        // Mark files as failed
        setAttachments((prev) =>
          prev.map((a) =>
            newAttachments.some((na) => na.id === a.id)
              ? { ...a, uploading: false, error: errorMsg }
              : a
          )
        );
        return;
      }

      const data = (await res.json()) as { files: AttachedFile[] };

      // Replace temp entries with uploaded entries
      setAttachments((prev) => {
        const nonTempFiles = prev.filter((a) => !newAttachments.some((na) => na.id === a.id));
        return [...nonTempFiles, ...data.files];
      });
    } catch {
      // Mark files as failed
      setAttachments((prev) =>
        prev.map((a) =>
          newAttachments.some((na) => na.id === a.id)
            ? { ...a, uploading: false, error: "Upload failed" }
            : a
        )
      );
    }
  }

  function handleAttachClick() {
    fileInputRef.current?.click();
  }

  function handleFileInputChange(e: React.ChangeEvent<HTMLInputElement>) {
    if (e.target.files && e.target.files.length > 0) {
      handleFiles(e.target.files);
      e.target.value = ""; // Reset input
    }
  }

  function handleRemoveAttachment(id: string) {
    setAttachments((prev) => prev.filter((a) => a.id !== id));
  }

  function handleDragOver(e: React.DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(true);
  }

  function handleDragLeave(e: React.DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);
  }

  function handleDrop(e: React.DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    setIsDragging(false);

    if (e.dataTransfer.files && e.dataTransfer.files.length > 0) {
      handleFiles(e.dataTransfer.files);
    }
  }

  function getSelectedFromAddress(): string {
    if (!selectedFromId) return personalEmail;
    const alias = aliases.find((a) => a.id === selectedFromId);
    return alias ? alias.address : personalEmail;
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

    // Check if any attachments are still uploading
    if (attachments.some((a) => a.uploading)) {
      setError("Please wait for attachments to finish uploading");
      return;
    }

    // Check for failed attachments
    if (attachments.some((a) => a.error)) {
      setError("Some attachments failed to upload. Please remove them and try again.");
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
          attachments: attachments.length > 0
            ? attachments.map((a) => ({
                filename: a.filename,
                content_type: a.content_type,
                url: a.url,
              }))
            : undefined,
          from_alias_id: selectedFromId || undefined,
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

          {/* From dropdown - only show if user has sendable aliases */}
          {aliases.length > 0 && (
            <div className="flex items-center gap-2">
              <Label className="w-12 text-sm text-muted-foreground">
                From
              </Label>
              <div className="relative flex-1" ref={fromDropdownRef}>
                <button
                  type="button"
                  onClick={() => setShowFromDropdown(!showFromDropdown)}
                  className="flex w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
                >
                  <span className="truncate">{getSelectedFromAddress()}</span>
                  <ChevronDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
                </button>
                {showFromDropdown && (
                  <div className="absolute z-50 mt-1 w-full rounded-md border bg-popover shadow-lg">
                    <div className="p-1">
                      <button
                        type="button"
                        onClick={() => {
                          setSelectedFromId("");
                          setShowFromDropdown(false);
                        }}
                        className={`flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent ${
                          !selectedFromId ? "bg-accent" : ""
                        }`}
                      >
                        <span className="truncate">{personalEmail}</span>
                        <span className="ml-2 text-xs text-muted-foreground">(Personal)</span>
                      </button>
                      {aliases.map((alias) => (
                        <button
                          key={alias.id}
                          type="button"
                          onClick={() => {
                            setSelectedFromId(alias.id);
                            setShowFromDropdown(false);
                          }}
                          className={`flex w-full items-center rounded-sm px-2 py-1.5 text-sm hover:bg-accent ${
                            selectedFromId === alias.id ? "bg-accent" : ""
                          }`}
                        >
                          <span className="truncate">{alias.address}</span>
                          {alias.name && (
                            <span className="ml-2 text-xs text-muted-foreground">({alias.name})</span>
                          )}
                        </button>
                      ))}
                    </div>
                  </div>
                )}
              </div>
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
            <div className="flex-1" />
            <Button
              type="button"
              variant="ghost"
              size="icon"
              className="h-8 w-8"
              onClick={handleAttachClick}
              title="Attach files"
            >
              <Paperclip className="h-4 w-4" />
            </Button>
            <input
              ref={fileInputRef}
              type="file"
              multiple
              className="hidden"
              onChange={handleFileInputChange}
            />
          </div>

          {/* ContentEditable editor with drag-drop support */}
          <div
            ref={editorRef}
            contentEditable
            className={`min-h-[200px] w-full rounded-md border bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 ${
              isDragging
                ? "border-primary border-2 bg-primary/5"
                : "border-input"
            }`}
            role="textbox"
            aria-label="Email body"
            aria-multiline="true"
            onDragOver={handleDragOver}
            onDragLeave={handleDragLeave}
            onDrop={handleDrop}
          />

          {/* Drag overlay hint */}
          {isDragging && (
            <div className="pointer-events-none absolute inset-0 flex items-center justify-center rounded-md bg-primary/10">
              <div className="rounded-lg bg-background px-4 py-2 shadow-lg">
                <p className="text-sm font-medium">Drop files to attach</p>
              </div>
            </div>
          )}

          {/* Attachment chips */}
          {attachments.length > 0 && (
            <div className="space-y-2">
              <div className="flex flex-wrap gap-2">
                {attachments.map((attachment) => (
                  <div
                    key={attachment.id}
                    className={`flex items-center gap-2 rounded-full border px-3 py-1 text-sm ${
                      attachment.error
                        ? "border-destructive bg-destructive/10"
                        : attachment.uploading
                          ? "border-muted bg-muted/50"
                          : "border-border bg-muted/30"
                    }`}
                  >
                    <FileIcon className="h-3 w-3 flex-shrink-0" />
                    <span className="max-w-[150px] truncate">{attachment.filename}</span>
                    <span className="text-xs text-muted-foreground">
                      {formatFileSize(attachment.size)}
                    </span>
                    {attachment.uploading && (
                      <span className="text-xs text-muted-foreground">Uploading...</span>
                    )}
                    {attachment.error && (
                      <span className="text-xs text-destructive">Failed</span>
                    )}
                    <button
                      type="button"
                      onClick={() => handleRemoveAttachment(attachment.id)}
                      className="ml-1 rounded-full p-0.5 hover:bg-muted"
                      title="Remove attachment"
                    >
                      <X className="h-3 w-3" />
                    </button>
                  </div>
                ))}
              </div>
              <p className="text-xs text-muted-foreground">
                Total: {formatFileSize(getTotalAttachmentSize())} / {formatFileSize(MAX_TOTAL_SIZE)}
              </p>
            </div>
          )}

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
