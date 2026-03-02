"use client";

import { useState, useEffect, useRef } from "react";
import { toast } from "sonner";
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
import { cn, validatePassword } from "@/lib/utils";
import type { User, Domain, BillingInfo } from "@/lib/types";
import { Check, Minus, RefreshCw, User as UserIcon, Globe, CreditCard, Users, AtSign, Trash2, RotateCw, UserX, UserPlus, X, Star, Pencil, Wrench, Building2, Tag } from "lucide-react";

export type Tab = "profile" | "domains" | "team" | "aliases" | "labels" | "organization" | "billing" | "system" | "jobs";

interface SettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  defaultTab?: Tab;
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

interface EmailJob {
  id: string;
  job_type: string;
  status: string;
  email_id?: string;
  thread_id?: string;
  error_message?: string;
  attempts: number;
  created_at: string;
  updated_at: string;
}

function JobsPanel() {
  const [jobs, setJobs] = useState<EmailJob[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    setLoading(true);
    api
      .get<{ jobs: EmailJob[] }>("/api/admin/jobs")
      .then((data) => setJobs(data.jobs))
      .catch(() => setError("Failed to load jobs"))
      .finally(() => setLoading(false));
  }, []);

  const statusColor = (s: string) => {
    switch (s) {
      case "completed": return "bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-400";
      case "failed": return "bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-400";
      case "pending": return "bg-yellow-100 text-yellow-800 dark:bg-yellow-900/30 dark:text-yellow-400";
      case "processing": return "bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-400";
      default: return "bg-muted text-muted-foreground";
    }
  };

  if (loading) {
    return (
      <Card>
        <CardContent className="py-8">
          <div className="flex items-center justify-center gap-2 text-sm text-muted-foreground">
            <Spinner /> Loading jobs...
          </div>
        </CardContent>
      </Card>
    );
  }

  if (error) {
    return (
      <Card>
        <CardContent className="py-8 text-center text-sm text-muted-foreground">{error}</CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Email Jobs</CardTitle>
        <CardDescription>Recent email sending jobs for your organization</CardDescription>
      </CardHeader>
      <CardContent>
        {jobs.length === 0 ? (
          <p className="text-sm text-muted-foreground">No jobs found.</p>
        ) : (
          <div className="border rounded-md overflow-hidden">
            <table className="w-full text-sm">
              <thead>
                <tr className="bg-muted/50 text-left">
                  <th className="px-3 py-2 font-medium">Type</th>
                  <th className="px-3 py-2 font-medium">Status</th>
                  <th className="px-3 py-2 font-medium">Attempts</th>
                  <th className="px-3 py-2 font-medium">Created</th>
                  <th className="px-3 py-2 font-medium">Error</th>
                </tr>
              </thead>
              <tbody>
                {jobs.map((job) => (
                  <tr key={job.id} className="border-t">
                    <td className="px-3 py-2 font-mono text-xs">{job.job_type}</td>
                    <td className="px-3 py-2">
                      <span className={cn("inline-block px-2 py-0.5 rounded text-xs font-medium", statusColor(job.status))}>
                        {job.status}
                      </span>
                    </td>
                    <td className="px-3 py-2">{job.attempts}</td>
                    <td className="px-3 py-2 text-muted-foreground text-xs">
                      {new Date(job.created_at).toLocaleString()}
                    </td>
                    <td className="px-3 py-2 text-xs text-destructive max-w-[200px] truncate">
                      {job.error_message || "—"}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function NotificationsCard() {
  const notifSupported = typeof Notification !== "undefined";
  const [permission, setPermission] = useState(
    () => notifSupported ? Notification.permission : "denied"
  );
  const enabled = permission === "granted";
  const denied = permission === "denied" && notifSupported;
  const isIncognito = !notifSupported;

  async function handleToggle(checked: boolean) {
    if (checked) {
      if (!notifSupported) return;
      const result = await Notification.requestPermission();
      setPermission(result);
      const granted = result === "granted";
      await api.patch("/api/users/me/preferences", { desktop_notifications: granted }).catch(() => {
        toast.error("Failed to save");
      });
    } else {
      await api.patch("/api/users/me/preferences", { desktop_notifications: false }).catch(() => {
        toast.error("Failed to save");
      });
      setPermission("default");
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Notifications</CardTitle>
        <CardDescription>Manage notification preferences</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        <label className="flex items-center gap-3 cursor-pointer">
          <input
            type="checkbox"
            className="h-4 w-4 rounded border"
            checked={enabled}
            disabled={isIncognito}
            onChange={(e) => handleToggle(e.target.checked)}
          />
          <div>
            <p className="text-sm font-medium">Desktop notifications</p>
            <p className="text-xs text-muted-foreground">Receive browser notifications for new emails</p>
          </div>
        </label>
        {isIncognito && (
          <p className="text-xs text-muted-foreground bg-muted rounded-md p-2">
            Desktop notifications are not available in incognito/private browsing mode. Open Inboxes in a regular browser window to enable them.
          </p>
        )}
        {denied && (
          <div className="text-xs text-muted-foreground bg-muted rounded-md p-2 space-y-1">
            <p className="font-medium text-foreground">Notifications are blocked by your browser</p>
            <p>Click the lock icon (or tune icon) in your browser&apos;s address bar, find &quot;Notifications&quot;, and change it to &quot;Allow&quot;. Then reload the page.</p>
            <p className="text-yellow-500">Note: if you&apos;re in an incognito window, notifications may not work - try a regular browser window instead.</p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

export function SettingsModal({ open, onOpenChange, defaultTab }: SettingsModalProps) {
  const { commercial } = useAppConfig();
  const { refreshDomains } = useDomains();
  const [activeTab, setActiveTab] = useState<Tab>(defaultTab || "profile");
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

  // Add domain state
  const [newDomainName, setNewDomainName] = useState("");
  const [addingDomain, setAddingDomain] = useState(false);
  const [verifyingDomain, setVerifyingDomain] = useState<string | null>(null);
  const [reorderingDomains, setReorderingDomains] = useState(false);
  const [reregisteringWebhook, setReregisteringWebhook] = useState(false);
  const [deletingDomain, setDeletingDomain] = useState<string | null>(null);

  // Billing state
  const [billingInfo, setBillingInfo] = useState<BillingInfo | null>(null);
  const [billingLoading, setBillingLoading] = useState(false);
  const [billingError, setBillingError] = useState("");

  // Team management state
  const [orgUsers, setOrgUsers] = useState<OrgUser[]>([]);
  const [teamLoading, setTeamLoading] = useState(false);
  const [teamError, setTeamError] = useState("");
  const [inviteEmail, setInviteEmail] = useState("");
  const [inviteName, setInviteName] = useState("");
  const [inviteRole, setInviteRole] = useState("member");
  const [inviting, setInviting] = useState(false);

  // Alias management state
  const [aliases, setAliases] = useState<AliasWithUsers[]>([]);
  const [aliasLoading, setAliasLoading] = useState(false);
  const [aliasError, setAliasError] = useState("");
  const [newAliasLocal, setNewAliasLocal] = useState("");
  const [newAliasDomain, setNewAliasDomain] = useState("");
  const [newAliasName, setNewAliasName] = useState("");
  const [creatingAlias, setCreatingAlias] = useState(false);
  const [expandedAlias, setExpandedAlias] = useState<string | null>(null);
  const [addUserAlias, setAddUserAlias] = useState("");
  const [editingAliasId, setEditingAliasId] = useState<string | null>(null);
  const [editingAliasName, setEditingAliasName] = useState("");

  // Discovered addresses state
  const [discoveredAddresses, setDiscoveredAddresses] = useState<DiscoveredAddress[]>([]);

  // Label management state
  const [customLabels, setCustomLabels] = useState<{ id: string; name: string }[]>([]);
  const [labelsLoading, setLabelsLoading] = useState(false);
  const [labelsError, setLabelsError] = useState("");
  const [newLabelName, setNewLabelName] = useState("");
  const [creatingLabel, setCreatingLabel] = useState(false);
  const [editingLabelId, setEditingLabelId] = useState<string | null>(null);
  const [editingLabelName, setEditingLabelName] = useState("");

  // Non-admin alias view state
  const [myAliases, setMyAliases] = useState<{ id: string; address: string; name: string; domain_id: string; can_send_as: boolean; is_default: boolean }[]>([]);
  const [myAliasesLoading, setMyAliasesLoading] = useState(false);

  // Organization settings state
  const [orgName, setOrgName] = useState("");
  const [orgResendKey, setOrgResendKey] = useState("");
  const [orgResendRPS, setOrgResendRPS] = useState(2);
  const [orgLoading, setOrgLoading] = useState(false);
  const [savingOrg, setSavingOrg] = useState(false);
  const [showDeleteConfirm, setShowDeleteConfirm] = useState(false);
  const [deleteConfirmText, setDeleteConfirmText] = useState("");

  // System email state
  const [systemFromAddress, setSystemFromAddress] = useState("");
  const [systemFromName, setSystemFromName] = useState("");
  const [systemLoading, setSystemLoading] = useState(false);
  const [savingSystem, setSavingSystem] = useState(false);
  const [sendingTest, setSendingTest] = useState(false);

  useEffect(() => {
    if (!open) return;
    if (defaultTab) setActiveTab(defaultTab);
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

    const pwError = validatePassword(newPassword);
    if (pwError) {
      setError(pwError);
      return;
    }

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

  async function handleAddDomain(e: React.FormEvent) {
    e.preventDefault();
    if (!newDomainName) return;
    setError("");
    setSuccess("");
    setAddingDomain(true);
    try {
      const result = await api.post<Domain>("/api/domains", { domain: newDomainName });
      setAllDomains((prev) => [...prev, result]);
      setNewDomainName("");
      setSuccess("Domain added. Configure DNS records below, then verify.");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to add domain");
    } finally {
      setAddingDomain(false);
    }
  }

  async function handleVerifyDomain(domainId: string) {
    setError("");
    setSuccess("");
    setVerifyingDomain(domainId);
    try {
      const result = await api.post<{ status: string; dns_records: unknown }>(`/api/domains/${domainId}/verify`);
      setAllDomains((prev) =>
        prev.map((d) => d.id === domainId ? { ...d, status: result.status as Domain["status"], dns_records: result.dns_records } : d)
      );
      if (result.status === "verified" || result.status === "active") {
        setSuccess("Domain verified!");
      } else {
        setSuccess(`Domain status: ${result.status}. DNS records may still be propagating.`);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to verify domain");
    } finally {
      setVerifyingDomain(null);
    }
  }

  async function handleReorderDomain(domainId: string, direction: "up" | "down") {
    const idx = allDomains.findIndex((d) => d.id === domainId);
    if (idx < 0) return;
    const swapIdx = direction === "up" ? idx - 1 : idx + 1;
    if (swapIdx < 0 || swapIdx >= allDomains.length) return;

    const updated = [...allDomains];
    [updated[idx], updated[swapIdx]] = [updated[swapIdx], updated[idx]];
    setAllDomains(updated);

    try {
      await api.patch("/api/domains/reorder", {
        order: updated.map((d, i) => ({ id: d.id, order: i })),
      });
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to reorder");
      // Revert
      setAllDomains(allDomains);
    }
  }

  async function handleReregisterWebhook() {
    if (!confirm("Re-register the webhook endpoint with Resend? This will create a new webhook.")) return;
    setError("");
    setSuccess("");
    setReregisteringWebhook(true);
    try {
      await api.post(`/api/domains/${allDomains[0]?.id}/webhook`);
      setSuccess("Webhook re-registered successfully");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to re-register webhook");
    } finally {
      setReregisteringWebhook(false);
    }
  }

  async function handleDeleteDomain(domainId: string) {
    if (!confirm("Remove this domain? Emails and aliases will be preserved but the domain will be hidden.")) return;
    setError("");
    setSuccess("");
    setDeletingDomain(domainId);
    try {
      await api.delete(`/api/domains/${domainId}`);
      setAllDomains((prev) => prev.filter((d) => d.id !== domainId));
      setVisibleIds((prev) => { const next = new Set(prev); next.delete(domainId); return next; });
      await refreshDomains();
      setSuccess("Domain removed");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to remove domain");
    } finally {
      setDeletingDomain(null);
    }
  }

  function handleSync() {
    setError("");
    startSync();
  }

  async function loadBilling() {
    setBillingLoading(true);
    setBillingError("");
    try {
      const data = await api.get<BillingInfo>("/api/billing");
      setBillingInfo(data);
    } catch {
      setBillingError("Failed to load billing info");
    } finally {
      setBillingLoading(false);
    }
  }

  const [checkoutLoading, setCheckoutLoading] = useState(false);
  const checkoutRef = useRef(false);

  async function handleCheckout() {
    if (checkoutRef.current) return;
    checkoutRef.current = true;
    setError("");
    setCheckoutLoading(true);
    try {
      const data = await api.post<{ url: string }>("/api/billing/checkout");
      const parsed = new URL(data.url);
      if (parsed.protocol !== "https:" || parsed.hostname !== "checkout.stripe.com") throw new Error("Invalid checkout URL");
      window.location.href = data.url;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to start checkout");
      checkoutRef.current = false;
      setCheckoutLoading(false);
    }
  }

  async function handleManageBilling() {
    setError("");
    try {
      const data = await api.post<{ url: string }>("/api/billing/portal");
      const parsed = new URL(data.url);
      if (!parsed.hostname.endsWith("stripe.com")) throw new Error("Invalid billing portal URL");
      window.location.href = data.url;
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to open billing portal");
    }
  }

  // ─── Team Management ──────────────────────────────────────────────────

  async function loadTeam() {
    setTeamLoading(true);
    setTeamError("");
    try {
      const data = await api.get<OrgUser[]>("/api/users");
      setOrgUsers(data);
    } catch {
      setTeamError("Failed to load team members");
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
      await api.post(`/api/users/${userId}/reinvite`, {});
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

  async function handleEnableUser(userId: string) {
    setError("");
    setSuccess("");
    try {
      await api.patch(`/api/users/${userId}/enable`);
      setSuccess("User re-enabled");
      loadTeam();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to enable user");
    }
  }

  async function handleChangeRole(userId: string, newRole: string) {
    setError("");
    setSuccess("");
    try {
      await api.patch(`/api/users/${userId}/role`, { role: newRole });
      setSuccess(`Role updated to ${newRole}`);
      loadTeam();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to change role");
    }
  }

  async function handleDeleteOrg() {
    if (deleteConfirmText !== "DELETE") return;
    setError("");
    setSuccess("");
    try {
      await api.delete("/api/orgs");
      window.location.href = "/login";
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to delete organization");
    }
  }

  // ─── Alias Management ─────────────────────────────────────────────────

  async function loadAliases() {
    setAliasLoading(true);
    setAliasError("");
    // Also load org users for the user assignment dropdown
    if (orgUsers.length === 0 && !teamLoading) loadTeam();
    try {
      const [aliasData, discoveredData] = await Promise.all([
        api.get<AliasWithUsers[]>("/api/aliases"),
        api.get<DiscoveredAddress[]>("/api/aliases/discovered"),
      ]);
      setAliases(aliasData);
      setDiscoveredAddresses(discoveredData);
    } catch {
      setAliasError("Failed to load aliases");
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

  async function handleUpdateAliasName(aliasId: string, newName: string) {
    setError("");
    try {
      await api.patch(`/api/aliases/${aliasId}`, { name: newName });
      setAliases((prev) =>
        prev.map((a) => (a.id === aliasId ? { ...a, name: newName } : a))
      );
      setEditingAliasId(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update alias name");
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

  // ─── System Email ──────────────────────────────────────────────────

  async function loadSystemEmail() {
    setSystemLoading(true);
    try {
      const data = await api.get<{ from_address: string; from_name: string }>("/api/system/email");
      setSystemFromAddress(data.from_address || "");
      setSystemFromName(data.from_name || "");
    } catch {
      // handled
    } finally {
      setSystemLoading(false);
    }
  }

  async function handleSaveSystemEmail(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSuccess("");
    setSavingSystem(true);
    try {
      await api.patch("/api/system/email", {
        from_address: systemFromAddress,
        from_name: systemFromName,
      });
      setSuccess("System email updated");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to save");
    } finally {
      setSavingSystem(false);
    }
  }

  async function handleSendTestEmail() {
    setError("");
    setSuccess("");
    setSendingTest(true);
    try {
      const result = await api.patch<{ saved: boolean; test_sent: boolean; test_error?: string }>(
        "/api/system/email",
        {
          from_address: systemFromAddress,
          from_name: systemFromName,
          send_test: true,
        }
      );
      if (result.test_sent) {
        setSuccess("Test email sent to your address");
      } else if (result.test_error) {
        setError(`Test email failed: ${result.test_error}`);
      }
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to send test");
    } finally {
      setSendingTest(false);
    }
  }

  // ─── Label Management ────────────────────────────────────────────────

  async function loadLabels() {
    setLabelsLoading(true);
    setLabelsError("");
    try {
      const data = await api.get<{ id: string; name: string }[]>("/api/labels");
      setCustomLabels(data);
    } catch {
      setLabelsError("Failed to load labels");
    } finally {
      setLabelsLoading(false);
    }
  }

  async function handleCreateLabel(e: React.FormEvent) {
    e.preventDefault();
    if (!newLabelName.trim()) return;
    setError("");
    setSuccess("");
    setCreatingLabel(true);
    try {
      await api.post("/api/labels", { name: newLabelName.trim() });
      setNewLabelName("");
      setSuccess("Label created");
      loadLabels();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to create label");
    } finally {
      setCreatingLabel(false);
    }
  }

  async function handleRenameLabel(labelId: string, newName: string) {
    setError("");
    try {
      await api.patch(`/api/labels/${labelId}`, { name: newName });
      setEditingLabelId(null);
      loadLabels();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to rename label");
    }
  }

  async function handleDeleteLabel(labelId: string) {
    if (!confirm("Delete this label? It will be removed from all threads.")) return;
    setError("");
    try {
      await api.delete(`/api/labels/${labelId}`);
      loadLabels();
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to delete label");
    }
  }

  // ─── My Aliases (non-admin) ───────────────────────────────────────────

  async function loadMyAliases() {
    setMyAliasesLoading(true);
    try {
      const data = await api.get<{ id: string; address: string; name: string; domain_id: string; can_send_as: boolean; is_default: boolean }[]>("/api/users/me/aliases");
      setMyAliases(data);
    } catch {
      // handled
    } finally {
      setMyAliasesLoading(false);
    }
  }

  // ─── Organization Settings ──────────────────────────────────────────────

  async function loadOrgSettings() {
    setOrgLoading(true);
    try {
      const data = await api.get<{ name: string; has_api_key: boolean; resend_rps: number }>("/api/orgs/settings");
      setOrgName(data.name || "");
      setOrgResendKey(data.has_api_key ? "********" : "");
      setOrgResendRPS(data.resend_rps || 2);
    } catch {
      // handled
    } finally {
      setOrgLoading(false);
    }
  }

  async function handleSaveOrgSettings(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setSuccess("");

    if (!orgResendRPS || orgResendRPS < 1 || orgResendRPS > 100) {
      setError("Rate limit must be between 1 and 100");
      return;
    }

    setSavingOrg(true);
    try {
      const payload: Record<string, unknown> = { name: orgName, resend_rps: orgResendRPS };
      if (orgResendKey && orgResendKey !== "********") {
        payload.api_key = orgResendKey;
      }
      await api.patch("/api/orgs/settings", payload);
      setSuccess("Organization settings updated");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to update");
    } finally {
      setSavingOrg(false);
    }
  }

  const isAdmin = user?.role === "admin";

  function activateTab(tabKey: Tab) {
    setActiveTab(tabKey);
    setError("");
    setSuccess("");
    if (tabKey === "billing" && !billingInfo && !billingLoading) loadBilling();
    if (tabKey === "team" && orgUsers.length === 0 && !teamLoading) loadTeam();
    if (tabKey === "aliases" && isAdmin && aliases.length === 0 && !aliasLoading) {
      loadAliases();
      if (orgUsers.length === 0 && !teamLoading) loadTeam();
    }
    if (tabKey === "aliases" && !isAdmin && myAliases.length === 0 && !myAliasesLoading) loadMyAliases();
    if (tabKey === "labels" && customLabels.length === 0 && !labelsLoading) loadLabels();
    if (tabKey === "organization" && !orgLoading && !orgName) loadOrgSettings();
    if (tabKey === "system" && !systemLoading && !systemFromAddress) loadSystemEmail();
  }

  const TABS: { key: Tab; label: string; icon: React.ReactNode; adminOnly?: boolean }[] = [
    { key: "profile", label: "Profile", icon: <UserIcon className="h-4 w-4" /> },
    { key: "domains", label: "Domains", icon: <Globe className="h-4 w-4" /> },
    { key: "team", label: "Team", icon: <Users className="h-4 w-4" />, adminOnly: true },
    { key: "aliases", label: "Aliases", icon: <AtSign className="h-4 w-4" /> },
    { key: "labels", label: "Labels", icon: <Tag className="h-4 w-4" /> },
    { key: "organization", label: "Organization", icon: <Building2 className="h-4 w-4" />, adminOnly: true },
    ...(commercial
      ? [{ key: "billing" as Tab, label: "Billing", icon: <CreditCard className="h-4 w-4" /> }]
      : []),
    ...(!commercial && user?.is_owner
      ? [{ key: "system" as Tab, label: "System", icon: <Wrench className="h-4 w-4" /> }]
      : []),
    ...(isAdmin
      ? [{ key: "jobs" as Tab, label: "Jobs", icon: <RefreshCw className="h-4 w-4" />, adminOnly: true }]
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
            <div
              role="tablist"
              aria-label="Settings"
              aria-orientation="vertical"
              className="space-y-1"
              onKeyDown={(e) => {
                const keys = ["ArrowDown", "ArrowUp", "Home", "End"];
                if (!keys.includes(e.key)) return;
                e.preventDefault();
                const tabKeys = visibleTabs.map((t) => t.key);
                const currentIndex = tabKeys.indexOf(activeTab);
                let nextIndex = currentIndex;
                if (e.key === "ArrowDown") nextIndex = (currentIndex + 1) % tabKeys.length;
                else if (e.key === "ArrowUp") nextIndex = (currentIndex - 1 + tabKeys.length) % tabKeys.length;
                else if (e.key === "Home") nextIndex = 0;
                else if (e.key === "End") nextIndex = tabKeys.length - 1;
                const nextTab = tabKeys[nextIndex];
                activateTab(nextTab);
                document.getElementById(`tab-${nextTab}`)?.focus();
              }}
            >
            {visibleTabs.map((tab) => (
              <button
                key={tab.key}
                id={`tab-${tab.key}`}
                role="tab"
                aria-selected={activeTab === tab.key}
                aria-controls={`panel-${tab.key}`}
                tabIndex={activeTab === tab.key ? 0 : -1}
                onClick={() => activateTab(tab.key)}
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
          </div>

          {/* Content */}
          <div
            role="tabpanel"
            id={`panel-${activeTab}`}
            aria-labelledby={`tab-${activeTab}`}
            tabIndex={0}
            className="flex-1 overflow-y-auto p-6"
          >
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
                  <div className="text-sm text-green-700 dark:text-green-400 bg-green-50 dark:bg-green-950/40 p-3 rounded-md mb-4">
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
                              maxLength={255}
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
                            <p className="text-xs text-muted-foreground">
                              Must include uppercase, lowercase, and a number
                            </p>
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

                    {/* Notifications */}
                    <NotificationsCard />

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
                          <div className="rounded-lg bg-green-50 dark:bg-green-950/40 border border-green-200 dark:border-green-800/40 p-4 space-y-1">
                            <p className="text-sm text-green-800 dark:text-green-400">
                              Imported <strong>{syncResult.sent_count}</strong> sent and{" "}
                              <strong>{syncResult.received_count}</strong> received emails
                            </p>
                            <p className="text-sm text-green-800 dark:text-green-400">
                              Created <strong>{syncResult.thread_count}</strong> threads
                            </p>
                            <p className="text-sm text-green-800 dark:text-green-400">
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
                        ) : billingError ? (
                          <div className="flex flex-col items-center gap-2 py-4 text-sm">
                            <span className="text-destructive">{billingError}</span>
                            <Button variant="outline" size="sm" onClick={loadBilling}>Retry</Button>
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
                          <div className="space-y-2">
                            <p className="text-sm text-muted-foreground">
                              Unable to load billing information.
                            </p>
                            <Button variant="outline" size="sm" onClick={() => {
                              setBillingLoading(true);
                              api.get<BillingInfo>("/api/billing").then(setBillingInfo).catch(() => { toast.error("Operation failed"); }).finally(() => setBillingLoading(false));
                            }}>
                              <RotateCw className="h-3 w-3 mr-1" /> Retry
                            </Button>
                          </div>
                        )}
                      </CardContent>
                      <CardFooter className="flex-col items-start gap-2">
                        {billingInfo?.plan === "pro" || (billingInfo?.plan === "cancelled" && billingInfo?.subscription) ? (
                          <Button onClick={handleManageBilling}>
                            Manage subscription
                          </Button>
                        ) : (
                          <>
                            <Button onClick={handleCheckout} disabled={user?.role !== "admin" || checkoutLoading}>
                              {checkoutLoading && <Spinner className="mr-2 h-3 w-3" />}
                              {checkoutLoading ? "Redirecting..." : "Upgrade to Pro"}
                            </Button>
                            {user?.role !== "admin" && (
                              <p className="text-xs text-muted-foreground">
                                Ask your workspace admin to upgrade the plan.
                              </p>
                            )}
                          </>
                        )}
                      </CardFooter>
                    </Card>
                  </div>
                )}

                {activeTab === "domains" && (
                  <div className="space-y-6">
                    {/* Add Domain */}
                    {isAdmin && (
                      <Card>
                        <CardHeader>
                          <CardTitle>Add Domain</CardTitle>
                          <CardDescription>Add a new domain via Resend</CardDescription>
                        </CardHeader>
                        <form onSubmit={handleAddDomain}>
                          <CardContent className="space-y-4">
                            <div className="flex items-center gap-2">
                              <Input
                                value={newDomainName}
                                onChange={(e) => setNewDomainName(e.target.value)}
                                placeholder="example.com"
                                required
                                maxLength={253}
                              />
                              <Button disabled={addingDomain} type="submit">
                                {addingDomain ? <Spinner className="mr-2" /> : null}
                                Add
                              </Button>
                            </div>
                          </CardContent>
                        </form>
                      </Card>
                    )}

                    {/* Domain list */}
                    <Card>
                      <CardHeader>
                        <CardTitle>Domains</CardTitle>
                        <CardDescription>
                          Manage domains, configure DNS, and control sidebar visibility.
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
                        {allDomains.map((d, idx) => {
                          const active = visibleIds.has(d.id);
                          const dnsRecords = Array.isArray(d.dns_records) ? d.dns_records as { type: string; name: string; value: string; status?: string }[] : [];
                          const needsVerify = d.status === "pending" || d.status === "not_started";
                          return (
                            <div key={d.id} className={`rounded-lg border p-3 space-y-2 transition-colors ${active ? "border-primary bg-primary/5" : "border-muted opacity-70"}`}>
                              <div className="flex items-center justify-between">
                                <div className="flex items-center gap-3">
                                  <button
                                    type="button"
                                    onClick={() => {
                                      if (!isAdmin) return;
                                      setVisibleIds((prev) => {
                                        const next = new Set(prev);
                                        if (next.has(d.id)) next.delete(d.id);
                                        else next.add(d.id);
                                        return next;
                                      });
                                    }}
                                    disabled={!isAdmin}
                                    className="shrink-0"
                                  >
                                    <div className={`h-4 w-4 rounded border flex items-center justify-center transition-colors ${active ? "bg-primary border-primary" : "border-muted-foreground"}`}>
                                      {active && <Check className="h-3 w-3 text-primary-foreground" />}
                                    </div>
                                  </button>
                                  <span className="font-medium text-sm">{d.domain}</span>
                                  <Badge variant={d.status === "active" ? "default" : d.status === "verified" ? "secondary" : "outline"}>
                                    {d.status}
                                  </Badge>
                                </div>
                                <div className="flex items-center gap-1">
                                  {isAdmin && idx > 0 && (
                                    <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => handleReorderDomain(d.id, "up")} title="Move up">
                                      <span className="text-xs">↑</span>
                                    </Button>
                                  )}
                                  {isAdmin && idx < allDomains.length - 1 && (
                                    <Button variant="ghost" size="icon" className="h-7 w-7" onClick={() => handleReorderDomain(d.id, "down")} title="Move down">
                                      <span className="text-xs">↓</span>
                                    </Button>
                                  )}
                                  {isAdmin && needsVerify && (
                                    <Button variant="outline" size="sm" className="h-7 text-xs" onClick={() => handleVerifyDomain(d.id)} disabled={verifyingDomain === d.id}>
                                      {verifyingDomain === d.id ? <Spinner className="mr-1 h-3 w-3" /> : null}
                                      Verify
                                    </Button>
                                  )}
                                  {isAdmin && (
                                    <Button variant="ghost" size="icon" className="h-7 w-7 text-destructive hover:text-destructive" onClick={() => handleDeleteDomain(d.id)} disabled={deletingDomain === d.id}>
                                      <Trash2 className="h-3.5 w-3.5" />
                                    </Button>
                                  )}
                                </div>
                              </div>
                              {/* DNS records for unverified domains */}
                              {needsVerify && dnsRecords.length > 0 && (
                                <div className="text-xs space-y-1 bg-muted/50 rounded p-2">
                                  <p className="font-medium text-muted-foreground">DNS Records to configure:</p>
                                  <div className="overflow-x-auto">
                                    <table className="w-full text-left">
                                      <thead>
                                        <tr className="text-muted-foreground">
                                          <th className="pr-3 py-0.5">Type</th>
                                          <th className="pr-3 py-0.5">Name</th>
                                          <th className="pr-3 py-0.5">Value</th>
                                          <th className="py-0.5">Status</th>
                                        </tr>
                                      </thead>
                                      <tbody>
                                        {dnsRecords.map((rec, ri) => (
                                          <tr key={ri}>
                                            <td className="pr-3 py-0.5 font-mono">{rec.type}</td>
                                            <td className="pr-3 py-0.5 font-mono truncate max-w-[120px]">{rec.name}</td>
                                            <td className="pr-3 py-0.5 font-mono truncate max-w-[200px]">{rec.value}</td>
                                            <td className="py-0.5">{rec.status || "—"}</td>
                                          </tr>
                                        ))}
                                      </tbody>
                                    </table>
                                  </div>
                                </div>
                              )}
                            </div>
                          );
                        })}
                        {allDomains.length === 0 && (
                          <p className="text-sm text-muted-foreground">
                            No domains found. Click Refresh to sync from Resend.
                          </p>
                        )}
                      </CardContent>
                      <CardFooter className="flex gap-2">
                        <Button
                          onClick={handleSaveDomainVisibility}
                          disabled={savingDomains || user?.role !== "admin" || visibleIds.size === 0}
                        >
                          {savingDomains ? <Spinner className="mr-2" /> : null}
                          Save Visibility
                        </Button>
                        {isAdmin && (
                          <Button
                            variant="outline"
                            onClick={handleReregisterWebhook}
                            disabled={reregisteringWebhook}
                          >
                            {reregisteringWebhook ? <Spinner className="mr-2" /> : null}
                            Re-register Webhook
                          </Button>
                        )}
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
                                maxLength={254}
                              />
                            </div>
                            <div className="space-y-2">
                              <label className="text-sm font-medium">Name</label>
                              <Input
                                value={inviteName}
                                onChange={(e) => setInviteName(e.target.value)}
                                placeholder="Optional"
                                maxLength={255}
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
                        ) : teamError ? (
                          <div className="flex flex-col items-center gap-2 py-4 text-sm">
                            <span className="text-destructive">{teamError}</span>
                            <Button variant="outline" size="sm" onClick={loadTeam}>Retry</Button>
                          </div>
                        ) : (
                          <>
                          <div className="divide-y">
                            {orgUsers.filter((u) => u.status !== "disabled").map((u) => (
                              <div
                                key={u.id}
                                className="flex items-center justify-between py-3"
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
                                  {/* Role change dropdown — not for owner or self */}
                                  {u.status === "active" && u.id !== user?.id && u.role !== "owner" && (
                                    <select
                                      value={u.role}
                                      onChange={(e) => handleChangeRole(u.id, e.target.value)}
                                      className="h-7 text-xs rounded border border-input bg-transparent px-1.5 focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                                    >
                                      <option value="member">Member</option>
                                      <option value="admin">Admin</option>
                                    </select>
                                  )}
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
                                  {u.id !== user?.id && (
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={() => handleDisableUser(u.id)}
                                      className="text-destructive hover:text-destructive"
                                    >
                                      <UserX className="h-3 w-3 mr-1" />
                                      {u.status === "invited" || u.status === "placeholder" ? "Revoke" : "Disable"}
                                    </Button>
                                  )}
                                </div>
                              </div>
                            ))}
                          </div>
                          {orgUsers.some((u) => u.status === "disabled") && (
                            <details className="mt-4">
                              <summary className="cursor-pointer text-sm text-muted-foreground hover:text-foreground">
                                Disabled users ({orgUsers.filter((u) => u.status === "disabled").length})
                              </summary>
                              <div className="divide-y mt-2">
                                {orgUsers.filter((u) => u.status === "disabled").map((u) => (
                                  <div
                                    key={u.id}
                                    className="flex items-center justify-between py-3 opacity-50"
                                  >
                                    <div className="space-y-1">
                                      <div className="flex items-center gap-2">
                                        <span className="text-sm font-medium">
                                          {u.name || u.email}
                                        </span>
                                        <Badge variant="destructive" className="text-xs">
                                          disabled
                                        </Badge>
                                      </div>
                                      {u.name && (
                                        <p className="text-xs text-muted-foreground">{u.email}</p>
                                      )}
                                    </div>
                                    <Button
                                      variant="outline"
                                      size="sm"
                                      onClick={() => handleEnableUser(u.id)}
                                    >
                                      Enable
                                    </Button>
                                  </div>
                                ))}
                              </div>
                            </details>
                          )}
                          </>
                        )}
                      </CardContent>
                    </Card>
                  </div>
                )}

                {activeTab === "aliases" && !isAdmin && (
                  <div className="space-y-6">
                    <Card>
                      <CardHeader>
                        <CardTitle>My Aliases</CardTitle>
                        <CardDescription>Email aliases assigned to your account</CardDescription>
                      </CardHeader>
                      <CardContent>
                        {myAliasesLoading ? (
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> Loading aliases...
                          </div>
                        ) : myAliases.length === 0 ? (
                          <p className="text-sm text-muted-foreground">No aliases assigned to you yet. Ask your admin to add you.</p>
                        ) : (
                          <div className="divide-y">
                            {myAliases.map((a) => (
                              <div key={a.id} className="flex items-center justify-between py-3">
                                <div className="space-y-0.5">
                                  <span className="text-sm font-medium">{a.address}</span>
                                  {a.name && <p className="text-xs text-muted-foreground">{a.name}</p>}
                                </div>
                                <div className="flex items-center gap-2">
                                  {a.is_default && <Badge variant="default" className="text-xs">Default</Badge>}
                                  {a.can_send_as && <Badge variant="secondary" className="text-xs">Can send</Badge>}
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
                                  onChange={(e) => setNewAliasLocal(e.target.value.replace(/[^a-zA-Z0-9._%+-]/g, ""))}
                                  placeholder="hello"
                                  required
                                  maxLength={64}
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
                              maxLength={255}
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
                        ) : aliasError ? (
                          <div className="flex flex-col items-center gap-2 py-4 text-sm">
                            <span className="text-destructive">{aliasError}</span>
                            <Button variant="outline" size="sm" onClick={loadAliases}>Retry</Button>
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
                                              {editingAliasId === alias.id ? (
                                                <form
                                                  className="flex items-center gap-1"
                                                  onSubmit={(ev) => {
                                                    ev.preventDefault();
                                                    handleUpdateAliasName(alias.id, editingAliasName);
                                                  }}
                                                >
                                                  <Input
                                                    className="h-6 text-xs w-28 px-1"
                                                    value={editingAliasName}
                                                    onChange={(ev) => setEditingAliasName(ev.target.value)}
                                                    placeholder="Display name"
                                                    autoFocus
                                                    maxLength={255}
                                                    onBlur={() => handleUpdateAliasName(alias.id, editingAliasName)}
                                                    onKeyDown={(ev) => {
                                                      if (ev.key === "Escape") setEditingAliasId(null);
                                                    }}
                                                  />
                                                </form>
                                              ) : (
                                                <button
                                                  className="flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"
                                                  onClick={() => {
                                                    setEditingAliasId(alias.id);
                                                    setEditingAliasName(alias.name || "");
                                                  }}
                                                >
                                                  {alias.name ? (
                                                    <span>({alias.name})</span>
                                                  ) : (
                                                    <span className="italic">Add name</span>
                                                  )}
                                                  <Pencil className="h-3 w-3" />
                                                </button>
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

                {activeTab === "labels" && (
                  <div className="space-y-6">
                    <Card>
                      <CardHeader>
                        <CardTitle>Custom Labels</CardTitle>
                        <CardDescription>Create labels to organize your email threads</CardDescription>
                      </CardHeader>
                      <form onSubmit={handleCreateLabel}>
                        <CardContent className="space-y-4">
                          <div className="flex items-center gap-2">
                            <Input
                              value={newLabelName}
                              onChange={(e) => setNewLabelName(e.target.value)}
                              placeholder="Label name"
                              required
                              maxLength={100}
                            />
                            <Button disabled={creatingLabel} type="submit">
                              {creatingLabel ? <Spinner className="mr-2" /> : null}
                              Create
                            </Button>
                          </div>
                        </CardContent>
                      </form>
                    </Card>

                    <Card>
                      <CardHeader>
                        <CardTitle>Labels</CardTitle>
                        <CardDescription>{customLabels.length} custom label{customLabels.length !== 1 ? "s" : ""}</CardDescription>
                      </CardHeader>
                      <CardContent>
                        {labelsLoading ? (
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> Loading labels...
                          </div>
                        ) : labelsError ? (
                          <div className="flex flex-col items-center gap-2 py-4 text-sm">
                            <span className="text-destructive">{labelsError}</span>
                            <Button variant="outline" size="sm" onClick={loadLabels}>Retry</Button>
                          </div>
                        ) : customLabels.length === 0 ? (
                          <p className="text-sm text-muted-foreground">No custom labels yet.</p>
                        ) : (
                          <div className="divide-y">
                            {customLabels.map((label) => (
                              <div key={label.id} className="flex items-center justify-between py-2.5">
                                {editingLabelId === label.id ? (
                                  <form
                                    className="flex items-center gap-2 flex-1"
                                    onSubmit={(ev) => {
                                      ev.preventDefault();
                                      handleRenameLabel(label.id, editingLabelName);
                                    }}
                                  >
                                    <Input
                                      className="h-7 text-sm flex-1"
                                      value={editingLabelName}
                                      onChange={(ev) => setEditingLabelName(ev.target.value)}
                                      autoFocus
                                      maxLength={100}
                                      onBlur={() => handleRenameLabel(label.id, editingLabelName)}
                                      onKeyDown={(ev) => { if (ev.key === "Escape") setEditingLabelId(null); }}
                                    />
                                  </form>
                                ) : (
                                  <div className="flex items-center gap-2">
                                    <Tag className="h-3.5 w-3.5 text-muted-foreground" />
                                    <span className="text-sm">{label.name}</span>
                                  </div>
                                )}
                                <div className="flex items-center gap-1">
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-7 w-7"
                                    onClick={() => { setEditingLabelId(label.id); setEditingLabelName(label.name); }}
                                  >
                                    <Pencil className="h-3 w-3" />
                                  </Button>
                                  <Button
                                    variant="ghost"
                                    size="icon"
                                    className="h-7 w-7 text-destructive hover:text-destructive"
                                    onClick={() => handleDeleteLabel(label.id)}
                                  >
                                    <Trash2 className="h-3.5 w-3.5" />
                                  </Button>
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </CardContent>
                    </Card>
                  </div>
                )}

                {activeTab === "organization" && isAdmin && (
                  <div className="space-y-6">
                    <Card>
                      <CardHeader>
                        <CardTitle>Organization</CardTitle>
                        <CardDescription>Manage your organization settings</CardDescription>
                      </CardHeader>
                      {orgLoading ? (
                        <CardContent>
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> Loading...
                          </div>
                        </CardContent>
                      ) : (
                        <form onSubmit={handleSaveOrgSettings}>
                          <CardContent className="space-y-4">
                            <div className="space-y-2">
                              <label className="text-sm font-medium">Organization Name</label>
                              <Input
                                value={orgName}
                                onChange={(e) => setOrgName(e.target.value)}
                                placeholder="My Organization"
                                maxLength={255}
                              />
                            </div>
                            <div className="space-y-2">
                              <label className="text-sm font-medium">Resend API Key</label>
                              <Input
                                type="password"
                                value={orgResendKey}
                                onChange={(e) => setOrgResendKey(e.target.value)}
                                placeholder="re_..."
                              />
                              <p className="text-xs text-muted-foreground">
                                Enter a new key to replace the existing one. Leave unchanged to keep current key.
                              </p>
                            </div>
                            <div className="space-y-2">
                              <label className="text-sm font-medium">Resend API Rate Limit</label>
                              <div className="flex items-center gap-2">
                                <Input
                                  type="number"
                                  min={1}
                                  max={100}
                                  value={orgResendRPS}
                                  onChange={(e) => setOrgResendRPS(parseInt(e.target.value) || 2)}
                                  className="w-24"
                                />
                                <span className="text-sm text-muted-foreground">requests/second</span>
                              </div>
                              <p className="text-xs text-muted-foreground">
                                Default is 2 (Resend&apos;s standard limit). Contact Resend to request a higher limit before changing this value.
                              </p>
                            </div>
                          </CardContent>
                          <CardFooter>
                            <Button disabled={savingOrg}>
                              {savingOrg ? <Spinner className="mr-2" /> : null}
                              Save
                            </Button>
                          </CardFooter>
                        </form>
                      )}
                    </Card>

                    {/* Danger zone */}
                    {user?.is_owner && (
                      <Card className="border-destructive">
                        <CardHeader>
                          <CardTitle className="text-destructive">Danger Zone</CardTitle>
                          <CardDescription>
                            Permanently delete this organization. All users will be disabled and domains removed.
                          </CardDescription>
                        </CardHeader>
                        <CardContent>
                          {!showDeleteConfirm ? (
                            <Button variant="destructive" onClick={() => setShowDeleteConfirm(true)}>
                              Delete Organization
                            </Button>
                          ) : (
                            <div className="space-y-3 text-sm">
                              <div className="bg-destructive/10 p-3 rounded-md space-y-1">
                                <p className="font-medium text-destructive">This will immediately:</p>
                                <ul className="list-disc list-inside text-muted-foreground space-y-0.5">
                                  <li>Cancel your subscription</li>
                                  <li>Enter a 7-day grace period, then disable all access</li>
                                  <li>Cancel all pending outbound emails</li>
                                  <li>Disable all user accounts</li>
                                </ul>
                              </div>
                              <div className="space-y-1.5">
                                <label className="text-sm font-medium">Type DELETE to confirm</label>
                                <Input
                                  value={deleteConfirmText}
                                  onChange={(e) => setDeleteConfirmText(e.target.value)}
                                  placeholder="DELETE"
                                  className="font-mono"
                                />
                              </div>
                              <div className="flex gap-2">
                                <Button
                                  variant="destructive"
                                  disabled={deleteConfirmText !== "DELETE"}
                                  onClick={handleDeleteOrg}
                                >
                                  Permanently Delete
                                </Button>
                                <Button variant="outline" onClick={() => { setShowDeleteConfirm(false); setDeleteConfirmText(""); }}>
                                  Cancel
                                </Button>
                              </div>
                            </div>
                          )}
                        </CardContent>
                      </Card>
                    )}
                  </div>
                )}

                {activeTab === "system" && user?.is_owner && (
                  <div className="space-y-6">
                    <Card>
                      <CardHeader>
                        <CardTitle>System Email</CardTitle>
                        <CardDescription>
                          Configure the &quot;from&quot; address for password resets, invitations, and other system emails
                        </CardDescription>
                      </CardHeader>
                      {systemLoading ? (
                        <CardContent>
                          <div className="flex items-center gap-2 text-sm text-muted-foreground">
                            <Spinner /> Loading...
                          </div>
                        </CardContent>
                      ) : (
                        <form onSubmit={handleSaveSystemEmail}>
                          <CardContent className="space-y-4">
                            <div className="space-y-2">
                              <label className="text-sm font-medium">From address</label>
                              <Input
                                value={systemFromAddress}
                                onChange={(e) => setSystemFromAddress(e.target.value)}
                                placeholder="noreply@yourdomain.com"
                                maxLength={254}
                              />
                              <p className="text-xs text-muted-foreground">
                                Must be a verified domain in your Resend account
                              </p>
                            </div>
                            <div className="space-y-2">
                              <label className="text-sm font-medium">From name</label>
                              <Input
                                value={systemFromName}
                                onChange={(e) => setSystemFromName(e.target.value)}
                                placeholder="Inboxes"
                                maxLength={255}
                              />
                            </div>
                            {systemFromAddress && (
                              <div className="rounded-md bg-muted p-3 text-sm">
                                <span className="text-muted-foreground">Preview: </span>
                                <span className="font-medium">
                                  {systemFromName
                                    ? `${systemFromName} <${systemFromAddress}>`
                                    : systemFromAddress}
                                </span>
                              </div>
                            )}
                          </CardContent>
                          <CardFooter className="flex gap-2">
                            <Button disabled={savingSystem}>
                              {savingSystem ? <Spinner className="mr-2" /> : null}
                              Save
                            </Button>
                            <Button
                              type="button"
                              variant="outline"
                              disabled={sendingTest || !systemFromAddress}
                              onClick={handleSendTestEmail}
                            >
                              {sendingTest ? <Spinner className="mr-2" /> : null}
                              Send Test Email
                            </Button>
                          </CardFooter>
                        </form>
                      )}
                    </Card>
                  </div>
                )}

                {activeTab === "jobs" && isAdmin && <JobsPanel />}
              </>
            )}
          </div>
        </div>
      </DialogContent>
    </Dialog>
  );
}
