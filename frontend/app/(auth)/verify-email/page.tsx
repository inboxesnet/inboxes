"use client";

import { useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
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

export default function VerifyEmailPage() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const emailParam = searchParams.get("email") || "";
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
          We sent a 6-digit code to <strong>{emailParam}</strong>. Enter it
          below to verify your account.
        </CardDescription>
      </CardHeader>
      <form onSubmit={handleSubmit}>
        <CardContent className="space-y-4">
          {error && (
            <div className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
              {error}
            </div>
          )}
          {resendSuccess && (
            <div className="text-sm text-green-700 bg-green-50 p-3 rounded-md">
              A new code has been sent to your email.
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
        </CardFooter>
      </form>
    </Card>
  );
}
