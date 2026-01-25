"use client";

import { useEffect, useState } from "react";
import { useWebSocket } from "@/hooks/use-websocket";
import { WifiOff, Wifi, Loader2 } from "lucide-react";

export function ConnectionStatus() {
  const { isConnected } = useWebSocket();
  const [isOnline, setIsOnline] = useState(true);
  const [showReconnecting, setShowReconnecting] = useState(false);

  // Track browser online/offline status
  useEffect(() => {
    setIsOnline(navigator.onLine);

    function handleOnline() {
      setIsOnline(true);
    }

    function handleOffline() {
      setIsOnline(false);
    }

    window.addEventListener("online", handleOnline);
    window.addEventListener("offline", handleOffline);

    return () => {
      window.removeEventListener("online", handleOnline);
      window.removeEventListener("offline", handleOffline);
    };
  }, []);

  // Show reconnecting state when WebSocket disconnects
  useEffect(() => {
    if (!isConnected && isOnline) {
      // Delay showing reconnecting message to avoid flashing during quick reconnects
      const timer = setTimeout(() => {
        setShowReconnecting(true);
      }, 3000);
      return () => clearTimeout(timer);
    } else {
      setShowReconnecting(false);
    }
  }, [isConnected, isOnline]);

  // Don't show anything if everything is fine
  if (isOnline && isConnected) {
    return null;
  }

  // Show offline banner
  if (!isOnline) {
    return (
      <div className="fixed bottom-0 left-0 right-0 z-50 flex items-center justify-center gap-2 bg-destructive px-4 py-2 text-sm text-destructive-foreground">
        <WifiOff className="h-4 w-4" />
        <span>You are offline. Check your internet connection.</span>
      </div>
    );
  }

  // Show reconnecting banner (only after delay)
  if (showReconnecting) {
    return (
      <div className="fixed bottom-0 left-0 right-0 z-50 flex items-center justify-center gap-2 bg-amber-500 px-4 py-2 text-sm text-white">
        <Loader2 className="h-4 w-4 animate-spin" />
        <span>Reconnecting to server...</span>
      </div>
    );
  }

  return null;
}
