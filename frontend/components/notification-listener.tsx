"use client";

import { useEffect, useCallback, useState } from "react";
import { useNotifications } from "@/contexts/notification-context";
import type { WSMessage } from "@/lib/types";

interface ToastNotification {
  id: string;
  from: string;
  subject: string;
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

      const id = Date.now().toString();
      setToasts((prev) => [
        ...prev,
        { id, from: payload?.from ?? "", subject: payload?.subject ?? "" },
      ]);

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

  if (toasts.length === 0) return null;

  return (
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
  );
}
