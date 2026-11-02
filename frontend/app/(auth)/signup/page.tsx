"use client";

import { useEffect, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  ArrowRight,
  Building2,
  Check,
  KeyRound,
  UserRound,
} from "lucide-react";
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

const STEPS = [
  { label: "Workspace", icon: Building2 },
  { label: "Your details", icon: UserRound },
  { label: "Password", icon: KeyRound },
] as const;

interface SignupResponse {
  requires_verification?: boolean;
  email?: string;
  user?: { id: string; org_id: string; email: string; name: string; role: string };
  onboarding_completed?: boolean;
}

export default function SignupPage() {
  const router = useRouter();
  const organizationInput = useRef<HTMLInputElement>(null);
  const nameInput = useRef<HTMLInputElement>(null);
  const emailInput = useRef<HTMLInputElement>(null);
  const passwordInput = useRef<HTMLInputElement>(null);
  const [step, setStep] = useState(0);
  const [orgName, setOrgName] = useState("");
  const [name, setName] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [checking, setChecking] = useState(true);
  const [blocked, setBlocked] = useState(false);
  const [isAuthenticated, setIsAuthenticated] = useState(false);

  useEffect(() => {
    let active = true;

    async function checkAccess() {
      try {
        const me = await api.get<{ email: string; name: string }>("/api/users/me", { noLatch: true });
        if (active) {
          setIsAuthenticated(true);
          setEmail(me.email);
          setName(me.name);
        }
      } catch {
        // No active session — check whether public signup is available.
      }

      try {
        const status = await api.get<{ needs_setup: boolean; commercial: boolean }>(
          "/api/setup/status"
        );
        if (status.needs_setup) {
          router.replace("/setup");
          return;
        }
        if (active && !status.commercial) setBlocked(true);
      } catch {
        // Keep the form available if the status endpoint cannot be reached.
      } finally {
        if (active) setChecking(false);
      }
    }

    void checkAccess();
    return () => {
      active = false;
    };
  }, [router]);

  function validateStep() {
    setError("");

    if (step === 0) {
      if (!organizationInput.current?.reportValidity()) return false;
      if (!orgName.trim()) {
        setError("Enter a workspace name to continue.");
        organizationInput.current?.focus();
        return false;
      }
      if (orgName.trim().length > 255) {
        setError("Workspace names must be 255 characters or fewer.");
        return false;
      }
    } else if (step === 1) {
      if (!nameInput.current?.reportValidity()) return false;
      if (!name.trim()) {
        setError("Enter your name to continue.");
        nameInput.current?.focus();
        return false;
      }
      if (name.trim().length > 255) {
        setError("Names must be 255 characters or fewer.");
        return false;
      }
      if (!emailInput.current?.reportValidity()) return false;
    } else if (!isAuthenticated) {
      if (!passwordInput.current?.reportValidity()) return false;
      const passwordError = validatePassword(password);
      if (passwordError) {
        setError(passwordError);
        return false;
      }
    }

    return true;
  }

  function actionableSignupError(err: unknown) {
    if (err instanceof ApiError) {
      const message = err.message.toLowerCase().replaceAll("_", " ");
      if (
        err.status === 401 ||
        message.includes("invalid credential") ||
        message.includes("invalid email or password") ||
        message.includes("incorrect password") ||
        message.includes("wrong password")
      ) {
        return "That password didn’t match this email. If you already have an account, use its current password to create this workspace.";
      }
      if (message.includes("email already registered")) {
        return "This email already has an account. Use that account’s current password to create another workspace.";
      }
      return err.message;
    }
    return "We couldn’t connect. Check your connection and try again.";
  }

  async function handleSubmit(event: React.FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!validateStep()) return;

    const visibleSteps = isAuthenticated ? 1 : STEPS.length;
    if (step < visibleSteps - 1) {
      setStep((current) => current + 1);
      return;
    }

    setLoading(true);
    try {
      const response = isAuthenticated
        ? await api.post<SignupResponse>("/api/orgs", { org_name: orgName.trim() })
        : await api.post<SignupResponse>("/api/auth/signup", {
            org_name: orgName.trim(),
            email: email.trim(),
            name: name.trim(),
            password,
          });

      if (response.requires_verification) {
        sessionStorage.setItem("verify-email", response.email || email.trim());
        router.push("/verify-email");
      } else if (isAuthenticated) {
        // Reload so workspace-scoped query data and the websocket start fresh.
        window.location.href = response.onboarding_completed ? "/d" : "/onboarding";
      } else {
        router.push(response.onboarding_completed ? "/d" : "/onboarding");
      }
    } catch (err) {
      if (err instanceof ApiError && err.message === "email_not_verified") {
        sessionStorage.setItem("verify-email", email.trim());
        router.push("/verify-email");
        return;
      }
      setError(actionableSignupError(err));
    } finally {
      setLoading(false);
    }
  }

  function goBack() {
    setError("");
    setStep((current) => Math.max(0, current - 1));
  }

  const visibleSteps = isAuthenticated ? STEPS.slice(0, 1) : STEPS;
  const title = isAuthenticated ? "Name your new workspace" : ["Name your workspace", "Tell us about you", "Secure your account"][step];
  const description = isAuthenticated
    ? `This workspace will be added to ${email}.`
    : [
        "Start with a name your team will recognize.",
        "Add the details for the account that owns this workspace.",
        "Choose the password for this account.",
      ][step];

  if (checking) {
    return (
      <Card className="overflow-hidden border-border/70 shadow-lg shadow-black/5">
        <div className="h-1 bg-primary" />
        <CardContent className="flex min-h-52 items-center justify-center gap-3 text-sm text-muted-foreground">
          <Spinner className="h-5 w-5" />
          <span role="status">Getting things ready…</span>
        </CardContent>
      </Card>
    );
  }

  if (blocked) {
    return (
      <Card className="overflow-hidden border-border/70 shadow-lg shadow-black/5">
        <div className="h-1 bg-primary" />
        <CardHeader className="pb-3">
          <div className="mb-3 flex h-11 w-11 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <Building2 className="h-5 w-5" aria-hidden="true" />
          </div>
          <CardTitle>Registration is closed</CardTitle>
          <CardDescription>
            This Inboxes instance is invite-only. Contact your administrator to request access.
          </CardDescription>
        </CardHeader>
        <CardFooter className="pt-2">
          <Link href="/login" className="text-sm font-medium text-primary hover:underline">
            Back to sign in
          </Link>
        </CardFooter>
      </Card>
    );
  }

  const progress = ((step + 1) / visibleSteps.length) * 100;
  const CurrentStepIcon = STEPS[step].icon;

  return (
    <Card className="overflow-hidden border-border/70 shadow-xl shadow-black/5">
      <div className="h-1 bg-gradient-to-r from-primary/50 via-primary to-primary/70" />
      <CardHeader className="space-y-5 pb-5 pt-6 sm:px-7">
        <div className="flex items-center gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-xl bg-primary/10 text-primary">
            <CurrentStepIcon className="h-5 w-5" aria-hidden="true" />
          </div>
          <div>
            <p className="text-[11px] font-semibold uppercase tracking-[0.16em] text-primary">
              Workspace setup
            </p>
            <p className="mt-0.5 text-xs text-muted-foreground">A few quick steps to get started</p>
          </div>
        </div>

        <div className="space-y-3">
          <div className="flex items-center justify-between text-xs">
            <span className="font-medium text-foreground">Step {step + 1} of {visibleSteps.length}</span>
            <span className="text-muted-foreground">{STEPS[step].label}</span>
          </div>
          <div
            className="h-1.5 overflow-hidden rounded-full bg-muted"
            role="progressbar"
            aria-label="Signup progress"
            aria-valuemin={1}
            aria-valuemax={visibleSteps.length}
            aria-valuenow={step + 1}
            aria-valuetext={`Step ${step + 1} of ${visibleSteps.length}: ${visibleSteps[step].label}`}
          >
            <div
              className="h-full rounded-full bg-primary transition-[width] duration-300 ease-out"
              style={{ width: `${progress}%` }}
            />
          </div>
          <ol className="grid grid-cols-3 gap-2" aria-label="Signup steps">
            {visibleSteps.map((signupStep, index) => {
              const StepIcon = signupStep.icon;
              const complete = index < step;
              const current = index === step;
              return (
                <li
                  key={signupStep.label}
                  aria-current={current ? "step" : undefined}
                  className={`flex items-center gap-1.5 text-[11px] sm:text-xs ${
                    current ? "font-semibold text-foreground" : complete ? "text-primary" : "text-muted-foreground"
                  }`}
                >
                  <span
                    className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full ${
                      current
                        ? "bg-primary text-primary-foreground"
                        : complete
                          ? "bg-primary/10 text-primary"
                          : "bg-muted text-muted-foreground"
                    }`}
                  >
                    {complete ? (
                      <Check className="h-3 w-3" aria-hidden="true" />
                    ) : (
                      <StepIcon className="h-3 w-3" aria-hidden="true" />
                    )}
                  </span>
                  <span className="truncate">{signupStep.label}</span>
                </li>
              );
            })}
          </ol>
        </div>

        <div className="space-y-1">
          <CardTitle id="signup-step-title" className="text-xl sm:text-2xl">
            {title}
          </CardTitle>
          <CardDescription>{description}</CardDescription>
        </div>
      </CardHeader>

      <form onSubmit={handleSubmit} noValidate aria-labelledby="signup-step-title">
        <CardContent className="space-y-4 sm:px-7">
          {error && (
            <div
              role="alert"
              aria-live="assertive"
              className="rounded-md border border-destructive/20 bg-destructive/10 p-3 text-sm text-destructive"
            >
              {error}
            </div>
          )}

          {step === 0 && (
            <div className="space-y-2">
              <label htmlFor="orgName" className="text-sm font-medium">
                Workspace name
              </label>
              <Input
                ref={organizationInput}
                id="orgName"
                value={orgName}
                onChange={(event) => setOrgName(event.target.value)}
                placeholder="Acme Studio"
                autoComplete="organization"
                maxLength={255}
                required
                autoFocus
              />
              <p className="text-xs text-muted-foreground">
                You can change this later in your workspace settings.
              </p>
              {isAuthenticated && (
                <p className="rounded-md bg-muted/70 px-3 py-2.5 text-xs leading-relaxed text-muted-foreground">
                  Your signed-in account will be the admin of this workspace. It starts with its own billing and Resend setup.
                </p>
              )}
            </div>
          )}

          {step === 1 && (
            <div className="space-y-4">
              <div className="space-y-2">
                <label htmlFor="name" className="text-sm font-medium">
                  Your name
                </label>
                <Input
                  ref={nameInput}
                  id="name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Jane Smith"
                  autoComplete="name"
                  maxLength={255}
                  required
                  autoFocus
                />
              </div>
              <div className="space-y-2">
                <label htmlFor="email" className="text-sm font-medium">
                  Email address
                </label>
                <Input
                  ref={emailInput}
                  id="email"
                  type="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  placeholder="you@example.com"
                  autoComplete="email"
                  maxLength={320}
                  required
                />
              </div>
              <p className="rounded-md bg-muted/70 px-3 py-2.5 text-xs leading-relaxed text-muted-foreground">
                If this email already has an account, use that account’s current password in the next step to add this workspace.
              </p>
            </div>
          )}

          {step === 2 && (
            <div className="space-y-2">
              <label htmlFor="password" className="text-sm font-medium">
                Account password
              </label>
              <Input
                ref={passwordInput}
                id="password"
                type="password"
                value={password}
                onChange={(event) => setPassword(event.target.value)}
                placeholder="At least 8 characters"
                autoComplete="current-password"
                minLength={8}
                maxLength={128}
                required
                autoFocus
                aria-describedby="password-guidance"
              />
              <p id="password-guidance" className="text-xs leading-relaxed text-muted-foreground">
                Use at least 8 characters, with an uppercase letter, a lowercase letter, and a number. For an existing account, enter its current password.
              </p>
            </div>
          )}
        </CardContent>

        <CardFooter className="flex-col gap-4 pt-1 sm:px-7">
          <div className="flex w-full items-center gap-3">
            {step > 0 && (
              <Button
                type="button"
                variant="outline"
                onClick={goBack}
                disabled={loading}
                aria-label="Go to previous step"
              >
                <ArrowLeft className="mr-2 h-4 w-4" aria-hidden="true" />
                Back
              </Button>
            )}
            <Button type="submit" className="h-11 flex-1" disabled={loading}>
              {loading ? <Spinner className="mr-2" /> : null}
              {loading
                ? "Creating workspace…"
                : step === visibleSteps.length - 1
                  ? "Create workspace"
                  : "Continue"}
              {!loading && step < visibleSteps.length - 1 ? (
                <ArrowRight className="ml-2 h-4 w-4" aria-hidden="true" />
              ) : null}
            </Button>
          </div>

          <p className="text-center text-sm text-muted-foreground">
            {isAuthenticated ? (
              <Link href="/d" className="font-medium text-primary hover:underline">
                Back to your workspace
              </Link>
            ) : (
              <>
                Already have an account?{" "}
                <Link href="/login" className="font-medium text-primary hover:underline">
                  Sign in
                </Link>
              </>
            )}
          </p>
          <p className="text-center text-[11px] leading-relaxed text-muted-foreground">
            By continuing, you agree to the Inboxes{" "}
            <Link href="/privacy" className="underline underline-offset-2 hover:text-foreground">
              Privacy Policy
            </Link>{" "}
            and{" "}
            <Link href="/terms" className="underline underline-offset-2 hover:text-foreground">
              Terms and Conditions
            </Link>
            .
          </p>
        </CardFooter>
      </form>
    </Card>
  );
}
