"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { Copy, Check } from "lucide-react";

interface ContactCardProps {
  email: string;
  children: React.ReactNode;
}

function getInitials(email: string): string {
  const local = email.split("@")[0] || "";
  return local.charAt(0).toUpperCase();
}

function getDisplayName(email: string): string {
  const local = email.split("@")[0] || email;
  return local
    .replace(/[._-]/g, " ")
    .replace(/\b\w/g, (c) => c.toUpperCase());
}

export function ContactCard({ email, children }: ContactCardProps) {
  const [open, setOpen] = useState(false);
  const [copied, setCopied] = useState(false);
  const triggerRef = useRef<HTMLButtonElement>(null);
  const cardRef = useRef<HTMLDivElement>(null);

  const close = useCallback(() => setOpen(false), []);

  useEffect(() => {
    if (!open) return;
    function handleClick(e: MouseEvent) {
      if (
        cardRef.current &&
        !cardRef.current.contains(e.target as Node) &&
        triggerRef.current &&
        !triggerRef.current.contains(e.target as Node)
      ) {
        close();
      }
    }
    function handleEscape(e: KeyboardEvent) {
      if (e.key === "Escape") close();
    }
    document.addEventListener("mousedown", handleClick);
    document.addEventListener("keydown", handleEscape);
    return () => {
      document.removeEventListener("mousedown", handleClick);
      document.removeEventListener("keydown", handleEscape);
    };
  }, [open, close]);

  async function handleCopy() {
    await navigator.clipboard.writeText(email);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  }

  return (
    <span className="relative inline-block">
      <span
        ref={triggerRef}
        role="button"
        tabIndex={0}
        onClick={(e) => {
          e.stopPropagation();
          setOpen(!open);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.stopPropagation();
            setOpen(!open);
          }
        }}
        className="hover:underline cursor-pointer text-left"
      >
        {children}
      </span>

      {open && (
        <div
          ref={cardRef}
          className="absolute left-0 top-full mt-1 z-50 bg-popover border rounded-lg shadow-lg p-3 w-[280px] animate-in fade-in-0 zoom-in-95"
        >
          <div className="flex items-start gap-3">
            <div className="h-10 w-10 rounded-full bg-primary/10 text-primary flex items-center justify-center text-sm font-semibold shrink-0">
              {getInitials(email)}
            </div>
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium truncate">
                {getDisplayName(email)}
              </p>
              <p className="text-xs text-muted-foreground truncate">{email}</p>
            </div>
          </div>
          <div className="mt-2 pt-2 border-t">
            <button
              onClick={handleCopy}
              className="flex items-center gap-1.5 text-xs text-muted-foreground hover:text-foreground w-full px-1 py-1 rounded hover:bg-muted transition-colors"
            >
              {copied ? (
                <Check className="h-3 w-3 text-green-600" />
              ) : (
                <Copy className="h-3 w-3" />
              )}
              {copied ? "Copied!" : "Copy email address"}
            </button>
          </div>
        </div>
      )}
    </span>
  );
}
