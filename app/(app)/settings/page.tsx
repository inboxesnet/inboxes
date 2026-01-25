"use client";

import { useState, useEffect, useCallback } from "react";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { Input } from "@/components/ui/input";
import { Button } from "@/components/ui/button";
import { Loader2, Bell, Volume2, User, Lock, Check } from "lucide-react";
import { useNotifications } from "@/hooks/use-notifications";
import { useNotificationContext } from "@/contexts/notification-context";
import { useToast } from "@/components/ui/toast";

interface NotificationPreferences {
  browser_notifications: boolean;
  notification_sound: boolean;
}

interface UserProfile {
  id: string;
  name: string;
  email: string;
  role: string;
}

export default function SettingsPage() {
  // Profile state
  const [profile, setProfile] = useState<UserProfile | null>(null);
  const [editName, setEditName] = useState("");
  const [nameLoading, setNameLoading] = useState(false);

  // Password state
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordLoading, setPasswordLoading] = useState(false);
  const [passwordError, setPasswordError] = useState("");

  // Notification preferences state
  const [preferences, setPreferences] = useState<NotificationPreferences | null>(null);
  const [loading, setLoading] = useState(true);
  const [browserToggleLoading, setBrowserToggleLoading] = useState(false);
  const [soundToggleLoading, setSoundToggleLoading] = useState(false);
  const [error, setError] = useState("");

  const { permission, requestPermission, isSupported } = useNotifications();
  const { setSoundEnabled } = useNotificationContext();
  const { addToast } = useToast();

  const fetchData = useCallback(async () => {
    try {
      const [profileRes, prefsRes] = await Promise.all([
        fetch("/api/users/me"),
        fetch("/api/users/me/preferences"),
      ]);

      if (profileRes.ok) {
        const data = await profileRes.json();
        setProfile(data.user);
        setEditName(data.user.name);
      }

      if (prefsRes.ok) {
        const data = await prefsRes.json();
        setPreferences(data.preferences);
        setSoundEnabled(data.preferences.notification_sound);
      }
    } catch {
      setError("Failed to load settings");
    } finally {
      setLoading(false);
    }
  }, [setSoundEnabled]);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  async function handleNameSave() {
    if (!profile || editName.trim() === profile.name) return;
    setNameLoading(true);
    setError("");

    try {
      const res = await fetch("/api/users/me", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ name: editName.trim() }),
      });

      if (res.ok) {
        const data = await res.json();
        setProfile(data.user);
        addToast("Name updated successfully", "default");
      } else {
        const data = await res.json();
        setError(data.error || "Failed to update name");
      }
    } catch {
      setError("Failed to update name");
    } finally {
      setNameLoading(false);
    }
  }

  async function handlePasswordChange(e: React.FormEvent) {
    e.preventDefault();
    setPasswordError("");

    // Validate
    if (!currentPassword) {
      setPasswordError("Current password is required");
      return;
    }
    if (!newPassword) {
      setPasswordError("New password is required");
      return;
    }
    if (newPassword.length < 8) {
      setPasswordError("New password must be at least 8 characters");
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordError("New passwords do not match");
      return;
    }

    setPasswordLoading(true);

    try {
      const res = await fetch("/api/users/me/password", {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          current_password: currentPassword,
          new_password: newPassword,
        }),
      });

      if (res.ok) {
        addToast("Password updated successfully", "default");
        setCurrentPassword("");
        setNewPassword("");
        setConfirmPassword("");
      } else {
        const data = await res.json();
        setPasswordError(data.error || "Failed to update password");
      }
    } catch {
      setPasswordError("Failed to update password");
    } finally {
      setPasswordLoading(false);
    }
  }

  async function handleBrowserNotificationsToggle() {
    if (!preferences) return;
    setBrowserToggleLoading(true);
    setError("");

    const newValue = !preferences.browser_notifications;

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

  const hasNameChanges = profile && editName.trim() !== profile.name;

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

      {/* Profile Section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <User className="h-5 w-5" />
            Profile
          </CardTitle>
          <CardDescription>
            Your personal information and account details.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {/* Name (editable) */}
          <div className="space-y-2">
            <Label htmlFor="name">Name</Label>
            <div className="flex gap-2">
              <Input
                id="name"
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
                placeholder="Your name"
                className="max-w-sm"
              />
              <Button
                onClick={handleNameSave}
                disabled={nameLoading || !hasNameChanges}
                size="sm"
              >
                {nameLoading ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : (
                  <>
                    <Check className="mr-1 h-4 w-4" />
                    Save
                  </>
                )}
              </Button>
            </div>
          </div>

          {/* Email (read-only) */}
          <div className="space-y-2">
            <Label htmlFor="email">Email</Label>
            <Input
              id="email"
              value={profile?.email || ""}
              disabled
              className="max-w-sm bg-muted"
            />
            <p className="text-sm text-muted-foreground">
              Your email address cannot be changed.
            </p>
          </div>

          {/* Role (read-only) */}
          <div className="space-y-2">
            <Label htmlFor="role">Role</Label>
            <Input
              id="role"
              value={profile?.role === "admin" ? "Admin" : "Member"}
              disabled
              className="max-w-sm bg-muted"
            />
            <p className="text-sm text-muted-foreground">
              Your role is managed by your organization admin.
            </p>
          </div>
        </CardContent>
      </Card>

      {/* Password Section */}
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <Lock className="h-5 w-5" />
            Change Password
          </CardTitle>
          <CardDescription>
            Update your password to keep your account secure.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handlePasswordChange} className="space-y-4 max-w-sm">
            {passwordError && (
              <div className="rounded-md bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {passwordError}
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="current-password">Current Password</Label>
              <Input
                id="current-password"
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                placeholder="Enter current password"
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="new-password">New Password</Label>
              <Input
                id="new-password"
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                placeholder="Enter new password"
              />
              <p className="text-sm text-muted-foreground">
                Must be at least 8 characters.
              </p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="confirm-password">Confirm New Password</Label>
              <Input
                id="confirm-password"
                type="password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                placeholder="Confirm new password"
              />
            </div>

            <Button type="submit" disabled={passwordLoading}>
              {passwordLoading ? (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              ) : null}
              Update Password
            </Button>
          </form>
        </CardContent>
      </Card>

      {/* Notifications Section */}
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
