"use client";

import { useEffect, useCallback, useState } from "react";
import { toast } from "sonner";
import { useNotifications } from "@/contexts/notification-context";
import { api } from "@/lib/api";
import { Bell, X } from "lucide-react";
import type { WSMessage } from "@/lib/types";

const PROMPT_DISMISSED_KEY = "notification_prompt_dismissed";

// threadUrl builds a deep link to the thread a notification refers to.
// The inbox page reads ?thread= from the URL and opens the reading pane.
function threadUrl(msg: WSMessage): string | null {
  if (!msg.domain_id) return null;
  const base = `/d/${msg.domain_id}/inbox`;
  return msg.thread_id ? `${base}?thread=${msg.thread_id}` : base;
}

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

  const handleEmailReceived = useCallback(
    (msg: WSMessage) => {
      const payload = msg.payload as {
        email_id?: string;
        from?: string;
        subject?: string;
      } | undefined;

      // Skip empty notifications (e.g. during bulk sync — no from/subject in payload)
      if (!payload?.from && !payload?.subject) return;

      const from = payload?.from ?? "";
      const subject = payload?.subject ?? "";
      const url = threadUrl(msg);

      // Desktop notification (visible when tab is backgrounded).
      // Clicking it focuses the tab and opens the thread.
      if (typeof Notification !== "undefined" && Notification.permission === "granted") {
        const notification = new Notification(from || "New email", {
          body: subject,
          tag: payload?.email_id ? `email-${payload.email_id}` : `email-${Date.now()}`,
        });
        notification.onclick = () => {
          window.focus();
          if (url) window.location.href = url;
          notification.close();
        };
      }

      // In-app toast — the one sonner stack, same as the rest of the app
      toast(from || "New email", {
        id: payload?.email_id ? `email-received-${payload.email_id}` : undefined,
        description: subject,
        duration: 5000,
        action: url
          ? {
              label: "Open",
              onClick: () => {
                window.location.href = url;
              },
            }
          : undefined,
      });
    },
    []
  );

  useEffect(() => {
    const unsub = subscribe("email.received", handleEmailReceived);
    return unsub;
  }, [subscribe, handleEmailReceived]);

  useEffect(() => {
    const unsub = subscribe("domain.not_found", (msg: WSMessage) => {
      const payload = msg.payload as { domain?: string; domains?: string[] } | undefined;
      const domain = payload?.domain || payload?.domains?.[0] || "unknown";
      toast.warning(`Email skipped - domain "${domain}" not found. Check Settings > Domains.`, {
        id: `domain-not-found-${domain}`,
        duration: 8000,
      });
    });
    return unsub;
  }, [subscribe]);

  useEffect(() => {
    const unsub = subscribe("domain.discovered", (msg: WSMessage) => {
      const payload = msg.payload as { domain?: string } | undefined;
      const domain = payload?.domain || "unknown";
      toast.info(`New domain found in Resend: ${domain}. Add it in Settings → Domains.`, {
        id: `domain-discovered-${domain}`,
        duration: 10000,
      });
    });
    return unsub;
  }, [subscribe]);

  return <NotificationPrompt />;
}
