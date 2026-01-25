"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Loader2, Bell, Volume2 } from "lucide-react";
import { useNotifications } from "@/hooks/use-notifications";
import { useNotificationContext } from "@/contexts/notification-context";

interface NotificationPreferences {
  browser_notifications: boolean;
  notification_sound: boolean;
}

export default function SettingsPage() {
  const [preferences, setPreferences] = useState<NotificationPreferences | null>(null);
  const [loading, setLoading] = useState(true);
  const [browserToggleLoading, setBrowserToggleLoading] = useState(false);
  const [soundToggleLoading, setSoundToggleLoading] = useState(false);
  const [error, setError] = useState("");

  const { permission, requestPermission, isSupported } = useNotifications();
  const { setSoundEnabled } = useNotificationContext();

  const fetchPreferences = useCallback(async () => {
    try {
      const res = await fetch("/api/users/me/preferences");
      if (res.ok) {
        const data = await res.json();
        setPreferences(data.preferences);
        // Sync sound preference with notification context
        setSoundEnabled(data.preferences.notification_sound);
      }
    } catch {
      setError("Failed to load notification preferences");
    } finally {
      setLoading(false);
    }
  }, [setSoundEnabled]);

  useEffect(() => {
    fetchPreferences();
  }, [fetchPreferences]);

  async function handleBrowserNotificationsToggle() {
    if (!preferences) return;
    setBrowserToggleLoading(true);
    setError("");

    const newValue = !preferences.browser_notifications;

    // If enabling, request permission first
    if (newValue && permission !== "granted") {
      const granted = await requestPermission();
      if (!granted) {
        setBrowserToggleLoading(false);
        setError("Browser notification permission was denied. Please enable it in your browser settings.");
        return;
      }
    }

    try {
      const res = await fetch("/api/users/me/preferences", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ browser_notifications: newValue }),
      });

      if (res.ok) {
        const data = await res.json();
        setPreferences(data.preferences);
      } else {
        const data = await res.json();
        setError(data.error || "Failed to update preference");
      }
    } catch {
      setError("Failed to update preference");
    } finally {
      setBrowserToggleLoading(false);
    }
  }

  async function handleSoundToggle() {
    if (!preferences) return;
    setSoundToggleLoading(true);
    setError("");

    const newValue = !preferences.notification_sound;

    try {
      const res = await fetch("/api/users/me/preferences", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ notification_sound: newValue }),
      });

      if (res.ok) {
        const data = await res.json();
        setPreferences(data.preferences);
        // Sync with notification context
        setSoundEnabled(data.preferences.notification_sound);
      } else {
        const data = await res.json();
        setError(data.error || "Failed to update preference");
      }
    } catch {
      setError("Failed to update preference");
    } finally {
      setSoundToggleLoading(false);
    }
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold">Settings</h2>
        <p className="mt-1 text-muted-foreground">
          Manage your account settings and preferences.
        </p>
      </div>

      {error && (
        <div className="rounded-md bg-destructive/10 px-4 py-3 text-sm text-destructive">
          {error}
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Notifications</CardTitle>
          <CardDescription>
            Configure how you receive notifications for new emails.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-6">
          {/* Browser Notifications Toggle */}
          <div className="flex items-center justify-between">
            <div className="flex items-start gap-3">
              <Bell className="mt-0.5 h-5 w-5 text-muted-foreground" />
              <div className="space-y-1">
                <Label htmlFor="browser-notifications-toggle" className="text-sm font-medium">
                  Browser notifications
                </Label>
                <p className="text-sm text-muted-foreground">
                  {!isSupported
                    ? "Browser notifications are not supported in this browser"
                    : permission === "denied"
                    ? "Notifications are blocked. Please enable them in your browser settings."
                    : preferences?.browser_notifications
                    ? "Receive push notifications when new emails arrive"
                    : "Enable to receive push notifications for new emails"}
                </p>
              </div>
            </div>
            <button
              id="browser-notifications-toggle"
              type="button"
              role="switch"
              aria-checked={preferences?.browser_notifications ?? false}
              disabled={browserToggleLoading || !isSupported || permission === "denied"}
              onClick={handleBrowserNotificationsToggle}
              className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 ${
                preferences?.browser_notifications ? "bg-primary" : "bg-input"
              }`}
            >
              <span
                className={`pointer-events-none block h-5 w-5 rounded-full bg-background shadow-lg ring-0 transition-transform ${
                  preferences?.browser_notifications ? "translate-x-5" : "translate-x-0"
                }`}
              />
              {browserToggleLoading && (
                <Loader2 className="absolute left-1/2 top-1/2 h-3 w-3 -translate-x-1/2 -translate-y-1/2 animate-spin text-muted-foreground" />
              )}
            </button>
          </div>

          {/* Notification Sound Toggle */}
          <div className="flex items-center justify-between">
            <div className="flex items-start gap-3">
              <Volume2 className="mt-0.5 h-5 w-5 text-muted-foreground" />
              <div className="space-y-1">
                <Label htmlFor="notification-sound-toggle" className="text-sm font-medium">
                  Notification sound
                </Label>
                <p className="text-sm text-muted-foreground">
                  {preferences?.notification_sound
                    ? "Play a sound when new emails arrive"
                    : "Enable to play a sound notification for new emails"}
                </p>
              </div>
            </div>
            <button
              id="notification-sound-toggle"
              type="button"
              role="switch"
              aria-checked={preferences?.notification_sound ?? false}
              disabled={soundToggleLoading}
              onClick={handleSoundToggle}
              className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full border-2 border-transparent transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background disabled:cursor-not-allowed disabled:opacity-50 ${
                preferences?.notification_sound ? "bg-primary" : "bg-input"
              }`}
            >
              <span
                className={`pointer-events-none block h-5 w-5 rounded-full bg-background shadow-lg ring-0 transition-transform ${
                  preferences?.notification_sound ? "translate-x-5" : "translate-x-0"
                }`}
              />
              {soundToggleLoading && (
                <Loader2 className="absolute left-1/2 top-1/2 h-3 w-3 -translate-x-1/2 -translate-y-1/2 animate-spin text-muted-foreground" />
              )}
            </button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
