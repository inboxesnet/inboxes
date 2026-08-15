"use client";

import { useState, useEffect } from "react";
import { useRouter } from "next/navigation";
import { api, ApiError } from "@/lib/api";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@/components/ui/card";
import { Spinner } from "@/components/ui/spinner";
import { CreditCard } from "lucide-react";

interface BillingInfo {
  plan: string;
  plan_expires_at: string | null;
}

interface Me {
  role: string;
}

export default function BillingPage() {
  const router = useRouter();
  const [billing, setBilling] = useState<BillingInfo | null>(null);
  const [me, setMe] = useState<Me | null>(null);
  const [error, setError] = useState("");
  const [actionLoading, setActionLoading] = useState(false);

  useEffect(() => {
    async function load() {
      try {
        const [b, u] = await Promise.all([
          api.get<BillingInfo>("/api/billing"),
          api.get<Me>("/api/users/me"),
        ]);
        if (b.plan === "pro") {
          // Plan is active — nothing to fix here
          router.replace("/d");
          return;
        }
        setBilling(b);
        setMe(u);
      } catch (err) {
        if (err instanceof ApiError && err.status === 401) {
          router.replace("/login");
          return;
        }
        setError("Failed to load billing status. Please try again.");
      }
    }
    load();
  }, [router]);

  async function startCheckout() {
    setActionLoading(true);
    setError("");
    try {
      const res = await api.post<{ url: string }>("/api/billing/checkout");
      const parsed = new URL(res.url);
      if (parsed.protocol !== "https:" || parsed.hostname !== "checkout.stripe.com")
        throw new Error("Invalid checkout URL");
      window.location.href = res.url;
    } catch {
      setError("Failed to start checkout. Please try again.");
      setActionLoading(false);
    }
  }

  async function openPortal() {
    setActionLoading(true);
    setError("");
    try {
      const res = await api.post<{ url: string }>("/api/billing/portal");
      window.location.href = res.url;
    } catch {
      setError("Failed to open the billing portal. Please try again.");
      setActionLoading(false);
    }
  }

  if (!billing || !me) {
    return (
      <div className="flex items-center justify-center py-16">
        {error ? (
          <p className="text-sm text-destructive">{error}</p>
        ) : (
          <Spinner className="h-8 w-8" />
        )}
      </div>
    );
  }

  const isAdmin = me.role === "admin";
  const expiresAt = billing.plan_expires_at
    ? new Date(billing.plan_expires_at).toLocaleDateString()
    : null;

  const copy = {
    past_due: {
      title: "Payment failed",
      description: expiresAt
        ? `Your last payment did not go through. Access ends on ${expiresAt} unless the payment method is updated.`
        : "Your last payment did not go through. Please update your payment method.",
    },
    cancelled: {
      title: "Subscription cancelled",
      description: expiresAt
        ? `Your subscription is cancelled. Access ends on ${expiresAt}.`
        : "Your subscription is cancelled.",
    },
    free: {
      title: "Subscription expired",
      description:
        "Your workspace no longer has an active subscription. Reactivate to regain access to your mail.",
    },
  }[billing.plan] ?? {
    title: "Subscription required",
    description: "Your workspace needs an active subscription.",
  };

  const stillHasAccess = billing.plan === "past_due" || billing.plan === "cancelled";

  return (
    <Card>
      <CardHeader className="items-center text-center">
        <div className="h-12 w-12 rounded-full bg-primary/10 flex items-center justify-center mb-2">
          <CreditCard className="h-6 w-6 text-primary" />
        </div>
        <CardTitle>{copy.title}</CardTitle>
        <CardDescription>{copy.description}</CardDescription>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {isAdmin ? (
          <>
            {billing.plan === "past_due" ? (
              <Button onClick={openPortal} disabled={actionLoading} className="w-full">
                {actionLoading ? "Redirecting..." : "Update payment method"}
              </Button>
            ) : (
              <Button onClick={startCheckout} disabled={actionLoading} className="w-full">
                {actionLoading ? "Redirecting..." : "Reactivate subscription"}
              </Button>
            )}
          </>
        ) : (
          <p className="text-sm text-muted-foreground text-center">
            Please ask your workspace admin to reactivate the subscription.
          </p>
        )}
        {stillHasAccess && (
          <Button variant="ghost" className="w-full" onClick={() => router.push("/d")}>
            Back to inbox
          </Button>
        )}
        {error && <p className="text-sm text-destructive text-center">{error}</p>}
      </CardContent>
    </Card>
  );
}
