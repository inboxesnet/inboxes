"use client";

import * as React from "react";
import { cn } from "@/lib/utils";
import { X } from "lucide-react";

interface DialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  children: React.ReactNode;
}

// Stack of open dialogs. Only the top dialog reacts to Escape, so a nested
// confirm dialog no longer closes its parent with the same keypress.
const dialogStack: symbol[] = [];

const FOCUSABLE_SELECTOR =
  'button:not([disabled]), [href], input:not([disabled]), select:not([disabled]), textarea:not([disabled]), [tabindex]:not([tabindex="-1"])';

function Dialog({ open, onOpenChange, children }: DialogProps) {
  const containerRef = React.useRef<HTMLDivElement>(null);
  const idRef = React.useRef<symbol | null>(null);

  React.useEffect(() => {
    if (!open) return;

    const id = Symbol("dialog");
    idRef.current = id;
    dialogStack.push(id);

    const previouslyFocused = document.activeElement as HTMLElement | null;

    // Move focus into the dialog
    const container = containerRef.current;
    const focusables = () =>
      Array.from(container?.querySelectorAll<HTMLElement>(FOCUSABLE_SELECTOR) ?? []);
    const first = focusables()[0];
    first?.focus();

    // Lock body scroll while any dialog is open
    const prevOverflow = document.body.style.overflow;
    document.body.style.overflow = "hidden";

    function handleKeyDown(e: KeyboardEvent) {
      // Only the top-most dialog handles keys
      if (dialogStack[dialogStack.length - 1] !== id) return;
      if (e.key === "Escape") {
        e.stopPropagation();
        onOpenChange(false);
        return;
      }
      if (e.key === "Tab") {
        // Focus trap: cycle within the dialog
        const items = focusables();
        if (items.length === 0) return;
        const firstItem = items[0];
        const lastItem = items[items.length - 1];
        const active = document.activeElement;
        if (e.shiftKey) {
          if (active === firstItem || !container?.contains(active)) {
            e.preventDefault();
            lastItem.focus();
          }
        } else {
          if (active === lastItem || !container?.contains(active)) {
            e.preventDefault();
            firstItem.focus();
          }
        }
      }
    }
    document.addEventListener("keydown", handleKeyDown);

    return () => {
      document.removeEventListener("keydown", handleKeyDown);
      const index = dialogStack.indexOf(id);
      if (index >= 0) dialogStack.splice(index, 1);
      if (dialogStack.length === 0) {
        document.body.style.overflow = prevOverflow;
      }
      previouslyFocused?.focus?.();
    };
  }, [open, onOpenChange]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50" ref={containerRef}>
      <div className="fixed inset-0 bg-black/50" aria-hidden="true" />
      <div
        className="fixed inset-0 flex items-center justify-center p-4"
        onClick={() => onOpenChange(false)}
      >
        {children}
      </div>
    </div>
  );
}

const DialogContent = React.forwardRef<
  HTMLDivElement,
  React.HTMLAttributes<HTMLDivElement> & { onClose?: () => void }
>(({ className, children, onClose, ...props }, ref) => (
  <div
    ref={ref}
    role="dialog"
    aria-modal="true"
    className={cn(
      "relative z-50 w-full max-w-lg rounded-lg border bg-background p-6 shadow-lg",
      className
    )}
    onClick={(e) => e.stopPropagation()}
    {...props}
  >
    {children}
    {onClose && (
      <button
        onClick={onClose}
        aria-label="Close dialog"
        className="absolute right-4 top-4 rounded-sm opacity-70 ring-offset-background transition-opacity hover:opacity-100"
      >
        <X className="h-4 w-4" aria-hidden="true" />
      </button>
    )}
  </div>
));
DialogContent.displayName = "DialogContent";

function DialogHeader({
  className,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        "flex flex-col space-y-1.5 text-center sm:text-left",
        className
      )}
      {...props}
    />
  );
}

function DialogTitle({
  className,
  ...props
}: React.HTMLAttributes<HTMLHeadingElement>) {
  return (
    <h2
      className={cn(
        "text-lg font-semibold leading-none tracking-tight",
        className
      )}
      {...props}
    />
  );
}

export { Dialog, DialogContent, DialogHeader, DialogTitle };
