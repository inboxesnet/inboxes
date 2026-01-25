"use client";

import { useState, useEffect, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Loader2,
  Plus,
  ChevronDown,
  ChevronRight,
  Trash2,
  UserPlus,
  X,
} from "lucide-react";
import { useToast } from "@/components/ui/toast";

interface AliasUser {
  id: string;
  name: string;
  email: string;
  can_send_as: boolean;
}

interface Alias {
  id: string;
  address: string;
  name: string;
  created_at: string;
  users: AliasUser[];
}

interface OrgUser {
  id: string;
  name: string;
  email: string;
}

export default function AliasesSettingsPage() {
  const [aliases, setAliases] = useState<Alias[]>([]);
  const [orgUsers, setOrgUsers] = useState<OrgUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [expandedAliasId, setExpandedAliasId] = useState<string | null>(null);
  const [deleteConfirmId, setDeleteConfirmId] = useState<string | null>(null);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const [orgDomain, setOrgDomain] = useState<string | null>(null);
  const { addToast } = useToast();

  const fetchData = useCallback(async () => {
    try {
      const [aliasesRes, usersRes, domainRes] = await Promise.all([
        fetch("/api/aliases"),
        fetch("/api/users"),
        fetch("/api/domains"),
      ]);

      if (aliasesRes.status === 403 || usersRes.status === 403) {
        setError("You do not have permission to access this page");
        return;
      }

      if (!aliasesRes.ok || !usersRes.ok) {
        throw new Error("Failed to load data");
      }

      const aliasesData = await aliasesRes.json();
      const usersData = await usersRes.json();
      const domainData = await domainRes.json();

      setAliases(aliasesData.aliases);
      setOrgUsers(
        usersData.users.map((u: { id: string; name: string; email: string }) => ({
          id: u.id,
          name: u.name,
          email: u.email,
        }))
      );
      setOrgDomain(domainData.domain?.domain || null);
    } catch {
      setError("Failed to load aliases");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchData();
  }, [fetchData]);

  function toggleExpand(aliasId: string) {
    setExpandedAliasId(expandedAliasId === aliasId ? null : aliasId);
  }

  async function handleToggleCanSendAs(aliasId: string, userId: string, currentValue: boolean) {
    setActionLoading(`${aliasId}-${userId}-toggle`);
    try {
      const alias = aliases.find((a) => a.id === aliasId);
      if (!alias) return;

      const updatedUsers = alias.users.map((u) =>
        u.id === userId ? { user_id: u.id, can_send_as: !currentValue } : { user_id: u.id, can_send_as: u.can_send_as }
      );

      const res = await fetch(`/api/aliases/${aliasId}`, {
        method: "PATCH",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ users: updatedUsers }),
      });

      if (!res.ok) {
        const data = await res.json();
        addToast(data.error || "Failed to update permission", "destructive");
        return;
      }

      const data = await res.json();
      setAliases((prev) =>
        prev.map((a) => (a.id === aliasId ? data.alias : a))
      );
    } catch {
      addToast("Failed to update permission", "destructive");
    } finally {
      setActionLoading(null);
    }
  }

  async function handleRemoveUser(aliasId: string, userId: string) {
    setActionLoading(`${aliasId}-${userId}-remove`);
    try {
      const res = await fetch(`/api/aliases/${aliasId}/users/${userId}`, {
        method: "DELETE",
      });

      if (!res.ok) {
        const data = await res.json();
        addToast(data.error || "Failed to remove user", "destructive");
        return;
      }

      setAliases((prev) =>
        prev.map((a) =>
          a.id === aliasId
            ? { ...a, users: a.users.filter((u) => u.id !== userId) }
            : a
        )
      );
      addToast("User removed from alias", "success");
    } catch {
      addToast("Failed to remove user", "destructive");
    } finally {
      setActionLoading(null);
    }
  }

  async function handleAddUser(aliasId: string, userId: string) {
    setActionLoading(`${aliasId}-add`);
    try {
      const res = await fetch(`/api/aliases/${aliasId}/users`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ user_id: userId, can_send_as: true }),
      });

      if (!res.ok) {
        const data = await res.json();
        addToast(data.error || "Failed to add user", "destructive");
        return;
      }

      const data = await res.json();
      setAliases((prev) =>
        prev.map((a) =>
          a.id === aliasId
            ? { ...a, users: [...a.users, data.alias_user] }
            : a
        )
      );
      addToast("User added to alias", "success");
    } catch {
      addToast("Failed to add user", "destructive");
    } finally {
      setActionLoading(null);
    }
  }

  async function handleDeleteAlias(aliasId: string) {
    setActionLoading(`${aliasId}-delete`);
    try {
      const res = await fetch(`/api/aliases/${aliasId}`, {
        method: "DELETE",
      });

      if (!res.ok) {
        const data = await res.json();
        addToast(data.error || "Failed to delete alias", "destructive");
        return;
      }

      setAliases((prev) => prev.filter((a) => a.id !== aliasId));
      setDeleteConfirmId(null);
      addToast("Alias deleted", "success");
    } catch {
      addToast("Failed to delete alias", "destructive");
    } finally {
      setActionLoading(null);
    }
  }

  function getAvailableUsersForAlias(alias: Alias) {
    const assignedUserIds = new Set(alias.users.map((u) => u.id));
    return orgUsers.filter((u) => !assignedUserIds.has(u.id));
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Loader2 className="h-6 w-6 animate-spin text-muted-foreground" />
      </div>
    );
  }

  if (error) {
    return (
      <div className="rounded-md bg-destructive/10 px-4 py-3 text-sm text-destructive">
        {error}
      </div>
    );
  }

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-2xl font-bold">Aliases</h2>
          <p className="mt-1 text-muted-foreground">
            Create and manage email aliases (distribution lists).
          </p>
        </div>
        <Button
          onClick={() => setCreateDialogOpen(true)}
          disabled={!orgDomain}
        >
          <Plus className="mr-2 h-4 w-4" />
          Create Alias
        </Button>
      </div>

      {!orgDomain && (
        <div className="rounded-md bg-warning/10 px-4 py-3 text-sm text-warning-foreground border border-warning/20">
          You need to set up a verified domain before creating aliases.
        </div>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Email Aliases</CardTitle>
          <CardDescription>
            Aliases let multiple team members receive emails sent to a shared address.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {aliases.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No aliases yet. Create one to get started.
            </p>
          ) : (
            <div className="space-y-2">
              {aliases.map((alias) => (
                <div
                  key={alias.id}
                  className="rounded-lg border bg-card"
                >
                  {/* Alias header row */}
                  <div
                    className="flex items-center justify-between p-4 cursor-pointer hover:bg-muted/50"
                    onClick={() => toggleExpand(alias.id)}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(e) => {
                      if (e.key === "Enter" || e.key === " ") {
                        toggleExpand(alias.id);
                      }
                    }}
                  >
                    <div className="flex items-center gap-3">
                      {expandedAliasId === alias.id ? (
                        <ChevronDown className="h-4 w-4 text-muted-foreground" />
                      ) : (
                        <ChevronRight className="h-4 w-4 text-muted-foreground" />
                      )}
                      <div>
                        <div className="font-medium">{alias.address}</div>
                        <div className="text-sm text-muted-foreground">
                          {alias.name}
                        </div>
                      </div>
                    </div>
                    <div className="flex items-center gap-4">
                      <span className="text-sm text-muted-foreground">
                        {alias.users.length} user{alias.users.length !== 1 ? "s" : ""}
                      </span>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        onClick={(e) => {
                          e.stopPropagation();
                          setDeleteConfirmId(alias.id);
                        }}
                      >
                        <Trash2 className="h-4 w-4" />
                      </Button>
                    </div>
                  </div>

                  {/* Expanded detail view */}
                  {expandedAliasId === alias.id && (
                    <div className="border-t p-4 space-y-4">
                      {/* Assigned users list */}
                      <div className="space-y-2">
                        <div className="text-sm font-medium">Assigned Users</div>
                        {alias.users.length === 0 ? (
                          <p className="text-sm text-muted-foreground">
                            No users assigned to this alias.
                          </p>
                        ) : (
                          <div className="space-y-2">
                            {alias.users.map((user) => (
                              <div
                                key={user.id}
                                className="flex items-center justify-between rounded-md border p-3"
                              >
                                <div>
                                  <div className="font-medium">{user.name}</div>
                                  <div className="text-sm text-muted-foreground">
                                    {user.email}
                                  </div>
                                </div>
                                <div className="flex items-center gap-3">
                                  <label className="flex items-center gap-2 text-sm">
                                    <input
                                      type="checkbox"
                                      checked={user.can_send_as}
                                      onChange={() =>
                                        handleToggleCanSendAs(
                                          alias.id,
                                          user.id,
                                          user.can_send_as
                                        )
                                      }
                                      disabled={
                                        actionLoading ===
                                        `${alias.id}-${user.id}-toggle`
                                      }
                                      className="h-4 w-4 rounded border-gray-300"
                                    />
                                    Can send as
                                  </label>
                                  <Button
                                    variant="ghost"
                                    size="sm"
                                    onClick={() =>
                                      handleRemoveUser(alias.id, user.id)
                                    }
                                    disabled={
                                      actionLoading ===
                                      `${alias.id}-${user.id}-remove`
                                    }
                                  >
                                    {actionLoading ===
                                    `${alias.id}-${user.id}-remove` ? (
                                      <Loader2 className="h-4 w-4 animate-spin" />
                                    ) : (
                                      <X className="h-4 w-4" />
                                    )}
                                  </Button>
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>

                      {/* Add user dropdown */}
                      <AddUserDropdown
                        alias={alias}
                        availableUsers={getAvailableUsersForAlias(alias)}
                        onAddUser={handleAddUser}
                        isLoading={actionLoading === `${alias.id}-add`}
                      />
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      {/* Create Alias Dialog */}
      <CreateAliasDialog
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
        domain={orgDomain}
        onSuccess={(newAlias: Alias) => {
          setAliases((prev) => [newAlias, ...prev]);
          addToast("Alias created successfully", "success");
        }}
      />

      {/* Delete Confirmation Dialog */}
      <Dialog
        open={deleteConfirmId !== null}
        onOpenChange={() => setDeleteConfirmId(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Delete Alias</DialogTitle>
          </DialogHeader>
          <p className="text-sm text-muted-foreground">
            Are you sure you want to delete this alias? This action cannot be undone.
            Emails sent to this address will no longer be delivered.
          </p>
          <div className="flex justify-end gap-3 mt-4">
            <Button
              variant="outline"
              onClick={() => setDeleteConfirmId(null)}
              disabled={actionLoading === `${deleteConfirmId}-delete`}
            >
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteConfirmId && handleDeleteAlias(deleteConfirmId)}
              disabled={actionLoading === `${deleteConfirmId}-delete`}
            >
              {actionLoading === `${deleteConfirmId}-delete` && (
                <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              )}
              Delete
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  );
}

function AddUserDropdown({
  alias,
  availableUsers,
  onAddUser,
  isLoading,
}: {
  alias: Alias;
  availableUsers: OrgUser[];
  onAddUser: (aliasId: string, userId: string) => void;
  isLoading: boolean;
}) {
  const [selectedUserId, setSelectedUserId] = useState("");

  if (availableUsers.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        All team members are already assigned to this alias.
      </p>
    );
  }

  return (
    <div className="flex items-center gap-2">
      <select
        value={selectedUserId}
        onChange={(e) => setSelectedUserId(e.target.value)}
        disabled={isLoading}
        className="flex h-10 w-full max-w-xs rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
      >
        <option value="">Select a user...</option>
        {availableUsers.map((user) => (
          <option key={user.id} value={user.id}>
            {user.name} ({user.email})
          </option>
        ))}
      </select>
      <Button
        onClick={() => {
          if (selectedUserId) {
            onAddUser(alias.id, selectedUserId);
            setSelectedUserId("");
          }
        }}
        disabled={!selectedUserId || isLoading}
        size="sm"
      >
        {isLoading ? (
          <Loader2 className="h-4 w-4 animate-spin" />
        ) : (
          <UserPlus className="h-4 w-4" />
        )}
        <span className="ml-2">Add User</span>
      </Button>
    </div>
  );
}

function CreateAliasDialog({
  open,
  onOpenChange,
  domain,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  domain: string | null;
  onSuccess: (alias: Alias) => void;
}) {
  const [addressPrefix, setAddressPrefix] = useState("");
  const [name, setName] = useState("");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  function resetForm() {
    setAddressPrefix("");
    setName("");
    setError("");
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    if (!domain) {
      setError("No domain configured");
      return;
    }

    const address = `${addressPrefix.trim().toLowerCase()}@${domain}`;

    setLoading(true);
    try {
      const res = await fetch("/api/aliases", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ address, name: name.trim() }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Failed to create alias");
        return;
      }

      onSuccess(data.alias);
      resetForm();
      onOpenChange(false);
    } catch {
      setError("Failed to create alias");
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create Alias</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          {error && (
            <div className="rounded-md bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="alias-address">Email Address</Label>
            <div className="flex items-center">
              <Input
                id="alias-address"
                type="text"
                placeholder="support"
                value={addressPrefix}
                onChange={(e) => setAddressPrefix(e.target.value)}
                disabled={loading}
                required
                className="rounded-r-none"
              />
              <span className="flex h-10 items-center rounded-r-md border border-l-0 border-input bg-muted px-3 text-sm text-muted-foreground">
                @{domain || "yourdomain.com"}
              </span>
            </div>
            <p className="text-xs text-muted-foreground">
              This will create an alias at {addressPrefix || "prefix"}@{domain || "yourdomain.com"}
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="alias-name">Display Name</Label>
            <Input
              id="alias-name"
              type="text"
              placeholder="Support Team"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={loading}
              required
            />
            <p className="text-xs text-muted-foreground">
              A friendly name for this alias
            </p>
          </div>

          <div className="flex justify-end gap-3 pt-4">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                resetForm();
                onOpenChange(false);
              }}
              disabled={loading}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={loading || !addressPrefix || !name}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Create Alias
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
