"use client";

import { Dialog, DialogContent } from "@/components/ui/dialog";

const SHORTCUTS = [
  { section: "Navigation", items: [
    { keys: ["j"], description: "Move focus down" },
    { keys: ["k"], description: "Move focus up" },
    { keys: ["Enter", "o"], description: "Open focused thread" },
    { keys: ["/", "\u2318K"], description: "Search" },
  ]},
  { section: "Actions", items: [
    { keys: ["\u2318N"], description: "Compose new email" },
    { keys: ["\u2318Enter"], description: "Send email" },
    { keys: ["x"], description: "Toggle select" },
    { keys: ["s"], description: "Star/unstar thread" },
    { keys: ["e"], description: "Archive" },
    { keys: ["#"], description: "Move to trash" },
    { keys: ["m"], description: "Mute/unmute thread" },
    { keys: ["r"], description: "Refresh" },
    { keys: ["Shift+I"], description: "Mark as read" },
    { keys: ["Shift+U"], description: "Mark as unread" },
  ]},
  { section: "Domains", items: [
    { keys: ["\u23181-9"], description: "Switch domain" },
  ]},
  { section: "Other", items: [
    { keys: ["?"], description: "Show keyboard shortcuts" },
  ]},
];

interface KeyboardShortcutsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function KeyboardShortcutsDialog({ open, onOpenChange }: KeyboardShortcutsDialogProps) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-md" onClose={() => onOpenChange(false)}>
        <h2 className="text-lg font-semibold mb-4">Keyboard Shortcuts</h2>
        <div className="space-y-5 max-h-[60vh] overflow-y-auto">
          {SHORTCUTS.map((section) => (
            <div key={section.section}>
              <h3 className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-2">
                {section.section}
              </h3>
              <div className="space-y-1.5">
                {section.items.map((item) => (
                  <div key={item.description} className="flex items-center justify-between text-sm">
                    <span>{item.description}</span>
                    <div className="flex items-center gap-1">
                      {item.keys.map((key) => (
                        <kbd
                          key={key}
                          className="px-1.5 py-0.5 bg-muted border rounded text-xs font-mono"
                        >
                          {key}
                        </kbd>
                      ))}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          ))}
        </div>
      </DialogContent>
    </Dialog>
  );
}
