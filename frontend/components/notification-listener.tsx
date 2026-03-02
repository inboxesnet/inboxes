"use client";

import { useEffect, useCallback, useState } from "react";
import { toast } from "sonner";
import { useNotifications } from "@/contexts/notification-context";
import { api } from "@/lib/api";
import { Bell, X } from "lucide-react";
import type { WSMessage } from "@/lib/types";

const MAX_TOASTS = 3;

interface ToastNotification {
  id: string;
  from: string;
  subject: string;
}

const PROMPT_DISMISSED_KEY = "notification_prompt_dismissed";

function NotificationPrompt() {
  const [show, setShow] = useState(false);

  useEffect(() => {
    if (typeof window === "undefined" || !("Notification" in window)) return;
    if (Notification.permission !== "default") return;
    if (localStorage.getItem(PROMPT_DISMISSED_KEY)) return;
    setShow(true);
  }, []);

  if (!show) return null;

  function handleEnable() {
    Notification.requestPermission().then((result) => {
      setShow(false);
      localStorage.setItem(PROMPT_DISMISSED_KEY, "1");
      if (result === "granted") {
        api.patch("/api/users/me/preferences", { desktop_notifications: true }).catch(() => {});
      } else if (result === "denied") {
        toast("Notifications blocked - if you're in an incognito window, try a regular browser window. Otherwise, check your browser's site settings.", { duration: 6000 });
      }
    });
  }

  function handleDismiss() {
    setShow(false);
    localStorage.setItem(PROMPT_DISMISSED_KEY, "1");
  }

  return (
    <div className="fixed bottom-4 left-4 z-50 max-w-sm bg-card border shadow-lg rounded-lg p-3 animate-in slide-in-from-left">
      <div className="flex items-start gap-3">
        <div className="h-8 w-8 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
          <Bell className="h-4 w-4 text-primary" />
        </div>
        <div className="flex-1 min-w-0">
          <p className="text-sm font-medium">Enable notifications?</p>
          <p className="text-xs text-muted-foreground mt-0.5">
            Get notified when new emails arrive.
          </p>
          <div className="flex items-center gap-2 mt-2">
            <button
              onClick={handleEnable}
              className="text-xs font-medium bg-primary text-primary-foreground px-3 py-1 rounded hover:bg-primary/90 transition-colors"
            >
              Enable
            </button>
            <button
              onClick={handleDismiss}
              className="text-xs text-muted-foreground hover:text-foreground transition-colors"
            >
              Not now
            </button>
          </div>
        </div>
        <button
          onClick={handleDismiss}
          className="text-muted-foreground hover:text-foreground p-0.5"
          aria-label="Dismiss notification prompt"
        >
          <X className="h-3.5 w-3.5" />
        </button>
      </div>
    </div>
  );
}

export function NotificationListener() {
  const { subscribe } = useNotifications();
  const [toasts, setToasts] = useState<ToastNotification[]>([]);

  const handleEmailReceived = useCallback(
    (msg: WSMessage) => {
      const payload = msg.payload as {
        from?: string;
        subject?: string;
      } | undefined;

      // Skip empty notifications (e.g. during bulk sync — no from/subject in payload)
      if (!payload?.from && !payload?.subject) return;

      const from = payload?.from ?? "";
      const subject = payload?.subject ?? "";

      // Desktop notification (visible when tab is backgrounded)
      if (typeof Notification !== "undefined" && Notification.permission === "granted") {
        new Notification(from || "New email", {
          body: subject,
          tag: `email-${Date.now()}`,
        });
      }

      // In-app toast (cap at MAX_TOASTS)
      const id = Date.now().toString();
      setToasts((prev) => {
        const next = [...prev, { id, from, subject }];
        return next.length > MAX_TOASTS ? next.slice(-MAX_TOASTS) : next;
      });

      // Auto-dismiss after 5s
      setTimeout(() => {
        setToasts((prev) => prev.filter((t) => t.id !== id));
      }, 5000);
    },
    []
  );

  useEffect(() => {
    const unsub = subscribe("email.received", handleEmailReceived);
    return unsub;
  }, [subscribe, handleEmailReceived]);

  return (
    <>
      <NotificationPrompt />
      {toasts.length > 0 && (
        <div className="fixed bottom-4 right-4 z-50 space-y-2">
          {toasts.map((toast) => (
            <div
              key={toast.id}
              className="bg-card border shadow-lg rounded-lg p-3 max-w-sm animate-in slide-in-from-right"
            >
              <p className="text-sm font-medium truncate">{toast.from}</p>
              <p className="text-xs text-muted-foreground truncate">
                {toast.subject}
              </p>
            </div>
          ))}
        </div>
      )}
    </>
  );
}
