"use client";

import { Suspense, useState, useEffect } from "react";
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
import { validatePassword } from "@/lib/utils";

export default function ClaimPage() {
  return (
    <Suspense fallback={<Card><CardContent className="flex items-center justify-center p-12"><Spinner className="h-6 w-6" /></CardContent></Card>}>
      <ClaimForm />
    </Suspense>
  );
}

function ClaimForm() {
  const router = useRouter();
  const searchParams = useSearchParams();
  const token = searchParams.get("token") || "";
  const [name, setName] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);
  const [validating, setValidating] = useState(true);
  const [valid, setValid] = useState(false);
  const [email, setEmail] = useState("");

  useEffect(() => {
    async function validate() {
      try {
        const res = await api.get<{ email: string }>(
          `/api/auth/claim/validate?token=${token}`
        );
        setEmail(res.email);
        setValid(true);
      } catch {
        setValid(false);
      } finally {
        setValidating(false);
      }
    }
    if (token) validate();
    else setValidating(false);
  }, [token]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError("");

    const pwError = validatePassword(password);
    if (pwError) {
      setError(pwError);
      return;
    }

    setLoading(true);

    try {
      await api.post("/api/auth/claim", { token, name, password });
      router.push("/d");
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

  if (validating) {
    return (
      <Card>
        <CardContent className="flex items-center justify-center p-12">
          <Spinner className="h-6 w-6" />
        </CardContent>
      </Card>
    );
  }

  if (!valid) {
    return (
      <Card>
        <CardHeader>
          <CardTitle>Invalid invitation</CardTitle>
          <CardDescription>
            This invite link is invalid or has expired.
          </CardDescription>
        </CardHeader>
        <CardFooter>
          <Button variant="outline" className="w-full" onClick={() => router.push("/login")}>
            Go to login
          </Button>
        </CardFooter>
      </Card>
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Claim your account</CardTitle>
        <CardDescription>
          Complete your account setup for {email}
        </CardDescription>
      </CardHeader>
      <form onSubmit={handleSubmit}>
        <CardContent className="space-y-4">
          {error && (
            <div role="alert" className="text-sm text-destructive bg-destructive/10 p-3 rounded-md">
              {error}
            </div>
          )}
          <div className="space-y-2">
            <label htmlFor="name" className="text-sm font-medium">
              Your name
            </label>
            <Input
              id="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Jane Smith"
              maxLength={255}
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
              minLength={8}
              required
            />
            <p className="text-xs text-muted-foreground">
              Must include uppercase, lowercase, and a number
            </p>
          </div>
        </CardContent>
        <CardFooter className="flex-col gap-3">
          <Button className="w-full" disabled={loading}>
            {loading ? <Spinner className="mr-2" /> : null}
            Set up account
          </Button>
          <a href="/login" className="text-sm text-muted-foreground hover:text-primary">
            Back to sign in
          </a>
        </CardFooter>
      </form>
    </Card>
  );
}
