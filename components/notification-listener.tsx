"use client";

import { useEffect } from "react";
import { useWebSocket } from "@/hooks/use-websocket";
import { useNotifications } from "@/hooks/use-notifications";

interface NewEmailPayload {
  threadId: string;
  emailId: string;
  subject: string;
  from: string;
  preview: string;
}

function extractName(address: string): string {
  const match = address.match(/^"?([^"<]+)"?\s*</);
  if (match) return match[1].trim();
  return address.split("@")[0];
}

export function NotificationListener() {
  const { subscribe } = useWebSocket();
  const { showNotification, permission } = useNotifications();

  useEffect(() => {
    if (permission !== "granted") return;

    const unsubscribe = subscribe((event) => {
      if (event.event === "new_email") {
        const payload = event.payload as NewEmailPayload;
        const senderName = extractName(payload.from);

        showNotification(`New email from ${senderName}`, {
          body: payload.subject + (payload.preview ? `\n${payload.preview}` : ""),
          threadId: payload.threadId,
        });
      }
    });

    return unsubscribe;
  }, [subscribe, showNotification, permission]);

  return null;
}
