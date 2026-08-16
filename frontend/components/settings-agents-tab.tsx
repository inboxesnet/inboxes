"use client";

// Settings → Agents: connect MCP agents (Claude Code, Codex, opencode),
// manage per-user agent credentials, and (admins) the org-level send switch.

import { useState, useEffect, useCallback } from "react";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { Copy, Check, Trash2 } from "lucide-react";
import { toast } from "sonner";

interface AgentKey {
  id: string;
  kind: "key" | "oauth";
  name: string;
  created_at: string;
  last_used_at: string | null;
  expires_at: string | null;
}

function apiBase(): string {
  return process.env.NEXT_PUBLIC_API_URL || (typeof window !== "undefined" ? window.location.origin : "");
}

function CopyBlock({ label, text }: { label: string; text: string }) {
  const [copied, setCopied] = useState(false);
  return (
    <div className="space-y-1">
      <div className="flex items-center justify-between">
        <span className="text-xs font-medium text-muted-foreground">{label}</span>
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-xs"
          onClick={() => {
            navigator.clipboard.writeText(text);
            setCopied(true);
            setTimeout(() => setCopied(false), 1500);
          }}
        >
          {copied ? <Check className="h-3 w-3" /> : <Copy className="h-3 w-3" />}
        </Button>
      </div>
      <pre className="text-xs bg-muted rounded-md p-2 overflow-x-auto whitespace-pre-wrap break-all">{text}</pre>
    </div>
  );
}

export function AgentsTab({ isAdmin }: { isAdmin: boolean }) {
  const [keys, setKeys] = useState<AgentKey[]>([]);
  const [loading, setLoading] = useState(true);
  const [creating, setCreating] = useState(false);
  const [newKeyName, setNewKeyName] = useState("");
  const [freshKey, setFreshKey] = useState<{ name: string; key: string } | null>(null);
  const [sendEnabled, setSendEnabled] = useState(false);
  const [savingToggle, setSavingToggle] = useState(false);

  const load = useCallback(async () => {
    try {
      const [keysRes, settings] = await Promise.all([
        api.get<{ keys: AgentKey[] }>("/api/agent-keys"),
        api.get<{ agent_send_enabled?: boolean }>("/api/orgs/settings"),
      ]);
      setKeys(keysRes.keys);
      setSendEnabled(Boolean(settings.agent_send_enabled));
    } catch {
      toast.error("Failed to load agent settings");
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  async function createKey() {
    setCreating(true);
    try {
      const res = await api.post<{ id: string; name: string; key: string }>("/api/agent-keys", {
        name: newKeyName || "API key",
      });
      setFreshKey({ name: res.name, key: res.key });
      setNewKeyName("");
      await load();
    } catch (err) {
      toast.error(err instanceof ApiError ? err.message : "Failed to create key");
    } finally {
      setCreating(false);
    }
  }

  async function revokeKey(id: string) {
    try {
      await api.delete(`/api/agent-keys/${id}`);
      setKeys((prev) => prev.filter((k) => k.id !== id));
      toast.success("Revoked");
    } catch {
      toast.error("Failed to revoke");
    }
  }

  async function toggleSend(value: boolean) {
    setSavingToggle(true);
    const prev = sendEnabled;
    setSendEnabled(value);
    try {
      await api.patch("/api/orgs/settings", { agent_send_enabled: value });
      toast.success(value ? "Agent send enabled" : "Agent send disabled");
    } catch {
      setSendEnabled(prev);
      toast.error("Failed to update setting");
    } finally {
      setSavingToggle(false);
    }
  }

  const base = apiBase();
  const mcpUrl = `${base}/mcp`;
  const keyPlaceholder = freshKey?.key || "<YOUR-KEY>";

  if (loading) {
    return (
      <div className="flex justify-center py-10">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="space-y-6">
      {isAdmin && (
        <Card>
          <CardHeader>
            <CardTitle>Agent send</CardTitle>
            <CardDescription>
              Off by default. When off, agents can read mail and create drafts, but
              only a human can send. When on, agents may send and schedule email as
              the user they are connected to.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <label className="flex items-center gap-3 cursor-pointer">
              <input
                type="checkbox"
                className="h-4 w-4 rounded border"
                checked={sendEnabled}
                disabled={savingToggle}
                onChange={(e) => toggleSend(e.target.checked)}
              />
              <span className="text-sm font-medium">
                Allow connected agents to send email
              </span>
            </label>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>API keys</CardTitle>
          <CardDescription>
            Keys act as you: they see your mail and carry your role. Each key is
            shown once at creation. OAuth connections agents made through the
            browser flow also appear here.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="flex items-center gap-2">
            <Input
              value={newKeyName}
              onChange={(e) => setNewKeyName(e.target.value)}
              placeholder="Key name, e.g. claude-code laptop"
              maxLength={100}
            />
            <Button onClick={createKey} disabled={creating}>
              {creating ? <Spinner className="mr-2" /> : null}
              Create
            </Button>
          </div>

          {freshKey && (
            <div className="rounded-md border border-amber-500/50 bg-amber-500/5 p-3 space-y-2">
              <p className="text-sm font-medium">
                Copy this key now — it is not shown again.
              </p>
              <CopyBlock label={freshKey.name} text={freshKey.key} />
            </div>
          )}

          {keys.length === 0 ? (
            <p className="text-sm text-muted-foreground">No agent credentials yet.</p>
          ) : (
            <div className="space-y-2">
              {keys.map((k) => (
                <div
                  key={k.id}
                  className="flex items-center justify-between rounded-lg border p-2"
                >
                  <div className="min-w-0">
                    <p className="text-sm font-medium truncate">
                      {k.name || (k.kind === "oauth" ? "OAuth connection" : "API key")}
                      <span className="ml-2 text-xs text-muted-foreground uppercase">{k.kind}</span>
                    </p>
                    <p className="text-xs text-muted-foreground">
                      Created {new Date(k.created_at).toLocaleDateString()}
                      {k.last_used_at
                        ? ` · last used ${new Date(k.last_used_at).toLocaleString()}`
                        : " · never used"}
                    </p>
                  </div>
                  <Button
                    variant="ghost"
                    size="sm"
                    className="text-destructive"
                    onClick={() => revokeKey(k.id)}
                  >
                    <Trash2 className="h-4 w-4" />
                  </Button>
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Connect an agent</CardTitle>
          <CardDescription>
            Point any MCP client at your Inboxes server. The OAuth browser flow
            works with clients that support it; an API key works everywhere.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <CopyBlock label="MCP server URL" text={mcpUrl} />
          <CopyBlock
            label="Claude Code (OAuth — approves in your browser)"
            text={`claude mcp add --transport http inboxes ${mcpUrl}`}
          />
          <CopyBlock
            label="Claude Code (API key)"
            text={`claude mcp add --transport http inboxes ${mcpUrl} --header "Authorization: Bearer ${keyPlaceholder}"`}
          />
          <CopyBlock
            label="Codex (~/.codex/config.toml)"
            text={`[mcp_servers.inboxes]\nurl = "${mcpUrl}"\nhttp_headers = { "Authorization" = "Bearer ${keyPlaceholder}" }`}
          />
          <CopyBlock
            label="opencode (opencode.json)"
            text={`{\n  "mcp": {\n    "inboxes": {\n      "type": "remote",\n      "url": "${mcpUrl}",\n      "headers": { "Authorization": "Bearer ${keyPlaceholder}" }\n    }\n  }\n}`}
          />
        </CardContent>
      </Card>
    </div>
  );
}
