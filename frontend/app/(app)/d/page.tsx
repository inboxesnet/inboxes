"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api, ApiError } from "@/lib/api";
import { Spinner } from "@/components/ui/spinner";
import type { Domain } from "@/lib/types";

export default function DomainRedirectPage() {
  const router = useRouter();
  const [failed, setFailed] = useState(false);

  const redirect = useCallback(async () => {
    setFailed(false);
    try {
      // noLatch: always make a real request here. A stale session latch
      // must not turn a plan problem into a bounce to /login.
      const domains = await api.get<Domain[]>("/api/domains", { noLatch: true });
      if (domains && domains.length > 0) {
        router.replace(`/d/${domains[0].id}/inbox`);
      } else {
        router.replace("/onboarding");
      }
    } catch (err) {
      // 402 = plan expired/invalid, not an auth failure — send the user
      // to the billing page, never back to login (avoids a redirect loop)
      if (err instanceof ApiError && err.status === 402) {
        router.replace("/billing");
      } else if (err instanceof ApiError && err.status === 401) {
        router.replace("/login");
      } else {
        // Network or server error — show a retry instead of looping
        setFailed(true);
      }
    }
  }, [router]);

  useEffect(() => {
    redirect();
  }, [redirect]);

  if (failed) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 text-sm text-muted-foreground">
        <p>Failed to load your domains.</p>
        <button
          onClick={redirect}
          className="text-primary underline underline-offset-4"
        >
          Try again
        </button>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center h-full">
      <Spinner className="h-8 w-8" />
    </div>
  );
}
