"use client";

import { useState, useEffect, Suspense } from "react";
import { useRouter, useSearchParams } from "next/navigation";
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

export default function LoginPage() {
  return (
    <Suspense>
      <LoginForm />
    </Suspense>
  );
}

function LoginForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [showSignup, setShowSignup] = useState(true);

  // Only same-origin relative paths are valid post-login destinations. Reject a
  // leading backslash too: browsers read "/\evil.com" as "//evil.com" and would
  // redirect off-site after login.
  const rawNext = searchParams.get("next") || "";
  const next =
    rawNext.startsWith("/") &&
    !rawNext.startsWith("//") &&
    !rawNext.startsWith("/\\")
      ? rawNext
      : "";

  useEffect(() => {
    // A user with a valid session does not need the login form.
    api
      .get("/api/users/me", { noLatch: true })
      .then(() => router.replace(next || "/d"))
      .catch(() => {
        // No session — stay on the login form.
      });
  }, [router, next]);

  useEffect(() => {
    api
      .get<{ commercial: boolean }>("/api/setup/status")
      .then((res) => {
        if (!res.commercial) {
          setShowSignup(false);
        }
      })
      .catch(() => {
        // If status check fails, keep signup link visible
      });
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");
    setLoading(true);

    try {
      const res = await api.post<{ onboarding_completed: boolean }>(
        "/api/auth/login",
        { email, password }
      );
      if (next) {
        router.push(next);
      } else if (res.onboarding_completed) {
        router.push("/d");
      } else {
        router.push("/onboarding");
      }
    } catch (err) {
      if (err instanceof ApiError) {
        if (err.message === "email_not_verified") {
          sessionStorage.setItem("verify-email", email);
          router.push("/verify-email");
          return;
        }
        setError(err.message);
      } else {
        setError("Something went wrong");
      }
    } finally {
      setLoading(false);
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Welcome back</CardTitle>
        <CardDescription>Sign in to your Inboxes account</CardDescription>
      </CardHeader>
      <form onSubmit={handleSubmit}>
        <CardContent className="space-y-4">
          {error && (
            <div role="alert" className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
              {error}
            </div>
          )}
          <div className="space-y-2">
            <label htmlFor="email" className="text-sm font-medium">
              Email
            </label>
            <Input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="you@example.com"
              required
            />
          </div>
          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label htmlFor="password" className="text-sm font-medium">
                Password
              </label>
              <Link
                href="/forgot-password"
                className="text-sm text-muted-foreground hover:text-primary"
              >
                Forgot password?
              </Link>
            </div>
            <Input
              id="password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              required
            />
          </div>
        </CardContent>
        <CardFooter className="flex flex-col space-y-4">
          <Button className="w-full" disabled={loading}>
            {loading ? <Spinner className="mr-2" /> : null}
            Sign in
          </Button>
          {showSignup && (
            <p className="text-sm text-muted-foreground text-center">
              Don&apos;t have an account?{" "}
              <Link href="/signup" className="text-primary hover:underline">
                Sign up
              </Link>
            </p>
          )}
        </CardFooter>
      </form>
    </Card>
  );
}
