"use client";

// OAuth consent page for MCP agent connections. An MCP client (Claude Code,
// Codex, etc.) sends the user here; the user approves and we redirect back
// to the client with an authorization code. The backend validates everything
// again server-side — this page is just the human checkpoint.

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";

export default function AuthorizePage() {
  return (
    <div className="min-h-screen flex items-center justify-center bg-background p-4">
      <div className="w-full max-w-md">
        <Suspense>
          <AuthorizeCard />
        </Suspense>
      </div>
    </div>
  );
}

function AuthorizeCard() {
  const router = useRouter();
  const params = useSearchParams();

  const clientId = params.get("client_id") || "";
  const redirectUri = params.get("redirect_uri") || "";
  const state = params.get("state") || "";
  const codeChallenge = params.get("code_challenge") || "";
  const codeChallengeMethod = params.get("code_challenge_method") || "";

  const [me, setMe] = useState<{ email: string } | null>(null);
  const [clientName, setClientName] = useState("");
  const [checking, setChecking] = useState(true);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    api
      .get<{ email: string }>("/api/users/me", { noLatch: true })
      .then((u) => setMe(u))
      .catch(() => {
        const here = window.location.pathname + window.location.search;
        router.replace("/login?next=" + encodeURIComponent(here));
      })
      .finally(() => setChecking(false));
  }, [router]);

  useEffect(() => {
    if (!clientId) return;
    api
      .get<{ name: string }>(`/api/oauth/client?client_id=${encodeURIComponent(clientId)}`)
      .then((c) => setClientName(c.name))
      .catch(() => setClientName(""));
  }, [clientId]);

  const paramsValid =
    clientId && redirectUri && codeChallenge && codeChallengeMethod === "S256";

  // Parse the redirect target so the user can see where the code will go. A
  // loopback address is a normal local CLI. A remote host is a red flag.
  let redirectHost = "";
  let redirectIsLoopback = false;
  try {
    const target = new URL(redirectUri);
    redirectHost = target.host || target.protocol;
    const h = target.hostname;
    redirectIsLoopback =
      h === "localhost" || h === "127.0.0.1" || h === "::1" || h === "[::1]";
  } catch {
    redirectHost = redirectUri;
  }

  async function approve() {
    setError("");
    setWorking(true);
    try {
      const res = await api.post<{ redirect_to: string }>("/api/oauth/approve", {
        client_id: clientId,
        redirect_uri: redirectUri,
        state,
        code_challenge: codeChallenge,
        code_challenge_method: codeChallengeMethod,
      });
      window.location.href = res.redirect_to;
    } catch (err) {
      setError(err instanceof Error ? err.message : "Approval failed");
      setWorking(false);
    }
  }

  function deny() {
    // Only bounce back to a loopback client. Never redirect the browser to a
    // remote URL an unknown client supplied — that is an open redirect.
    if (redirectIsLoopback) {
      try {
        const u = new URL(redirectUri);
        u.searchParams.set("error", "access_denied");
        if (state) u.searchParams.set("state", state);
        window.location.href = u.toString();
        return;
      } catch {
        // fall through to the app
      }
    }
    router.push("/d");
  }

  if (checking || !me) {
    return (
      <Card>
        <CardContent className="py-10 flex justify-center">
          <Spinner />
        </CardContent>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Connect an agent</CardTitle>
        <CardDescription>
          {clientName ? `“${clientName}”` : "An MCP client"} wants to access your
          Inboxes account as {me.email}.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-3 text-sm">
        {!paramsValid ? (
          <div role="alert" className="text-destructive bg-destructive/10 p-3 rounded-md">
            This authorization link is invalid or incomplete. Close this page and
            retry from your MCP client.
          </div>
        ) : (
          <>
            <div className="rounded-md border p-3 text-xs">
              <span className="text-muted-foreground">Sends the connection code to</span>
              <div className="mt-1 font-mono break-all">{redirectHost}</div>
            </div>
            {!redirectIsLoopback && (
              <div role="alert" className="text-destructive bg-destructive/10 p-3 rounded-md">
                Warning: this sends your connection code to a remote site, not a
                program on your computer. A real CLI uses a local address like
                127.0.0.1. Approve only if you started this connection yourself.
              </div>
            )}
            <p className="text-muted-foreground">The agent will be able to:</p>
            <ul className="list-disc pl-5 space-y-1 text-muted-foreground">
              <li>Read the mail your account can see</li>
              <li>Search threads and read conversations</li>
              <li>Create and edit drafts for you to review</li>
              <li>
                Send email <span className="font-medium">only if</span> your org has
                enabled agent send in Settings → Agents
              </li>
            </ul>
            <p className="text-muted-foreground">
              Approve creates one new credential. It appears in Settings →
              Agents, and you can revoke it there anytime. Your other keys do
              not change.
            </p>
            <p className="text-muted-foreground">
              If you did not start this connection yourself, click Deny.
            </p>
          </>
        )}
        {error && (
          <div role="alert" className="text-destructive bg-destructive/10 p-3 rounded-md">
            {error}
          </div>
        )}
      </CardContent>
      <CardFooter className="flex gap-2 justify-end">
        <Button variant="outline" onClick={deny} disabled={working}>
          Deny
        </Button>
        <Button onClick={approve} disabled={working || !paramsValid}>
          {working ? <Spinner className="mr-2" /> : null}
          Approve
        </Button>
      </CardFooter>
    </Card>
  );
}
