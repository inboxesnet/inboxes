"use client";

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";

const CHANNEL_NAME = "inboxes-cache-sync";

/**
 * Listens for cross-tab cache invalidation messages via BroadcastChannel.
 * Call postInvalidation() after mutations to notify other tabs.
 */
export function useBroadcastSync() {
  const qc = useQueryClient();

  useEffect(() => {
    if (typeof BroadcastChannel === "undefined") return;

    const channel = new BroadcastChannel(CHANNEL_NAME);
    channel.onmessage = (event) => {
      const keys = event.data?.keys;
      if (Array.isArray(keys)) {
        for (const key of keys) {
          qc.invalidateQueries({ queryKey: key });
        }
      }
    };

    return () => channel.close();
  }, [qc]);
}

/** Post invalidation keys to other tabs */
export function postInvalidation(keys: readonly (readonly unknown[])[]) {
  if (typeof BroadcastChannel === "undefined") return;
  try {
    const channel = new BroadcastChannel(CHANNEL_NAME);
    channel.postMessage({ keys });
    channel.close();
  } catch {
    // BroadcastChannel not supported
  }
}
