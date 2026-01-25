"use client";

import { useEffect, useState } from "react";
import { Bell, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { useNotifications } from "@/hooks/use-notifications";

const NOTIFICATION_PROMPT_KEY = "notification-prompt-dismissed";

export function NotificationPrompt() {
  const { permission, requestPermission, isSupported } = useNotifications();
  const [showPrompt, setShowPrompt] = useState(false);
  const [isRequesting, setIsRequesting] = useState(false);

  useEffect(() => {
    // Only show if:
    // 1. Notifications are supported
    // 2. Permission is not yet decided (still "default")
    // 3. User hasn't dismissed the prompt before
    if (!isSupported) return;

    const dismissed = localStorage.getItem(NOTIFICATION_PROMPT_KEY);
    if (dismissed) return;

    if (permission === "default") {
      // Delay showing prompt to not overwhelm on first load
      const timer = setTimeout(() => {
        setShowPrompt(true);
      }, 2000);
      return () => clearTimeout(timer);
    }
  }, [isSupported, permission]);

  // Hide if permission changes (granted or denied)
  useEffect(() => {
    if (permission !== "default") {
      setShowPrompt(false);
    }
  }, [permission]);

  const handleEnable = async () => {
    setIsRequesting(true);
    await requestPermission();
    setIsRequesting(false);
    setShowPrompt(false);
  };

  const handleDismiss = () => {
    localStorage.setItem(NOTIFICATION_PROMPT_KEY, "true");
    setShowPrompt(false);
  };

  if (!showPrompt) return null;

  return (
    <div className="fixed bottom-4 right-4 z-50 max-w-sm animate-in slide-in-from-bottom-4 fade-in duration-300">
      <div className="rounded-lg border bg-background p-4 shadow-lg">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-primary/10">
            <Bell className="h-5 w-5 text-primary" />
          </div>
          <div className="flex-1">
            <h3 className="font-semibold">Enable notifications?</h3>
            <p className="mt-1 text-sm text-muted-foreground">
              Get notified when new emails arrive, even when you&apos;re not on this tab.
            </p>
            <div className="mt-3 flex gap-2">
              <Button size="sm" onClick={handleEnable} disabled={isRequesting}>
                {isRequesting ? "Enabling..." : "Enable"}
              </Button>
              <Button size="sm" variant="ghost" onClick={handleDismiss}>
                Not now
              </Button>
            </div>
          </div>
          <button
            onClick={handleDismiss}
            className="text-muted-foreground hover:text-foreground"
            aria-label="Dismiss"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>
    </div>
  );
}
