"use client";

import { useState, useEffect } from "react";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { Badge } from "@/components/ui/badge";
import { useDomains } from "@/contexts/domain-context";
import type { User, Domain } from "@/lib/types";
import { Check, Minus, RefreshCw } from "lucide-react";

export default function SettingsPage() {
  const { refreshDomains } = useDomains();
  const [user, setUser] = useState<User | null>(null);
  const [name, setName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const [syncing, setSyncing] = useState(false);

  // Domain management state
  const [allDomains, setAllDomains] = useState<Domain[]>([]);
  const [visibleIds, setVisibleIds] = useState<Set<string>>(new Set());
  const [loadingDomains, setLoadingDomains] = useState(false);
  const [savingDomains, setSavingDomains] = useState(false);
  const [refreshingDomains, setRefreshingDomains] = useState(false);
  const [syncProgress, setSyncProgress] = useState<{
    phase: string;
    imported: number;
    total: number;
    message: string;
  } | null>(null);
  const [syncResult, setSyncResult] = useState<{
    sent_count: number;
    received_count: number;
    thread_count: number;
    address_count: number;
  } | null>(null);

  useEffect(() => {
    async function load() {
      try {
        const [userData, domainData] = await Promise.all([
          api.get<User>("/api/users/me"),
          api.get<Domain[]>("/api/domains/all"),
        ]);
        setUser(userData);
        setName(userData.name);
        setAllDomains(domainData);
        setVisibleIds(new Set(domainData.filter((d) => !d.hidden).map((d) => d.id)));
      } catch {
        // handled
      } finally {
        setLoading(false);
      }
    }
    load();
  }, []);

  async function handleUpdateProfile(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSuccess("");
    setSaving(true);
    try {
      await api.patch("/api/users/me", { name });
      setSuccess("Profile updated");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update");
    } finally {
      setSaving(false);
    }
  }

  async function handleUpdatePassword(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSuccess("");
    setSavingPassword(true);
    try {
      await api.patch("/api/users/me/password", {
        current_password: currentPassword,
        new_password: newPassword,
      });
      setCurrentPassword("");
      setNewPassword("");
      setSuccess("Password updated");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update");
    } finally {
      setSavingPassword(false);
    }
  }

  async function handleRefreshDomains() {
    setError("");
    setRefreshingDomains(true);
    try {
      const data = await api.post<Domain[]>("/api/domains/sync");
      setAllDomains(data);
      setVisibleIds(new Set(data.filter((d) => !d.hidden).map((d) => d.id)));
      setSuccess("Domains refreshed from Resend");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to refresh domains");
    } finally {
      setRefreshingDomains(false);
    }
  }

  async function handleSaveDomainVisibility() {
    setError("");
    setSavingDomains(true);
    try {
      await api.patch("/api/domains/visibility", {
        visible: Array.from(visibleIds),
      });
      await refreshDomains();
      setSuccess("Domain visibility updated");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save");
    } finally {
      setSavingDomains(false);
    }
  }

  function handleSync() {
    setError("");
    setSyncing(true);
    setSyncProgress(null);
    setSyncResult(null);

    const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
    const source = new EventSource(`${API_URL}/api/orgs/sync-stream`, {
      withCredentials: true,
    });

    source.addEventListener("progress", (e) => {
      const data = JSON.parse((e as MessageEvent).data);
      setSyncProgress(data);
    });

    source.addEventListener("done", (e) => {
      const data = JSON.parse((e as MessageEvent).data);
      setSyncResult(data);
      source.close();
      setSyncing(false);
      setSuccess("Sync completed");
    });

    source.addEventListener("error", (e) => {
      try {
        const data = JSON.parse((e as MessageEvent).data);
        setError(data.error || "Sync failed");
      } catch {
        setError("Connection lost during sync");
      }
      source.close();
      setSyncing(false);
    });

    source.onerror = () => {
      if (source.readyState === EventSource.CLOSED) return;
      source.close();
      if (!syncResult) {
        setError("Connection lost during sync");
        setSyncing(false);
      }
    };
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <Spinner className="h-8 w-8" />
      </div>
    );
  }

  return (
    <div className="max-w-2xl mx-auto p-6 space-y-6">
      <h1 className="text-2xl font-semibold">Settings</h1>

      {error && (
        <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
          {error}
        </div>
      )}
      {success && (
        <div className="text-sm text-green-700 bg-green-50 p-3 rounded-md">
          {success}
        </div>
      )}

      {/* Profile */}
      <Card>
        <CardHeader>
          <CardTitle>Profile</CardTitle>
          <CardDescription>Update your personal information</CardDescription>
        </CardHeader>
        <form onSubmit={handleUpdateProfile}>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Email</label>
              <Input value={user?.email || ""} disabled />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">Name</label>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
            </div>
          </CardContent>
          <CardFooter>
            <Button disabled={saving}>
              {saving ? <Spinner className="mr-2" /> : null}
              Save
            </Button>
          </CardFooter>
        </form>
      </Card>

      {/* Domains */}
      <Card>
        <CardHeader>
          <CardTitle>Domains</CardTitle>
          <CardDescription>
            Choose which domains appear in the sidebar. Refresh to pull new domains from Resend.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-3">
          <div className="flex items-center justify-between">
            <button
              type="button"
              onClick={() => {
                if (user?.role !== "admin") return;
                if (visibleIds.size > 0) {
                  setVisibleIds(new Set());
                } else {
                  setVisibleIds(new Set(allDomains.map((d) => d.id)));
                }
              }}
              disabled={user?.role !== "admin"}
              className="flex items-center gap-3 text-sm text-muted-foreground hover:text-foreground transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            >
              <div
                className={`h-4 w-4 rounded border flex items-center justify-center transition-colors ${
                  visibleIds.size === allDomains.length
                    ? "bg-primary border-primary"
                    : visibleIds.size > 0
                      ? "bg-primary border-primary"
                      : "border-muted-foreground"
                }`}
              >
                {visibleIds.size === allDomains.length ? (
                  <Check className="h-3 w-3 text-primary-foreground" />
                ) : visibleIds.size > 0 ? (
                  <Minus className="h-3 w-3 text-primary-foreground" />
                ) : null}
              </div>
              <span>
                {visibleIds.size} of {allDomains.length} active
              </span>
            </button>
            <Button
              variant="outline"
              size="sm"
              onClick={handleRefreshDomains}
              disabled={refreshingDomains}
            >
              {refreshingDomains ? (
                <Spinner className="mr-2" />
              ) : (
                <RefreshCw className="h-3.5 w-3.5 mr-2" />
              )}
              Refresh
            </Button>
          </div>
          {allDomains.map((d) => {
            const active = visibleIds.has(d.id);
            return (
              <button
                key={d.id}
                type="button"
                onClick={() => {
                  if (user?.role !== "admin") return;
                  setVisibleIds((prev) => {
                    const next = new Set(prev);
                    if (next.has(d.id)) next.delete(d.id);
                    else next.add(d.id);
                    return next;
                  });
                }}
                disabled={user?.role !== "admin"}
                className={`flex w-full items-center justify-between rounded-lg border p-3 transition-colors disabled:cursor-not-allowed ${
                  active
                    ? "border-primary bg-primary/5"
                    : "border-muted opacity-50"
                }`}
              >
                <div className="flex items-center gap-3">
                  <div
                    className={`h-4 w-4 rounded border flex items-center justify-center transition-colors ${
                      active
                        ? "bg-primary border-primary"
                        : "border-muted-foreground"
                    }`}
                  >
                    {active && (
                      <Check className="h-3 w-3 text-primary-foreground" />
                    )}
                  </div>
                  <span className="font-medium">{d.domain}</span>
                </div>
                <Badge
                  variant={
                    d.status === "active"
                      ? "default"
                      : d.status === "verified"
                        ? "secondary"
                        : "outline"
                  }
                >
                  {d.status}
                </Badge>
              </button>
            );
          })}
          {allDomains.length === 0 && (
            <p className="text-sm text-muted-foreground">
              No domains found. Click Refresh to sync from Resend.
            </p>
          )}
        </CardContent>
        <CardFooter>
          <Button
            onClick={handleSaveDomainVisibility}
            disabled={savingDomains || user?.role !== "admin"}
          >
            {savingDomains ? <Spinner className="mr-2" /> : null}
            Save
          </Button>
        </CardFooter>
      </Card>

      {/* Sync */}
      <Card>
        <CardHeader>
          <CardTitle>Email Sync</CardTitle>
          <CardDescription>
            Re-import emails from Resend. Safe to run multiple times — duplicates are skipped.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {syncProgress && syncProgress.total > 0 && (
            <div className="space-y-2">
              <div className="flex justify-between text-sm text-muted-foreground">
                <span>{syncProgress.phase === "done" ? "Complete" : "Importing..."}</span>
                <span>{syncProgress.imported} / {syncProgress.total}</span>
              </div>
              <div className="h-2 rounded-full bg-muted overflow-hidden">
                <div
                  className="h-full bg-primary rounded-full transition-all duration-300 ease-out"
                  style={{ width: `${Math.round((syncProgress.imported / syncProgress.total) * 100)}%` }}
                />
              </div>
            </div>
          )}
          {syncProgress && syncProgress.phase === "fetching" && (
            <div className="flex items-center gap-2 text-sm text-muted-foreground">
              <Spinner /> Fetching emails from Resend...
            </div>
          )}
          {syncResult && (
            <div className="rounded-lg bg-green-50 border border-green-200 p-4 space-y-1">
              <p className="text-sm text-green-800">
                Imported <strong>{syncResult.sent_count}</strong> sent and{" "}
                <strong>{syncResult.received_count}</strong> received emails
              </p>
              <p className="text-sm text-green-800">
                Created <strong>{syncResult.thread_count}</strong> threads
              </p>
              <p className="text-sm text-green-800">
                Discovered <strong>{syncResult.address_count}</strong> addresses
              </p>
            </div>
          )}
        </CardContent>
        <CardFooter>
          <Button onClick={handleSync} disabled={syncing}>
            {syncing ? <><Spinner className="mr-2" /> Syncing...</> : "Sync emails"}
          </Button>
        </CardFooter>
      </Card>

      {/* Password */}
      <Card>
        <CardHeader>
          <CardTitle>Password</CardTitle>
          <CardDescription>Change your password</CardDescription>
        </CardHeader>
        <form onSubmit={handleUpdatePassword}>
          <CardContent className="space-y-4">
            <div className="space-y-2">
              <label className="text-sm font-medium">Current password</label>
              <Input
                type="password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                required
              />
            </div>
            <div className="space-y-2">
              <label className="text-sm font-medium">New password</label>
              <Input
                type="password"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                minLength={8}
                required
              />
            </div>
          </CardContent>
          <CardFooter>
            <Button disabled={savingPassword}>
              {savingPassword ? <Spinner className="mr-2" /> : null}
              Update password
            </Button>
          </CardFooter>
        </form>
      </Card>
    </div>
  );
}
