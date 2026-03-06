"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Image from "next/image";
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
import { validatePassword } from "@/lib/utils";

type Step = "account" | "email_config";

interface ResendDomain {
  id: string;
  name: string;
  status: string;
}

export default function SetupPage() {
  const router = useRouter();
  const [step, setStep] = useState<Step>("account");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [systemResendKey, setSystemResendKey] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  const [skippedKeyWarning, setSkippedKeyWarning] = useState(false);

  // Step 2 state
  const [fullAccess, setFullAccess] = useState(false);
  const [domains, setDomains] = useState<ResendDomain[]>([]);
  const [selectedDomain, setSelectedDomain] = useState("");
  const [manualDomain, setManualDomain] = useState("");
  const [prefix, setPrefix] = useState("noreply");
  const [fromName, setFromName] = useState("Inboxes");
  const [validating, setValidating] = useState(false);

  // Redirect if setup is not needed
  useEffect(() => {
    async function check() {
      try {
        const status = await api.get<{ needs_setup: boolean; commercial: boolean }>(
          "/api/setup/status"
        );
        if (!status.needs_setup || status.commercial) {
          router.replace("/login");
          return;
        }
      } catch {
        router.replace("/login");
        return;
      }
      setChecking(false);
    }
    check();
  }, [router]);

  async function handleNext(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const pwError = validatePassword(password);
    if (pwError) {
      setError(pwError);
      return;
    }

    // No key: warn once then submit directly
    if (!systemResendKey) {
      if (!skippedKeyWarning) {
        setSkippedKeyWarning(true);
        return;
      }
      // Submit without email config
      await handleSubmit();
      return;
    }

    // Validate the key
    setValidating(true);
    try {
      const result = await api.post<{
        valid: boolean;
        full_access: boolean;
        domains: ResendDomain[];
        error?: string;
      }>("/api/setup/validate-key", { api_key: systemResendKey });

      if (result.full_access && result.domains.length > 0) {
        setFullAccess(true);
        setDomains(result.domains);
        setSelectedDomain(result.domains[0].name);
      } else {
        setFullAccess(false);
        setDomains([]);
      }
      setStep("email_config");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Failed to validate key");
    } finally {
      setValidating(false);
    }
  }

  async function handleSubmit(fromAddress?: string, fromNameVal?: string) {
    setLoading(true);
    setError("");
    try {
      await api.post("/api/setup", {
        name,
        email,
        password,
        system_resend_key: systemResendKey || undefined,
        system_from_address: fromAddress || undefined,
        system_from_name: fromNameVal || undefined,
      });
      router.push("/onboarding");
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Something went wrong");
      }
    } finally {
      setLoading(false);
    }
  }

  async function handleEmailConfigSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const domain = fullAccess ? selectedDomain : manualDomain;
    if (!domain) {
      setError("Please enter or select a domain");
      return;
    }
    if (!prefix) {
      setError("Please enter an address prefix");
      return;
    }

    const fromAddress = `${prefix}@${domain}`;
    await handleSubmit(fromAddress, fromName);
  }

  const previewAddress = fullAccess
    ? `${prefix}@${selectedDomain}`
    : manualDomain
      ? `${prefix}@${manualDomain}`
      : `${prefix}@yourdomain.com`;

  if (checking) {
    return (
      <div className="flex items-center justify-center py-20">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  const brandHeader = (
    <div className="flex flex-col items-center gap-3 mb-8">
      <Image src="/logo.png" alt="Inboxes" width={48} height={48} className="rounded-xl" />
      <div className="text-center">
        <h1 className="text-2xl font-bold">Inboxes</h1>
        <p className="text-sm text-muted-foreground mt-1">
          The missing inbox for Resend
        </p>
      </div>
    </div>
  );

  if (step === "email_config") {
    return (
      <>
      {brandHeader}
      <Card>
        <CardHeader>
          <CardTitle>Configure system email</CardTitle>
          <CardDescription>
            Choose the address system emails (password resets, invites) will be sent from
          </CardDescription>
        </CardHeader>
        <form onSubmit={handleEmailConfigSubmit}>
          <CardContent className="space-y-4">
            {error && (
              <div role="alert" className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
                {error}
              </div>
            )}

            {!fullAccess && (
              <div className="text-sm text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/40 border border-amber-200 dark:border-amber-800/40 p-3 rounded-md">
                Your key appears to be send-only or couldn&apos;t fetch domains.
                Enter a domain you&apos;ve configured in Resend manually.
              </div>
            )}

            <div className="space-y-2">
              <label className="text-sm font-medium">Domain</label>
              {fullAccess ? (
                <select
                  value={selectedDomain}
                  onChange={(e) => setSelectedDomain(e.target.value)}
                  className="flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring"
                >
                  {domains.map((d) => (
                    <option key={d.id} value={d.name}>
                      {d.name} ({d.status})
                    </option>
                  ))}
                </select>
              ) : (
                <Input
                  value={manualDomain}
                  onChange={(e) => setManualDomain(e.target.value)}
                  placeholder="yourdomain.com"
                  required
                />
              )}
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">Address prefix</label>
              <Input
                value={prefix}
                onChange={(e) => setPrefix(e.target.value)}
                placeholder="noreply"
                required
              />
            </div>

            <div className="space-y-2">
              <label className="text-sm font-medium">From name</label>
              <Input
                value={fromName}
                onChange={(e) => setFromName(e.target.value)}
                placeholder="Inboxes"
              />
            </div>

            <div className="rounded-md bg-muted p-3 text-sm">
              <span className="text-muted-foreground">Preview: </span>
              <span className="font-medium">
                {fromName ? `${fromName} <${previewAddress}>` : previewAddress}
              </span>
            </div>
          </CardContent>
          <CardFooter className="flex gap-2">
            <Button
              type="button"
              variant="outline"
              onClick={() => {
                setStep("account");
                setError("");
              }}
            >
              Back
            </Button>
            <Button className="flex-1" disabled={loading}>
              {loading ? <Spinner className="mr-2" /> : null}
              Complete setup
            </Button>
          </CardFooter>
        </form>
      </Card>
      </>
    );
  }

  return (
    <>
    {brandHeader}
    <Card>
      <CardHeader>
        <CardTitle>Set up your instance</CardTitle>
        <CardDescription>
          Create the first admin account to get started
        </CardDescription>
      </CardHeader>
      <form onSubmit={handleNext}>
        <CardContent className="space-y-4">
          {error && (
            <div role="alert" className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
              {error}
            </div>
          )}
          <div className="space-y-2">
            <label htmlFor="name" className="text-sm font-medium">
              Name
            </label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Your name"
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="email" className="text-sm font-medium">
              Email
            </label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="admin@example.com"
              required
            />
          </div>
          <div className="space-y-2">
            <label htmlFor="password" className="text-sm font-medium">
              Password
            </label>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="At least 8 characters"
              required
              minLength={8}
            />
            <p className="text-xs text-muted-foreground">
              Must include uppercase, lowercase, and a number
            </p>
          </div>
          <div className="space-y-2">
            <label htmlFor="system-key" className="text-sm font-medium">
              System Resend API Key{" "}
              <span className="text-muted-foreground font-normal">(optional)</span>
            </label>
            <Input
              id="system-key"
              type="password"
              value={systemResendKey}
              onChange={(e) => {
                setSystemResendKey(e.target.value);
                setSkippedKeyWarning(false);
              }}
              placeholder="re_..."
            />
            <p className="text-xs text-muted-foreground">
              Used for transactional emails like password resets and invites.
              You can add this later in environment variables.
            </p>
          </div>
          {skippedKeyWarning && !systemResendKey && (
            <div className="text-sm text-amber-700 bg-amber-50 border border-amber-200 p-3 rounded-md">
              Without a system Resend key, password resets and email invites
              won&apos;t work. Submit again to continue anyway.
            </div>
          )}
        </CardContent>
        <CardFooter>
          <Button className="w-full" disabled={loading || validating}>
            {(loading || validating) ? <Spinner className="mr-2" /> : null}
            {skippedKeyWarning && !systemResendKey
              ? "Continue without email"
              : systemResendKey
                ? "Next"
                : "Create admin account"}
          </Button>
        </CardFooter>
      </form>
    </Card>
    </>
  );
}
