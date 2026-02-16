"use client";

import { useState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { useDomains } from "@/contexts/domain-context";
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
import { Badge } from "@/components/ui/badge";
import { Spinner } from "@/components/ui/spinner";
import type { Domain, DiscoveredAddress } from "@/lib/types";
import {
  Check,
  Minus,
  Key,
  Globe,
  Download,
  Users,
  ArrowRight,
} from "lucide-react";

type Step = "connect" | "domains" | "sync" | "addresses";

const STEPS: { key: Step; label: string; icon: React.ReactNode }[] = [
  { key: "connect", label: "Connect Resend", icon: <Key className="h-4 w-4" /> },
  { key: "domains", label: "Your Domains", icon: <Globe className="h-4 w-4" /> },
  { key: "sync", label: "Import Emails", icon: <Download className="h-4 w-4" /> },
  { key: "addresses", label: "Set Up Addresses", icon: <Users className="h-4 w-4" /> },
];

export default function OnboardingPage() {
  const router = useRouter();
  const { refreshDomains, refreshUnreadCounts } = useDomains();
  const [step, setStep] = useState<Step | null>(null);
  const [apiKey, setApiKey] = useState("");
  const [domains, setDomains] = useState<Domain[]>([]);
  const [selectedDomainIds, setSelectedDomainIds] = useState<Set<string>>(new Set());
  const [discoveredAddresses, setDiscoveredAddresses] = useState<
    DiscoveredAddress[]
  >([]);
  const [addressAssignments, setAddressAssignments] = useState<
    Record<string, "user" | "alias" | "skip">
  >({});
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
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

  // Resume onboarding at the right step based on server state
  useEffect(() => {
    async function checkStatus() {
      try {
        const res = await api.get<{ step: Step }>("/api/onboarding/status");
        setStep(res.step);

        // If resuming at domains step, load domains from DB
        if (res.step === "domains" || res.step === "sync" || res.step === "addresses") {
          const domainData = await api.get<Domain[]>("/api/domains/all");
          setDomains(domainData);
          setSelectedDomainIds(new Set(domainData.filter((d) => !d.hidden).map((d) => d.id)));
        }

        // If resuming at addresses step, load addresses
        if (res.step === "addresses") {
          const rows = await api.get<DiscoveredAddress[]>("/api/onboarding/addresses");
          setDiscoveredAddresses(rows || []);
          const defaults: Record<string, "alias"> = {};
          (rows || []).forEach((a) => {
            defaults[a.id] = "alias";
          });
          setAddressAssignments(defaults);
        }
      } catch {
        setStep("connect");
      }
    }
    checkStatus();
  }, []);

  const syncTriggeredRef = useRef(false);

  // Auto-trigger sync when entering the sync step
  useEffect(() => {
    if (step === "sync" && !syncTriggeredRef.current && !loading && !syncResult) {
      syncTriggeredRef.current = true;
      handleSync();
    }
  }, [step]); // eslint-disable-line react-hooks/exhaustive-deps

  const currentIdx = step ? STEPS.findIndex((s) => s.key === step) : -1;

  async function handleConnect(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);
    try {
      const res = await api.post<{ domains: Domain[] }>(
        "/api/onboarding/connect",
        { api_key: apiKey }
      );
      setDomains(res.domains);
      setSelectedDomainIds(new Set(res.domains.map((d: Domain) => d.id)));
      setStep("domains");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to connect");
    } finally {
      setLoading(false);
    }
  }

  async function handleSelectDomains() {
    setError("");
    setLoading(true);
    try {
      await api.post("/api/onboarding/domains", {
        domain_ids: Array.from(selectedDomainIds),
      });
      // Set up webhook in background — don't block the user
      api.post("/api/onboarding/webhook").catch(() => {});
      setStep("sync");
    } catch (err) {
      setError(
        err instanceof ApiError ? err.message : "Failed to update domains"
      );
    } finally {
      setLoading(false);
    }
  }

  function handleSync() {
    setError("");
    setLoading(true);
    setSyncProgress(null);
    setSyncResult(null);

    const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080";
    const source = new EventSource(`${API_URL}/api/onboarding/sync-stream`, {
      withCredentials: true,
    });

    source.addEventListener("progress", (e) => {
      const data = JSON.parse((e as MessageEvent).data);
      setSyncProgress(data);
    });

    source.addEventListener("done", async (e) => {
      const data = JSON.parse((e as MessageEvent).data);
      setSyncResult(data);
      source.close();
      setLoading(false);

      // Fetch discovered addresses for the next step
      try {
        const rows = await api.get<DiscoveredAddress[]>(
          "/api/onboarding/addresses"
        );
        setDiscoveredAddresses(rows || []);
        const defaults: Record<string, "alias"> = {};
        (rows || []).forEach((a) => {
          defaults[a.id] = "alias";
        });
        setAddressAssignments(defaults);
      } catch {
        // addresses will be empty, user can set them up later
      }
    });

    source.addEventListener("error", (e) => {
      // SSE error event — could be a stream error or connection drop
      try {
        const data = JSON.parse((e as MessageEvent).data);
        setError(data.error || "Sync failed");
      } catch {
        setError("Connection lost during sync");
      }
      source.close();
      setLoading(false);
    });

    source.onerror = () => {
      // Built-in EventSource error (connection failure)
      if (source.readyState === EventSource.CLOSED) return;
      source.close();
      if (!syncResult) {
        setError("Connection lost during sync");
        setLoading(false);
      }
    };
  }

  async function handleComplete() {
    setError("");
    setLoading(true);
    try {
      const assignments = Object.entries(addressAssignments).map(
        ([id, type]) => ({ address_id: id, type })
      );
      await api.post("/api/onboarding/addresses", { assignments });
      const res = await api.post<{ first_domain_id: string }>(
        "/api/onboarding/complete"
      );
      await Promise.all([refreshDomains(), refreshUnreadCounts()]);
      router.push(`/d/${res.first_domain_id}/inbox`);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to complete");
    } finally {
      setLoading(false);
    }
  }

  if (!step) {
    return (
      <div className="flex items-center justify-center h-screen">
        <Spinner className="h-8 w-8" />
      </div>
    );
  }

  return (
    <div className="min-h-screen bg-muted/30 flex items-start justify-center pt-16 p-4">
      <div className="w-full max-w-2xl space-y-8">
        {/* Progress */}
        <div className="flex items-center justify-center gap-2">
          {STEPS.map((s, i) => (
            <div key={s.key} className="flex items-center gap-2">
              <div
                className={`flex items-center gap-1.5 rounded-full px-3 py-1.5 text-xs font-medium transition-colors ${
                  i < currentIdx
                    ? "bg-primary text-primary-foreground"
                    : i === currentIdx
                      ? "bg-primary text-primary-foreground"
                      : "bg-muted text-muted-foreground"
                }`}
              >
                {i < currentIdx ? (
                  <Check className="h-3 w-3" />
                ) : (
                  s.icon
                )}
                <span className="hidden sm:inline">{s.label}</span>
              </div>
              {i < STEPS.length - 1 && (
                <ArrowRight className="h-3 w-3 text-muted-foreground" />
              )}
            </div>
          ))}
        </div>

        {/* Step: Connect */}
        {step === "connect" && (
          <Card>
            <CardHeader>
              <CardTitle>Connect your Resend account</CardTitle>
              <CardDescription>
                Paste your Resend API key to get started. We&apos;ll fetch your
                domains and email history.
              </CardDescription>
            </CardHeader>
            <form onSubmit={handleConnect}>
              <CardContent className="space-y-4">
                {error && (
                  <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
                    {error}
                  </div>
                )}
                <div className="space-y-2">
                  <label htmlFor="apiKey" className="text-sm font-medium">
                    Resend API Key
                  </label>
                  <Input
                    id="apiKey"
                    value={apiKey}
                    onChange={(e) => setApiKey(e.target.value)}
                    placeholder="re_..."
                    required
                  />
                </div>
              </CardContent>
              <CardFooter>
                <Button className="w-full" disabled={loading}>
                  {loading ? <Spinner className="mr-2" /> : null}
                  Connect
                </Button>
              </CardFooter>
            </form>
          </Card>
        )}

        {/* Step: Domains */}
        {step === "domains" && (
          <Card>
            <CardHeader>
              <CardTitle>Your domains</CardTitle>
              <CardDescription>
                We found {domains.length} domain
                {domains.length !== 1 ? "s" : ""} in your Resend account.
                Deselect any you don&apos;t want to use.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <div className="space-y-3">
                <button
                  type="button"
                  onClick={() => {
                    if (selectedDomainIds.size > 0) {
                      setSelectedDomainIds(new Set());
                    } else {
                      setSelectedDomainIds(new Set(domains.map((d) => d.id)));
                    }
                  }}
                  className="flex items-center gap-3 text-sm text-muted-foreground hover:text-foreground transition-colors"
                >
                  <div
                    className={`h-4 w-4 rounded border flex items-center justify-center transition-colors ${
                      selectedDomainIds.size === domains.length
                        ? "bg-primary border-primary"
                        : selectedDomainIds.size > 0
                          ? "bg-primary border-primary"
                          : "border-muted-foreground"
                    }`}
                  >
                    {selectedDomainIds.size === domains.length ? (
                      <Check className="h-3 w-3 text-primary-foreground" />
                    ) : selectedDomainIds.size > 0 ? (
                      <Minus className="h-3 w-3 text-primary-foreground" />
                    ) : null}
                  </div>
                  <span>
                    {selectedDomainIds.size > 0 ? "Deselect all" : "Select all"}
                  </span>
                </button>
                {domains.map((d) => {
                  const selected = selectedDomainIds.has(d.id);
                  return (
                    <button
                      key={d.id}
                      type="button"
                      onClick={() =>
                        setSelectedDomainIds((prev) => {
                          const next = new Set(prev);
                          if (next.has(d.id)) next.delete(d.id);
                          else next.add(d.id);
                          return next;
                        })
                      }
                      className={`flex w-full items-center justify-between rounded-lg border p-3 transition-colors ${
                        selected
                          ? "border-primary bg-primary/5"
                          : "border-muted opacity-50"
                      }`}
                    >
                      <div className="flex items-center gap-3">
                        <div
                          className={`h-4 w-4 rounded border flex items-center justify-center transition-colors ${
                            selected
                              ? "bg-primary border-primary"
                              : "border-muted-foreground"
                          }`}
                        >
                          {selected && (
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
              </div>
            </CardContent>
            <CardFooter>
              <Button
                className="w-full"
                onClick={handleSelectDomains}
                disabled={selectedDomainIds.size === 0 || loading}
              >
                {loading ? <Spinner className="mr-2" /> : null}
                Continue with {selectedDomainIds.size} domain
                {selectedDomainIds.size !== 1 ? "s" : ""}
              </Button>
            </CardFooter>
          </Card>
        )}

        {/* Step: Sync */}
        {step === "sync" && (
          <Card>
            <CardHeader>
              <CardTitle>Import email history</CardTitle>
              <CardDescription>
                {syncProgress
                  ? syncProgress.message
                  : "We\u2019ll import your sent and received emails from Resend and organize them into threads."}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {error && (
                <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
                  {error}
                </div>
              )}

              {/* Scanning phase (counting emails before import) */}
              {syncProgress && syncProgress.phase === "scanning" && (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Spinner />
                  {syncProgress.message}
                </div>
              )}

              {/* Import progress bar */}
              {syncProgress && syncProgress.phase === "importing" && syncProgress.total > 0 && (
                <div className="space-y-2">
                  <div className="flex justify-between text-sm text-muted-foreground">
                    <span>Importing emails...</span>
                    <span>
                      {syncProgress.imported} / {syncProgress.total}
                    </span>
                  </div>
                  <div className="h-2 rounded-full bg-muted overflow-hidden">
                    <div
                      className="h-full bg-primary rounded-full transition-all duration-300 ease-out"
                      style={{
                        width: `${Math.round((syncProgress.imported / syncProgress.total) * 100)}%`,
                      }}
                    />
                  </div>
                </div>
              )}

              {/* Discovering addresses */}
              {syncProgress && syncProgress.phase === "addresses" && (
                <div className="flex items-center gap-2 text-sm text-muted-foreground">
                  <Spinner />
                  Discovering addresses...
                </div>
              )}

              {/* Final result */}
              {syncResult && (
                <div className="rounded-lg bg-green-50 border border-green-200 p-4 space-y-1">
                  <p className="text-sm text-green-800">
                    Imported{" "}
                    <strong>{syncResult.sent_count}</strong> sent and{" "}
                    <strong>{syncResult.received_count}</strong> received emails
                  </p>
                  <p className="text-sm text-green-800">
                    Created{" "}
                    <strong>{syncResult.thread_count}</strong> threads
                  </p>
                  <p className="text-sm text-green-800">
                    Discovered{" "}
                    <strong>{syncResult.address_count}</strong> addresses
                  </p>
                </div>
              )}
            </CardContent>
            <CardFooter>
              {syncResult ? (
                <Button
                  className="w-full"
                  onClick={() => setStep("addresses")}
                >
                  Continue
                </Button>
              ) : (
                <div className="flex items-center gap-2 w-full text-sm text-muted-foreground justify-center">
                  <Spinner className="h-4 w-4" /> Importing your emails...
                </div>
              )}
            </CardFooter>
          </Card>
        )}

        {/* Step: Addresses */}
        {step === "addresses" && (
          <Card>
            <CardHeader>
              <CardTitle>Set up addresses</CardTitle>
              <CardDescription>
                Categorize each discovered address as a person, alias, or skip.
              </CardDescription>
            </CardHeader>
            <CardContent>
              {error && (
                <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md mb-4">
                  {error}
                </div>
              )}
              {discoveredAddresses.length === 0 ? (
                <p className="text-sm text-muted-foreground">
                  No addresses discovered. You can add them later in settings.
                </p>
              ) : (
                <div className="space-y-3">
                  {discoveredAddresses.map((addr) => (
                    <div
                      key={addr.id}
                      className="flex items-center justify-between rounded-lg border p-3"
                    >
                      <div>
                        <p className="font-medium text-sm">{addr.address}</p>
                        <p className="text-xs text-muted-foreground">
                          {addr.email_count} email
                          {addr.email_count !== 1 ? "s" : ""}
                        </p>
                      </div>
                      <div className="flex gap-1">
                        {(["user", "alias", "skip"] as const).map((type) => (
                          <button
                            key={type}
                            onClick={() =>
                              setAddressAssignments((prev) => ({
                                ...prev,
                                [addr.id]: type,
                              }))
                            }
                            className={`px-2.5 py-1 text-xs rounded-md border transition-colors ${
                              addressAssignments[addr.id] === type
                                ? "bg-primary text-primary-foreground border-primary"
                                : "hover:bg-muted"
                            }`}
                          >
                            {type === "user"
                              ? "Person"
                              : type === "alias"
                                ? "Alias"
                                : "Skip"}
                          </button>
                        ))}
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </CardContent>
            <CardFooter>
              <Button
                className="w-full"
                onClick={handleComplete}
                disabled={loading}
              >
                {loading ? <Spinner className="mr-2" /> : null}
                Complete setup
              </Button>
            </CardFooter>
          </Card>
        )}
      </div>
    </div>
  );
}
