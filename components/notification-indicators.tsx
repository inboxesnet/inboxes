"use client";

import { useEffect, useRef } from "react";
import { useNotificationContext } from "@/contexts/notification-context";

export function NotificationIndicators() {
  const { unreadCount } = useNotificationContext();
  const originalFaviconRef = useRef<string | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  // Update document title with unread count
  useEffect(() => {
    const baseTitle = "Inbox — Inboxes.net";
    if (unreadCount > 0) {
      document.title = `(${unreadCount}) ${baseTitle}`;
    } else {
      document.title = baseTitle;
    }
  }, [unreadCount]);

  // Update favicon with unread indicator dot
  useEffect(() => {
    // Find the existing favicon link element
    let faviconLink = document.querySelector(
      'link[rel="icon"]'
    ) as HTMLLinkElement | null;

    // Store original favicon URL on first run
    if (originalFaviconRef.current === null) {
      originalFaviconRef.current = faviconLink?.href || "/favicon.ico";
    }

    // Create canvas for generating favicon with dot
    if (!canvasRef.current) {
      canvasRef.current = document.createElement("canvas");
      canvasRef.current.width = 32;
      canvasRef.current.height = 32;
    }

    const canvas = canvasRef.current;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;

    // Load the original favicon image
    const img = new Image();
    img.crossOrigin = "anonymous";
    img.onload = () => {
      // Clear canvas
      ctx.clearRect(0, 0, 32, 32);

      // Draw the original favicon
      ctx.drawImage(img, 0, 0, 32, 32);

      // If there are unread messages, draw a red dot
      if (unreadCount > 0) {
        // Draw red dot in top-right corner
        ctx.fillStyle = "#ef4444"; // red-500
        ctx.beginPath();
        ctx.arc(24, 8, 8, 0, 2 * Math.PI);
        ctx.fill();

        // Draw white border for contrast
        ctx.strokeStyle = "#ffffff";
        ctx.lineWidth = 2;
        ctx.beginPath();
        ctx.arc(24, 8, 8, 0, 2 * Math.PI);
        ctx.stroke();
      }

      // Update favicon
      const newFaviconUrl = canvas.toDataURL("image/png");

      if (!faviconLink) {
        faviconLink = document.createElement("link");
        faviconLink.rel = "icon";
        document.head.appendChild(faviconLink);
      }

      faviconLink.href = newFaviconUrl;
    };

    img.onerror = () => {
      // If loading fails, just update the title (favicon stays unchanged)
    };

    img.src = originalFaviconRef.current;

    // Cleanup: restore original favicon when component unmounts
    return () => {
      if (faviconLink && originalFaviconRef.current) {
        faviconLink.href = originalFaviconRef.current;
      }
    };
  }, [unreadCount]);

  return null;
}
