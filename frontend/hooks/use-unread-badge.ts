"use client";

import { useEffect, useRef } from "react";
import { useDomains } from "@/contexts/domain-context";

const BASE_TITLE = "Inboxes.net";
const BADGE_SIZE = 5;
const BADGE_COLOR = "#ef4444";

/**
 * Updates the browser tab title and favicon badge based on unread counts.
 * - Tab title: "Inbox (3) - Inboxes.net" (current domain's unread count)
 * - Favicon: red dot on bottom-right when any domain has unreads
 */
export function useUnreadBadge(pageTitle: string, domainId?: string) {
  const { unreadCounts, domains } = useDomains();
  const domainName = domains.find((d) => d.id === domainId)?.domain;
  const baseFaviconRef = useRef<HTMLImageElement | null>(null);
  const canvasRef = useRef<HTMLCanvasElement | null>(null);

  const domainUnread = domainId ? (unreadCounts[domainId] || 0) : 0;
  const totalUnread = Object.values(unreadCounts).reduce((sum, n) => sum + n, 0);

  // Update tab title: "Inbox (3) - cx.agency - Inboxes.net"
  useEffect(() => {
    const label = domainUnread > 0 ? `${pageTitle} (${domainUnread})` : pageTitle;
    const parts = domainName ? [label, domainName, BASE_TITLE] : [label, BASE_TITLE];
    document.title = parts.join(" - ");
  }, [pageTitle, domainUnread, domainName]);

  // Update favicon badge based on global unread count
  useEffect(() => {
    const linkEl = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (!linkEl) return;

    // Load the base favicon once
    if (!baseFaviconRef.current) {
      const img = new Image();
      img.crossOrigin = "anonymous";
      img.src = linkEl.href;
      baseFaviconRef.current = img;
    }

    if (!canvasRef.current) {
      canvasRef.current = document.createElement("canvas");
    }

    const img = baseFaviconRef.current;
    const canvas = canvasRef.current;

    function draw() {
      const size = 32;
      canvas.width = size;
      canvas.height = size;
      const ctx = canvas.getContext("2d");
      if (!ctx) return;

      ctx.clearRect(0, 0, size, size);
      ctx.drawImage(img, 0, 0, size, size);

      if (totalUnread > 0) {
        // Red circle, bottom-right
        ctx.beginPath();
        ctx.arc(size - BADGE_SIZE, size - BADGE_SIZE, BADGE_SIZE, 0, 2 * Math.PI);
        ctx.fillStyle = BADGE_COLOR;
        ctx.fill();
      }

      linkEl!.href = canvas.toDataURL("image/png");
    }

    if (img.complete) {
      draw();
    } else {
      img.onload = draw;
    }
  }, [totalUnread]);
}
