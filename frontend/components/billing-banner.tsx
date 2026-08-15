"use client";

import Link from "next/link";
import { useQuery } from "@tanstack/react-query";
import { AlertTriangle } from "lucide-react";
import { api } from "@/lib/api";
import { queryKeys } from "@/lib/query-keys";
import { useAppConfig } from "@/contexts/app-config-context";
import type { BillingInfo } from "@/lib/types";

function daysLeft(expiresAt: string | null): number | null {
  if (!expiresAt) return null;
  const ms = new Date(expiresAt).getTime() - Date.now();
  if (Number.isNaN(ms)) return null;
  return Math.max(0, Math.ceil(ms / (24 * 60 * 60 * 1000)));
}

// Persistent plan-state banner. Shows during the past_due grace window,
// after a cancellation (until access ends), and in read-only lapsed mode.
export function BillingBanner() {
  const { commercial } = useAppConfig();

  const { data } = useQuery({
    queryKey: queryKeys.billing.info(),
    queryFn: () => api.get<BillingInfo>("/api/billing"),
    enabled: commercial,
    staleTime: 60 * 1000,
  });

  if (!commercial || !data || data.plan === "pro") return null;

  let message: React.ReactNode = null;
  let action = "Update payment";
  let tone = "bg-amber-500/15 text-amber-900 dark:text-amber-200 border-amber-500/30";

  if (data.plan === "past_due") {
    const days = daysLeft(data.plan_expires_at);
    message =
      days !== null
        ? `Payment failed. Update your payment method within ${days} day${days === 1 ? "" : "s"} to keep full access.`
        : "Payment failed. Update your payment method to keep full access.";
  } else if (data.plan === "cancelled") {
    const days = daysLeft(data.plan_expires_at);
    action = "Reactivate";
    message =
      days !== null
        ? `Subscription cancelled. Access ends in ${days} day${days === 1 ? "" : "s"}.`
        : "Subscription cancelled.";
  } else if (data.read_only) {
    action = "Reactivate";
    tone = "bg-destructive/10 text-destructive border-destructive/30";
    message =
      "Your plan expired. This workspace is read-only: you can read mail, but you cannot send or change it.";
  } else {
    return null;
  }

  return (
    <div
      role="alert"
      className={`flex print:hidden items-center justify-center gap-2 border-b px-4 py-1.5 text-sm ${tone}`}
    >
      <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
      <span>{message}</span>
      <Link
        href="/billing"
        className="font-medium underline underline-offset-4 shrink-0"
      >
        {action}
      </Link>
    </div>
  );
}
