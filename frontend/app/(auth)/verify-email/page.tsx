"use client";

import { Suspense, useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
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

function VerifyEmailForm() {
  const router = useRouter();
  const [emailParam, setEmailParam] = useState("");

  useEffect(() => {
    const stored = sessionStorage.getItem("verify-email") || "";
    if (!stored) {
      router.replace("/login");
      return;
    }
    setEmailParam(stored);
  }, [router]);
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [resending, setResending] = useState(false);
  const [resendSuccess, setResendSuccess] = useState(false);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      await api.post("/api/auth/verify-email", {
        email: emailParam,
        code,
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

  async function handleResend() {
    setError("");
    setResending(true);
    setResendSuccess(false);

    try {
      await api.post("/api/auth/resend-verification", {
        email: emailParam,
      });
      setResendSuccess(true);
    } catch {
      setError("Failed to resend code");
    } finally {
      setResending(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Verify your email</CardTitle>
        <CardDescription>
          We emailed the next steps to <strong>{emailParam}</strong>. If you&apos;re
          creating a new account, enter the 6-digit code below. If you already
          have an account, sign in with its current password to add this workspace.
        </CardDescription>
      </CardHeader>
      <form onSubmit={handleSubmit}>
        <CardContent className="space-y-4">
          {error && (
            <div role="alert" className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
              {error}
            </div>
          )}
          {resendSuccess && (
            <div className="text-sm text-green-700 dark:text-green-400 bg-green-50 dark:bg-green-950/40 p-3 rounded-md">
              If this email still needs verification, a new code has been sent.
            </div>
          )}
          <div className="space-y-2">
            <label htmlFor="code" className="text-sm font-medium">
              Verification code
            </label>
            <Input
              id="code"
              value={code}
              onChange={(e) => setCode(e.target.value)}
              placeholder="000000"
              maxLength={6}
              pattern="[0-9]{6}"
              className="text-center text-lg tracking-widest"
              required
            />
          </div>
        </CardContent>
        <CardFooter className="flex flex-col space-y-4">
          <Button className="w-full" disabled={loading || code.length !== 6}>
            {loading ? <Spinner className="mr-2" /> : null}
            Verify
          </Button>
          <button
            type="button"
            onClick={handleResend}
            disabled={resending}
            className="text-sm text-muted-foreground hover:text-primary"
          >
            {resending ? "Sending..." : "Didn\u0027t receive a code? Resend"}
          </button>
          <Link href="/login" className="text-sm text-primary hover:underline">
            Sign in with an existing account
          </Link>
        </CardFooter>
      </form>
    </Card>
  );
}

export default function VerifyEmailPage() {
  return (
    <Suspense fallback={<Spinner />}>
      <VerifyEmailForm />
    </Suspense>
  );
}
