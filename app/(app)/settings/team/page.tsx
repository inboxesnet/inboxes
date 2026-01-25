"use client";

import { useState, useEffect, useCallback } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Loader2, Plus, RefreshCw, UserX } from "lucide-react";
import { useToast } from "@/components/ui/toast";

interface TeamUser {
  id: string;
  email: string;
  name: string;
  role: "admin" | "member";
  status: "invited" | "active" | "disabled";
  invite_expires_at: string | null;
  claimed_at: string | null;
  created_at: string;
}

export default function TeamSettingsPage() {
  const [users, setUsers] = useState<TeamUser[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [inviteDialogOpen, setInviteDialogOpen] = useState(false);
  const [actionLoading, setActionLoading] = useState<string | null>(null);
  const { addToast } = useToast();

  const fetchUsers = useCallback(async () => {
    try {
      const res = await fetch("/api/users");
      if (!res.ok) {
        if (res.status === 403) {
          setError("You do not have permission to access this page");
          return;
        }
        throw new Error("Failed to load users");
      }
      const data = await res.json();
      setUsers(data.users);
    } catch {
      setError("Failed to load team members");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    fetchUsers();
  }, [fetchUsers]);

  async function handleReinvite(userId: string) {
    setActionLoading(userId);
    try {
      const res = await fetch(`/api/users/${userId}/reinvite`, {
        method: "POST",
      });
      if (!res.ok) {
        const data = await res.json();
        addToast(data.error || "Failed to resend invite", "destructive");
        return;
      }
      const data = await res.json();
      // Update user in list
      setUsers((prev) =>
        prev.map((u) =>
          u.id === userId
            ? { ...u, invite_expires_at: data.invite_expires_at }
            : u
        )
      );
      addToast("Invite resent successfully", "success");
    } catch {
      addToast("Failed to resend invite", "destructive");
    } finally {
      setActionLoading(null);
    }
  }

  async function handleDisable(userId: string) {
    setActionLoading(userId);
    try {
      const res = await fetch(`/api/users/${userId}/disable`, {
        method: "PATCH",
      });
      if (!res.ok) {
        const data = await res.json();
        addToast(data.error || "Failed to disable user", "destructive");
        return;
      }
      const data = await res.json();
      // Update user in list
      setUsers((prev) =>
        prev.map((u) => (u.id === userId ? data.user : u))
      );
      addToast("User disabled successfully", "success");
    } catch {
      addToast("Failed to disable user", "destructive");
    } finally {
      setActionLoading(null);
    }
  }

  function getStatusBadge(status: "invited" | "active" | "disabled") {
    switch (status) {
      case "active":
        return <Badge variant="success">Active</Badge>;
      case "invited":
        return <Badge variant="warning">Invited</Badge>;
      case "disabled":
        return <Badge variant="destructive">Disabled</Badge>;
    }
  }

  function getRoleBadge(role: "admin" | "member") {
    return (
      <Badge variant={role === "admin" ? "default" : "secondary"}>
        {role === "admin" ? "Admin" : "Member"}
      </Badge>
    );
  }

  function formatDate(dateStr: string | null) {
    if (!dateStr) return "-";
    return new Date(dateStr).toLocaleDateString("en-US", {
      month: "short",
      day: "numeric",
      year: "numeric",
    });
  }

  function isInviteExpired(expiresAt: string | null) {
    if (!expiresAt) return false;
    return new Date(expiresAt) < new Date();
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
          <h2 className="text-2xl font-bold">Team Management</h2>
          <p className="mt-1 text-muted-foreground">
            Invite and manage team members.
          </p>
        </div>
        <Button onClick={() => setInviteDialogOpen(true)}>
          <Plus className="mr-2 h-4 w-4" />
          Invite User
        </Button>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Team Members</CardTitle>
          <CardDescription>
            All members of your organization.
          </CardDescription>
        </CardHeader>
        <CardContent>
          {users.length === 0 ? (
            <p className="text-sm text-muted-foreground">No team members yet.</p>
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b">
                    <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                      Name
                    </th>
                    <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                      Email
                    </th>
                    <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                      Role
                    </th>
                    <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                      Status
                    </th>
                    <th className="py-3 pr-4 text-left font-medium text-muted-foreground">
                      Joined
                    </th>
                    <th className="py-3 text-left font-medium text-muted-foreground">
                      Actions
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {users.map((member) => (
                    <tr key={member.id} className="border-b last:border-0">
                      <td className="py-3 pr-4 font-medium">{member.name}</td>
                      <td className="py-3 pr-4 text-muted-foreground">
                        {member.email}
                      </td>
                      <td className="py-3 pr-4">{getRoleBadge(member.role)}</td>
                      <td className="py-3 pr-4">
                        <div className="flex items-center gap-2">
                          {getStatusBadge(member.status)}
                          {member.status === "invited" &&
                            isInviteExpired(member.invite_expires_at) && (
                              <span className="text-xs text-destructive">
                                (expired)
                              </span>
                            )}
                        </div>
                      </td>
                      <td className="py-3 pr-4 text-muted-foreground">
                        {formatDate(member.claimed_at || member.created_at)}
                      </td>
                      <td className="py-3">
                        <div className="flex gap-2">
                          {member.status === "invited" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleReinvite(member.id)}
                              disabled={actionLoading === member.id}
                            >
                              {actionLoading === member.id ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <RefreshCw className="h-4 w-4" />
                              )}
                              <span className="ml-1">Reinvite</span>
                            </Button>
                          )}
                          {member.status === "active" && (
                            <Button
                              variant="ghost"
                              size="sm"
                              onClick={() => handleDisable(member.id)}
                              disabled={actionLoading === member.id}
                              className="text-destructive hover:text-destructive"
                            >
                              {actionLoading === member.id ? (
                                <Loader2 className="h-4 w-4 animate-spin" />
                              ) : (
                                <UserX className="h-4 w-4" />
                              )}
                              <span className="ml-1">Disable</span>
                            </Button>
                          )}
                        </div>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </CardContent>
      </Card>

      <InviteUserDialog
        open={inviteDialogOpen}
        onOpenChange={setInviteDialogOpen}
        onSuccess={(newUser: TeamUser) => {
          setUsers((prev) => [...prev, newUser]);
          addToast("User invited successfully", "success");
        }}
      />
    </div>
  );
}

function InviteUserDialog({
  open,
  onOpenChange,
  onSuccess,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onSuccess: (user: TeamUser) => void;
}) {
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [role, setRole] = useState<"admin" | "member">("member");
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  function resetForm() {
    setEmail("");
    setName("");
    setRole("member");
    setError("");
  }

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const res = await fetch("/api/users/invite", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ email, name, role }),
      });

      const data = await res.json();

      if (!res.ok) {
        setError(data.error || "Failed to invite user");
        return;
      }

      onSuccess(data);
      resetForm();
      onOpenChange(false);
    } catch {
      setError("Failed to invite user");
    } finally {
      setLoading(false);
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Invite Team Member</DialogTitle>
        </DialogHeader>

        <form onSubmit={handleSubmit} className="mt-4 space-y-4">
          {error && (
            <div className="rounded-md bg-destructive/10 px-4 py-3 text-sm text-destructive">
              {error}
            </div>
          )}

          <div className="space-y-2">
            <Label htmlFor="invite-email">Email</Label>
            <Input
              id="invite-email"
              type="email"
              placeholder="user@yourcompany.com"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              disabled={loading}
              required
            />
            <p className="text-xs text-muted-foreground">
              Must match your organization&apos;s domain
            </p>
          </div>

          <div className="space-y-2">
            <Label htmlFor="invite-name">Name</Label>
            <Input
              id="invite-name"
              type="text"
              placeholder="John Doe"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={loading}
              required
            />
          </div>

          <div className="space-y-2">
            <Label htmlFor="invite-role">Role</Label>
            <select
              id="invite-role"
              className="flex h-10 w-full rounded-md border border-input bg-background px-3 py-2 text-sm ring-offset-background focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2"
              value={role}
              onChange={(e) => setRole(e.target.value as "admin" | "member")}
              disabled={loading}
            >
              <option value="member">Member</option>
              <option value="admin">Admin</option>
            </select>
            <p className="text-xs text-muted-foreground">
              Admins can manage team members, domains, and aliases
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
            <Button type="submit" disabled={loading || !email || !name}>
              {loading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
              Send Invite
            </Button>
          </div>
        </form>
      </DialogContent>
    </Dialog>
  );
}
