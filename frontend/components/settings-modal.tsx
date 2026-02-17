"use client";

import { useState, useEffect } from "react";
import { api, ApiError } from "@/lib/api";
import { useSyncJob } from "@/hooks/use-sync-job";
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
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";
import { Badge } from "@/components/ui/badge";
import { useDomains } from "@/contexts/domain-context";
import { cn } from "@/lib/utils";
import type { User, Domain, BillingInfo } from "@/lib/types";
import { Check, Minus, RefreshCw, User as UserIcon, Globe, CreditCard } from "lucide-react";

type Tab = "profile" | "domains" | "billing";

interface SettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function SettingsModal({ open, onOpenChange }: SettingsModalProps) {
  const { refreshDomains } = useDomains();
  const [activeTab, setActiveTab] = useState<Tab>("profile");
  const [user, setUser] = useState<User | null>(null);
  const [name, setName] = useState("");
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [savingPassword, setSavingPassword] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");
  const {
    progress: syncProgress,
    result: syncResult,
    error: syncError,
    isRunning: syncing,
    startSync,
  } = useSyncJob();

  // Domain management state
  const [allDomains, setAllDomains] = useState<Domain[]>([]);
  const [visibleIds, setVisibleIds] = useState<Set<string>>(new Set());
  const [savingDomains, setSavingDomains] = useState(false);
  const [refreshingDomains, setRefreshingDomains] = useState(false);

  // Billing state
  const [billingEnabled, setBillingEnabled] = useState(false);
  const [billingInfo, setBillingInfo] = useState<BillingInfo | null>(null);
  const [billingLoading, setBillingLoading] = useState(false);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError("");
    setSuccess("");
    async function load() {
      try {
        const [userData, domainData, settingsData] = await Promise.all([
          api.get<User>("/api/users/me"),
          api.get<Domain[]>("/api/domains/all"),
          api.get<{ billing_enabled: boolean }>("/api/orgs/settings"),
        ]);
        setUser(userData);
        setName(userData.name);
        setAllDomains(domainData);
        setVisibleIds(new Set(domainData.filter((d) => !d.hidden).map((d) => d.id)));
        setBillingEnabled(settingsData.billing_enabled);
      } catch {
        // handled
      } finally {
        setLoading(false);
      }
    }
    load();
  }, [open]);

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
    startSync();
  }

  async function loadBilling() {
    setBillingLoading(true);
    try {
      const data = await api.get<BillingInfo>("/api/billing");
      setBillingInfo(data);
    } catch {
      // handled
    } finally {
      setBillingLoading(false);
    }
  }

  async function handleCheckout() {
    setError("");
    try {
      const data = await api.post<{ url: string }>("/api/billing/checkout");
      window.location.href = data.url;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start checkout");
    }
  }

  async function handleManageBilling() {
    setError("");
    try {
      const data = await api.post<{ url: string }>("/api/billing/portal");
      window.location.href = data.url;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to open billing portal");
    }
  }

  const TABS: { key: Tab; label: string; icon: React.ReactNode }[] = [
    { key: "profile", label: "Profile", icon: <UserIcon className="h-4 w-4" /> },
    { key: "domains", label: "Domains", icon: <Globe className="h-4 w-4" /> },
    ...(billingEnabled
      ? [{ key: "billing" as Tab, label: "Billing", icon: <CreditCard className="h-4 w-4" /> }]
      : []),
  ];

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-3xl p-0 overflow-hidden h-[min(600px,80vh)]"
        onClose={() => onOpenChange(false)}
      >
        <div className="flex h-full">
          {/* Sidebar */}
          <div className="w-[180px] border-r bg-muted/30 p-3 space-y-1 shrink-0">
            <h3 className="font-semibold text-sm px-2 py-2">Settings</h3>
            {TABS.map((tab) => (
              <button
                key={tab.key}
                onClick={() => {
                  setActiveTab(tab.key);
                  setError("");
                  setSuccess("");
                  if (tab.key === "billing" && !billingInfo && !billingLoading) {
                    loadBilling();
                  }
                }}
                className={cn(
                  "flex items-center gap-2 w-full rounded-md px-2 py-1.5 text-sm transition-colors",
                  activeTab === tab.key
                    ? "bg-accent text-accent-foreground font-medium"
                    : "text-muted-foreground hover:bg-accent/50 hover:text-foreground"
                )}
              >
                {tab.icon}
                {tab.label}
              </button>
            ))}
          </div>

          {/* Content */}
          <div className="flex-1 overflow-y-auto p-6">
            {loading ? (
              <div className="flex items-center justify-center h-full">
                <Spinner className="h-6 w-6" />
              </div>
            ) : (
              <>
                {error && (
                  <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md mb-4">
                    {error}
                  </div>
                )}
                {success && (
                  <div className="text-sm text-green-700 bg-green-50 p-3 rounded-md mb-4">
                    {success}
                  </div>
                )}

                {activeTab === "profile" && (
                  <div className="space-y-6">
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

                    {/* Sync */}
                    <Card>
                      <CardHeader>
                        <CardTitle>Email Sync</CardTitle>
                        <CardDescription>
                          Re-import emails from Resend. Safe to run multiple times — duplicates are skipped.
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-4">
                        {syncError && (
                          <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
                            {syncError}
                          </div>
                        )}
                        {syncProgress && (syncProgress.phase === "scanning" || syncProgress.phase === "pending") && (
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> {syncProgress.message}
                          </div>
                        )}
                        {syncProgress && syncProgress.phase === "importing" && syncProgress.total > 0 && (
                          <div className="space-y-2">
                            <div className="flex justify-between text-sm text-muted-foreground">
                              <span>Importing...</span>
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
                  </div>
                )}

                {activeTab === "billing" && billingEnabled && (
                  <div className="space-y-6">
                    <Card>
                      <CardHeader>
                        <CardTitle>Subscription</CardTitle>
                        <CardDescription>
                          Manage your subscription and billing
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-4">
                        {billingLoading ? (
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> Loading billing info...
                          </div>
                        ) : billingInfo ? (
                          <div className="space-y-3">
                            <div className="flex items-center gap-2">
                              <span className="text-sm font-medium">Plan:</span>
                              <Badge variant={billingInfo.plan === "pro" ? "default" : "secondary"}>
                                {billingInfo.plan === "pro" ? "Pro" : billingInfo.plan === "cancelled" ? "Cancelled" : "Free"}
                              </Badge>
                            </div>
                            {billingInfo.subscription && (
                              <>
                                {billingInfo.subscription.cancel_at_period_end && (
                                  <p className="text-sm text-muted-foreground">
                                    Your subscription will end on{" "}
                                    {new Date(billingInfo.subscription.current_period_end).toLocaleDateString()}
                                  </p>
                                )}
                                {!billingInfo.subscription.cancel_at_period_end && billingInfo.plan === "pro" && (
                                  <p className="text-sm text-muted-foreground">
                                    Next billing date:{" "}
                                    {new Date(billingInfo.subscription.current_period_end).toLocaleDateString()}
                                  </p>
                                )}
                              </>
                            )}
                            {billingInfo.plan === "cancelled" && billingInfo.plan_expires_at && (
                              <p className="text-sm text-muted-foreground">
                                Access until: {new Date(billingInfo.plan_expires_at).toLocaleDateString()}
                              </p>
                            )}
                          </div>
                        ) : (
                          <p className="text-sm text-muted-foreground">
                            Unable to load billing information.
                          </p>
                        )}
                      </CardContent>
                      <CardFooter>
                        {billingInfo?.plan === "pro" || (billingInfo?.plan === "cancelled" && billingInfo?.subscription) ? (
                          <Button onClick={handleManageBilling}>
                            Manage subscription
                          </Button>
                        ) : (
                          <Button onClick={handleCheckout} disabled={user?.role !== "admin"}>
                            Upgrade to Pro
                          </Button>
                        )}
                      </CardFooter>
                    </Card>
                  </div>
                )}

                {activeTab === "domains" && (
                  <div className="space-y-6">
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
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
