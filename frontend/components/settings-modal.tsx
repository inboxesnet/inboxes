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
import { useAppConfig } from "@/contexts/app-config-context";
import { useDomains } from "@/contexts/domain-context";
import { cn } from "@/lib/utils";
import type { User, Domain, BillingInfo } from "@/lib/types";
import { Check, Minus, RefreshCw, User as UserIcon, Globe, CreditCard, Users, AtSign, Trash2, RotateCw, UserX, UserPlus, X, Star } from "lucide-react";

type Tab = "profile" | "domains" | "team" | "aliases" | "billing";

interface SettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

interface OrgUser {
  id: string;
  email: string;
  name: string;
  role: string;
  status: string;
  created_at: string;
}

interface AliasUser {
  user_id: string;
  can_send_as: boolean;
  is_default: boolean;
  name: string;
  email: string;
}

interface AliasWithUsers {
  id: string;
  domain_id: string;
  address: string;
  name: string;
  created_at: string;
  users?: AliasUser[];
}

interface DiscoveredAddress {
  id: string;
  domain_id: string;
  address: string;
  local_part: string;
  type: string;
  email_count: number;
}

export function SettingsModal({ open, onOpenChange }: SettingsModalProps) {
  const { commercial } = useAppConfig();
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
  const [billingInfo, setBillingInfo] = useState<BillingInfo | null>(null);
  const [billingLoading, setBillingLoading] = useState(false);

  // Team management state
  const [orgUsers, setOrgUsers] = useState<OrgUser[]>([]);
  const [teamLoading, setTeamLoading] = useState(false);
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteName, setInviteName] = useState("");
  const [inviteRole, setInviteRole] = useState("member");
  const [inviting, setInviting] = useState(false);

  // Alias management state
  const [aliases, setAliases] = useState<AliasWithUsers[]>([]);
  const [aliasLoading, setAliasLoading] = useState(false);
  const [newAliasLocal, setNewAliasLocal] = useState("");
  const [newAliasDomain, setNewAliasDomain] = useState("");
  const [newAliasName, setNewAliasName] = useState("");
  const [creatingAlias, setCreatingAlias] = useState(false);
  const [expandedAlias, setExpandedAlias] = useState<string | null>(null);
  const [addUserAlias, setAddUserAlias] = useState("");

  // Discovered addresses state
  const [discoveredAddresses, setDiscoveredAddresses] = useState<DiscoveredAddress[]>([]);

  useEffect(() => {
    if (!open) return;
    setLoading(true);
    setError("");
    setSuccess("");
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

  // ─── Team Management ──────────────────────────────────────────────────

  async function loadTeam() {
    setTeamLoading(true);
    try {
      const data = await api.get<OrgUser[]>("/api/users");
      setOrgUsers(data);
    } catch {
      // handled
    } finally {
      setTeamLoading(false);
    }
  }

  async function handleInvite(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSuccess("");
    setInviting(true);
    try {
      await api.post("/api/users/invite", {
        email: inviteEmail,
        name: inviteName,
        role: inviteRole,
      });
      setInviteEmail("");
      setInviteName("");
      setInviteRole("member");
      setSuccess("Invitation sent");
      loadTeam();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to invite user");
    } finally {
      setInviting(false);
    }
  }

  async function handleReinvite(userId: string) {
    setError("");
    setSuccess("");
    try {
      await api.get(`/api/users/${userId}/reinvite`);
      setSuccess("Invite resent");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to resend invite");
    }
  }

  async function handleDisableUser(userId: string) {
    if (!confirm("Are you sure you want to disable this user?")) return;
    setError("");
    setSuccess("");
    try {
      await api.patch(`/api/users/${userId}/disable`);
      setSuccess("User disabled");
      loadTeam();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to disable user");
    }
  }

  // ─── Alias Management ─────────────────────────────────────────────────

  async function loadAliases() {
    setAliasLoading(true);
    try {
      const [aliasData, discoveredData] = await Promise.all([
        api.get<AliasWithUsers[]>("/api/aliases"),
        api.get<DiscoveredAddress[]>("/api/aliases/discovered"),
      ]);
      setAliases(aliasData);
      setDiscoveredAddresses(discoveredData);
    } catch {
      // handled
    } finally {
      setAliasLoading(false);
    }
  }

  async function handleCreateAlias(e: React.FormEvent) {
    e.preventDefault();
    if (!newAliasLocal || !newAliasDomain) return;
    setError("");
    setSuccess("");
    setCreatingAlias(true);
    try {
      await api.post("/api/aliases", {
        address: `${newAliasLocal}@${allDomains.find((d) => d.id === newAliasDomain)?.domain}`,
        name: newAliasName,
        domain_id: newAliasDomain,
      });
      setNewAliasLocal("");
      setNewAliasName("");
      setSuccess("Alias created");
      loadAliases();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create alias");
    } finally {
      setCreatingAlias(false);
    }
  }

  async function handleDeleteAlias(aliasId: string) {
    if (!confirm("Delete this alias? Emails will no longer be routed to it.")) return;
    setError("");
    setSuccess("");
    try {
      await api.delete(`/api/aliases/${aliasId}`);
      setSuccess("Alias deleted");
      loadAliases();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to delete alias");
    }
  }

  async function handleAddUserToAlias(aliasId: string, userId: string) {
    setError("");
    try {
      await api.post(`/api/aliases/${aliasId}/users`, { user_id: userId, can_send_as: true });
      loadAliases();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to add user");
    }
  }

  async function handleRemoveUserFromAlias(aliasId: string, userId: string) {
    setError("");
    try {
      await api.delete(`/api/aliases/${aliasId}/users/${userId}`);
      loadAliases();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to remove user");
    }
  }

  async function handleSetDefault(aliasId: string) {
    setError("");
    try {
      await api.patch(`/api/aliases/${aliasId}/default`);
      setSuccess("Default alias updated");
      loadAliases();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to set default");
    }
  }

  async function handleCreateFromDiscovered(addr: DiscoveredAddress) {
    setError("");
    try {
      await api.post("/api/aliases", {
        address: addr.address,
        name: addr.local_part,
        domain_id: addr.domain_id,
      });
      setSuccess(`Alias ${addr.address} created`);
      loadAliases();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create alias");
    }
  }

  const isAdmin = user?.role === "admin";

  const TABS: { key: Tab; label: string; icon: React.ReactNode; adminOnly?: boolean }[] = [
    { key: "profile", label: "Profile", icon: <UserIcon className="h-4 w-4" /> },
    { key: "domains", label: "Domains", icon: <Globe className="h-4 w-4" /> },
    { key: "team", label: "Team", icon: <Users className="h-4 w-4" />, adminOnly: true },
    { key: "aliases", label: "Aliases", icon: <AtSign className="h-4 w-4" />, adminOnly: true },
    ...(commercial
      ? [{ key: "billing" as Tab, label: "Billing", icon: <CreditCard className="h-4 w-4" /> }]
      : []),
  ];

  const visibleTabs = TABS.filter((t) => !t.adminOnly || isAdmin);

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
            {visibleTabs.map((tab) => (
              <button
                key={tab.key}
                onClick={() => {
                  setActiveTab(tab.key);
                  setError("");
                  setSuccess("");
                  if (tab.key === "billing" && !billingInfo && !billingLoading) {
                    loadBilling();
                  }
                  if (tab.key === "team" && orgUsers.length === 0 && !teamLoading) {
                    loadTeam();
                  }
                  if (tab.key === "aliases" && aliases.length === 0 && !aliasLoading) {
                    loadAliases();
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

                {activeTab === "billing" && commercial && (
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

                {activeTab === "team" && isAdmin && (
                  <div className="space-y-6">
                    {/* Invite form */}
                    <Card>
                      <CardHeader>
                        <CardTitle>Invite Team Member</CardTitle>
                        <CardDescription>Send an email invitation to join your workspace</CardDescription>
                      </CardHeader>
                      <form onSubmit={handleInvite}>
                        <CardContent className="space-y-4">
                          <div className="grid grid-cols-2 gap-4">
                            <div className="space-y-2">
                              <label className="text-sm font-medium">Email *</label>
                              <Input
                                type="email"
                                value={inviteEmail}
                                onChange={(e) => setInviteEmail(e.target.value)}
                                placeholder="colleague@company.com"
                                required
                              />
                            </div>
                            <div className="space-y-2">
                              <label className="text-sm font-medium">Name</label>
                              <Input
                                value={inviteName}
                                onChange={(e) => setInviteName(e.target.value)}
                                placeholder="Optional"
                              />
                            </div>
                          </div>
                          <div className="space-y-2">
                            <label className="text-sm font-medium">Role</label>
                            <select
                              value={inviteRole}
                              onChange={(e) => setInviteRole(e.target.value)}
                              className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                            >
                              <option value="member">Member</option>
                              <option value="admin">Admin</option>
                            </select>
                          </div>
                        </CardContent>
                        <CardFooter>
                          <Button disabled={inviting}>
                            {inviting ? <Spinner className="mr-2" /> : <UserPlus className="mr-2 h-4 w-4" />}
                            Send Invite
                          </Button>
                        </CardFooter>
                      </form>
                    </Card>

                    {/* User list */}
                    <Card>
                      <CardHeader>
                        <CardTitle>Team Members</CardTitle>
                        <CardDescription>{orgUsers.length} member{orgUsers.length !== 1 ? "s" : ""}</CardDescription>
                      </CardHeader>
                      <CardContent>
                        {teamLoading ? (
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> Loading team...
                          </div>
                        ) : (
                          <div className="divide-y">
                            {orgUsers.map((u) => (
                              <div
                                key={u.id}
                                className={cn(
                                  "flex items-center justify-between py-3",
                                  u.status === "disabled" && "opacity-50"
                                )}
                              >
                                <div className="space-y-1">
                                  <div className="flex items-center gap-2">
                                    <span className="text-sm font-medium">
                                      {u.name || u.email}
                                    </span>
                                    <Badge variant={u.role === "admin" ? "default" : "secondary"} className="text-xs">
                                      {u.role}
                                    </Badge>
                                    <Badge
                                      variant={
                                        u.status === "active"
                                          ? "default"
                                          : u.status === "disabled"
                                            ? "destructive"
                                            : "outline"
                                      }
                                      className="text-xs"
                                    >
                                      {u.status}
                                    </Badge>
                                  </div>
                                  {u.name && (
                                    <p className="text-xs text-muted-foreground">{u.email}</p>
                                  )}
                                </div>
                                <div className="flex items-center gap-2">
                                  {(u.status === "invited" || u.status === "placeholder") && (
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={() => handleReinvite(u.id)}
                                    >
                                      <RotateCw className="h-3 w-3 mr-1" />
                                      Resend
                                    </Button>
                                  )}
                                  {u.status === "active" && u.id !== user?.id && (
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={() => handleDisableUser(u.id)}
                                      className="text-destructive hover:text-destructive"
                                    >
                                      <UserX className="h-3 w-3 mr-1" />
                                      Disable
                                    </Button>
                                  )}
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </CardContent>
                    </Card>
                  </div>
                )}

                {activeTab === "aliases" && isAdmin && (
                  <div className="space-y-6">
                    {/* Discovered addresses — quick create */}
                    {discoveredAddresses.length > 0 && (
                      <Card>
                        <CardHeader>
                          <CardTitle>Discovered Addresses</CardTitle>
                          <CardDescription>
                            These addresses were found in your email traffic. Create aliases from them with one click.
                          </CardDescription>
                        </CardHeader>
                        <CardContent>
                          <div className="space-y-2">
                            {discoveredAddresses.map((addr) => (
                              <div key={addr.id} className="flex items-center justify-between rounded-lg border p-3">
                                <div className="space-y-0.5">
                                  <span className="text-sm font-medium">{addr.address}</span>
                                  <p className="text-xs text-muted-foreground">
                                    {addr.email_count} email{addr.email_count !== 1 ? "s" : ""}
                                  </p>
                                </div>
                                <Button
                                  size="sm"
                                  variant="outline"
                                  onClick={() => handleCreateFromDiscovered(addr)}
                                >
                                  Create Alias
                                </Button>
                              </div>
                            ))}
                          </div>
                        </CardContent>
                      </Card>
                    )}

                    {/* Create alias */}
                    <Card>
                      <CardHeader>
                        <CardTitle>Create Alias</CardTitle>
                        <CardDescription>Add an email alias for your team to send and receive from</CardDescription>
                      </CardHeader>
                      <form onSubmit={handleCreateAlias}>
                        <CardContent className="space-y-4">
                          <div className="flex items-center gap-2">
                            <div className="flex-1 space-y-2">
                              <label className="text-sm font-medium">Address</label>
                              <div className="flex items-center gap-1">
                                <Input
                                  value={newAliasLocal}
                                  onChange={(e) => setNewAliasLocal(e.target.value)}
                                  placeholder="hello"
                                  required
                                />
                                <span className="text-muted-foreground shrink-0">@</span>
                                <select
                                  value={newAliasDomain}
                                  onChange={(e) => setNewAliasDomain(e.target.value)}
                                  required
                                  className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                                >
                                  <option value="">Domain...</option>
                                  {allDomains.map((d) => (
                                    <option key={d.id} value={d.id}>{d.domain}</option>
                                  ))}
                                </select>
                              </div>
                            </div>
                          </div>
                          <div className="space-y-2">
                            <label className="text-sm font-medium">Display Name</label>
                            <Input
                              value={newAliasName}
                              onChange={(e) => setNewAliasName(e.target.value)}
                              placeholder="e.g. Support, Sales (optional)"
                            />
                          </div>
                        </CardContent>
                        <CardFooter>
                          <Button disabled={creatingAlias}>
                            {creatingAlias ? <Spinner className="mr-2" /> : null}
                            Create Alias
                          </Button>
                        </CardFooter>
                      </form>
                    </Card>

                    {/* Alias list */}
                    <Card>
                      <CardHeader>
                        <CardTitle>Aliases</CardTitle>
                        <CardDescription>{aliases.length} alias{aliases.length !== 1 ? "es" : ""}</CardDescription>
                      </CardHeader>
                      <CardContent>
                        {aliasLoading ? (
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> Loading aliases...
                          </div>
                        ) : aliases.length === 0 ? (
                          <p className="text-sm text-muted-foreground">No aliases yet. Create one above.</p>
                        ) : (
                          <div className="divide-y">
                            {/* Group by domain */}
                            {allDomains
                              .filter((d) => aliases.some((a) => a.domain_id === d.id))
                              .map((domain) => (
                                <div key={domain.id} className="py-3">
                                  <p className="text-xs font-medium text-muted-foreground uppercase tracking-wide mb-2">
                                    {domain.domain}
                                  </p>
                                  <div className="space-y-2">
                                    {aliases
                                      .filter((a) => a.domain_id === domain.id)
                                      .map((alias) => {
                                        const isMyDefault = alias.users?.some((au) => au.user_id === user?.id && au.is_default);
                                        return (
                                        <div key={alias.id} className="rounded-lg border p-3 space-y-2">
                                          <div className="flex items-center justify-between">
                                            <div className="flex items-center gap-2">
                                              <button
                                                onClick={() => handleSetDefault(alias.id)}
                                                title={isMyDefault ? "Default send-from" : "Set as default send-from"}
                                                className="shrink-0"
                                              >
                                                <Star className={cn(
                                                  "h-4 w-4",
                                                  isMyDefault
                                                    ? "text-yellow-500 fill-yellow-500"
                                                    : "text-muted-foreground/40 hover:text-yellow-500"
                                                )} />
                                              </button>
                                              <span className="text-sm font-medium">{alias.address}</span>
                                              {alias.name && (
                                                <span className="text-xs text-muted-foreground">({alias.name})</span>
                                              )}
                                            </div>
                                            <div className="flex items-center gap-2">
                                              <button
                                                onClick={() => setExpandedAlias(expandedAlias === alias.id ? null : alias.id)}
                                                className="text-xs text-muted-foreground hover:text-foreground"
                                              >
                                                {(alias.users?.length || 0)} user{(alias.users?.length || 0) !== 1 ? "s" : ""}
                                              </button>
                                              <Button
                                                variant="ghost"
                                                size="icon"
                                                className="h-7 w-7 text-destructive hover:text-destructive"
                                                onClick={() => handleDeleteAlias(alias.id)}
                                              >
                                                <Trash2 className="h-3.5 w-3.5" />
                                              </Button>
                                            </div>
                                          </div>
                                          {/* Expanded user list */}
                                          {expandedAlias === alias.id && (
                                            <div className="border-t pt-2 space-y-2">
                                              {alias.users && alias.users.length > 0 ? (
                                                alias.users.map((au) => (
                                                  <div key={au.user_id} className="flex items-center justify-between text-sm">
                                                    <span>{au.name || au.email}</span>
                                                    <button
                                                      onClick={() => handleRemoveUserFromAlias(alias.id, au.user_id)}
                                                      className="text-xs text-destructive hover:underline"
                                                    >
                                                      <X className="h-3 w-3" />
                                                    </button>
                                                  </div>
                                                ))
                                              ) : (
                                                <p className="text-xs text-muted-foreground">No users assigned</p>
                                              )}
                                              {/* Add user dropdown */}
                                              <div className="flex items-center gap-2">
                                                <select
                                                  value={addUserAlias}
                                                  onChange={(e) => setAddUserAlias(e.target.value)}
                                                  className="flex h-7 flex-1 rounded-md border border-input bg-transparent px-2 text-xs shadow-sm focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                                                >
                                                  <option value="">Add user...</option>
                                                  {orgUsers
                                                    .filter((u) => u.status === "active" && !alias.users?.some((au) => au.user_id === u.id))
                                                    .map((u) => (
                                                      <option key={u.id} value={u.id}>{u.name || u.email}</option>
                                                    ))}
                                                </select>
                                                <Button
                                                  size="sm"
                                                  variant="outline"
                                                  className="h-7 text-xs"
                                                  disabled={!addUserAlias}
                                                  onClick={() => {
                                                    if (addUserAlias) {
                                                      handleAddUserToAlias(alias.id, addUserAlias);
                                                      setAddUserAlias("");
                                                    }
                                                  }}
                                                >
                                                  Add
                                                </Button>
                                              </div>
                                            </div>
                                          )}
                                        </div>
                                        );
                                      })}
                                  </div>
                                </div>
                              ))}
                          </div>
                        )}
                      </CardContent>
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
