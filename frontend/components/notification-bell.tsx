"use client";

import { useEffect, useRef, useState } from "react";
import { AlertTriangle, Bell, CheckCircle2, XCircle } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import { formatRelativeTime } from "@/lib/utils";
import { useNotifications } from "@/contexts/notification-context";

interface NotificationEvent {
  id: number;
  event: string;
  created_at: string;
  read: boolean;
  domain_id?: string;
  thread_id?: string;
  payload?: Record<string, unknown>;
}

interface NotificationsResponse {
  events: NotificationEvent[];
  unread_count: number;
  read_cursor: number;
}

// The bell shows persistent warnings; toasts cover transient events.
const WARNING_EVENTS = new Set([
  "domain.disconnected",
  "domain.reconnected",
  "domain.dns_degraded",
  "domain.not_found",
  "email.status_updated",
  "plan.changed",
]);

function describeEvent(evt: NotificationEvent): string {
  const p = evt.payload || {};
  const domain = (p.domain as string) || "";
  switch (evt.event) {
    case "domain.disconnected":
      return p.reason === "api_key_revoked"
        ? "API key revoked — all domains disconnected."
        : `Domain ${domain} disconnected from Resend.`;
    case "domain.reconnected":
      return `Domain ${domain} reconnected.`;
    case "domain.dns_degraded": {
      const degraded = (p.degraded as string[]) || [];
      return `DNS records changed for ${domain}: ${degraded.join(", ") || "verification failed"}.`;
    }
    case "domain.not_found": {
      const domains = (p.domains as string[]) || [];
      const name = domain || domains.join(", ");
      return `Domain ${name} not found in Resend.`;
    }
    case "email.status_updated": {
      const subject = (p.subject as string) || "an email";
      const status = p.status as string;
      return status === "failed"
        ? `Failed to send: ${subject}`
        : `Bounced: ${subject}`;
    }
    case "plan.changed":
      return p.plan === "past_due"
        ? "Payment failed. Update your payment method."
        : "Your subscription was cancelled.";
    default:
      return evt.event;
  }
}

function eventIcon(evt: NotificationEvent) {
  if (evt.event === "domain.reconnected") {
    return <CheckCircle2 className="h-3.5 w-3.5 shrink-0 text-green-500" />;
  }
  if (evt.event === "email.status_updated") {
    return <XCircle className="h-3.5 w-3.5 shrink-0 text-destructive" />;
  }
  return <AlertTriangle className="h-3.5 w-3.5 shrink-0 text-yellow-500" />;
}

export function NotificationBell() {
  const [open, setOpen] = useState(false);
  // Snapshot of the ids that were unread when the panel opened, so the
  // highlight survives the immediate mark-all-read.
  const [freshIds, setFreshIds] = useState<Set<number>>(new Set());
  const panelRef = useRef<HTMLDivElement>(null);
  const qc = useQueryClient();
  const { subscribe } = useNotifications();

  const { data } = useQuery({
    queryKey: queryKeys.notifications.list(),
    queryFn: () => api.get<NotificationsResponse>("/api/notifications"),
    staleTime: 60_000,
  });

  // A live warning event refreshes the list and the badge.
  useEffect(() => {
    return subscribe("*", (msg) => {
      if (WARNING_EVENTS.has(msg.event)) {
        qc.invalidateQueries({ queryKey: queryKeys.notifications.all });
      }
    });
  }, [subscribe, qc]);

  useEffect(() => {
    if (!open) return;
    function onMouseDown(e: MouseEvent) {
      if (!panelRef.current?.contains(e.target as Node)) setOpen(false);
    }
    function onKeyDown(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onMouseDown);
    document.addEventListener("keydown", onKeyDown);
    return () => {
      document.removeEventListener("mousedown", onMouseDown);
      document.removeEventListener("keydown", onKeyDown);
    };
  }, [open]);

  const events = data?.events ?? [];
  const unread = data?.unread_count ?? 0;

  async function toggleOpen() {
    if (open) {
      setOpen(false);
      return;
    }
    setOpen(true);
    setFreshIds(new Set(events.filter((e) => !e.read).map((e) => e.id)));
    // Opening the panel always marks all read.
    if (events.length > 0 && unread > 0) {
      qc.setQueryData<NotificationsResponse>(
        queryKeys.notifications.list(),
        (old) =>
          old && {
            ...old,
            unread_count: 0,
            read_cursor: events[0].id,
            events: old.events.map((e) => ({ ...e, read: true })),
          }
      );
      try {
        await api.post("/api/notifications/read", { last_id: events[0].id });
      } catch {
        qc.invalidateQueries({ queryKey: queryKeys.notifications.all });
      }
    }
  }

  return (
    <div className="relative" ref={panelRef}>
      <button
        onClick={toggleOpen}
        className="relative p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
        title="Notifications"
        aria-label={unread > 0 ? `Notifications (${unread} unread)` : "Notifications"}
      >
        <Bell className="h-4 w-4" />
        {unread > 0 && (
          <span className="absolute top-0.5 right-0.5 min-w-[16px] h-4 px-1 rounded-full bg-destructive text-destructive-foreground text-[10px] leading-4 text-center font-medium">
            {unread > 9 ? "9+" : unread}
          </span>
        )}
      </button>
      {open && (
        <div className="absolute top-full right-0 mt-1 z-50 bg-popover border rounded-md shadow-md w-80 max-h-96 overflow-y-auto">
          <div className="px-3 py-2 border-b text-xs font-medium text-muted-foreground">
            Notifications
          </div>
          {events.length === 0 ? (
            <p className="px-3 py-6 text-xs text-muted-foreground text-center">
              No notifications from the last 30 days.
            </p>
          ) : (
            <ul>
              {events.map((evt) => (
                <li
                  key={evt.id}
                  className={`flex items-start gap-2 px-3 py-2 text-xs border-b last:border-b-0 ${
                    freshIds.has(evt.id) ? "bg-muted/50" : ""
                  }`}
                >
                  <span className="mt-0.5">{eventIcon(evt)}</span>
                  <span className="flex-1 leading-snug">{describeEvent(evt)}</span>
                  <span className="text-muted-foreground shrink-0">
                    {formatRelativeTime(evt.created_at)}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </div>
  );
}
