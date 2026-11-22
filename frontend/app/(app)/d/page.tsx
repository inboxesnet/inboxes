"use client";

import { useCallback, useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api, ApiError } from "@/lib/api";
import { Spinner } from "@/components/ui/spinner";
import type { Domain } from "@/lib/types";

export default function DomainRedirectPage() {
  const router = useRouter();
  const [failed, setFailed] = useState(false);
  const [noAccess, setNoAccess] = useState(false);

  const redirect = useCallback(async () => {
    setFailed(false);
    try {
      // noLatch: always make a real request here. A stale session latch
      // must not turn a plan problem into a bounce to /login.
      const domains = await api.get<Domain[]>("/api/domains", { noLatch: true });
      if (domains && domains.length > 0) {
        router.replace(`/d/${domains[0].id}/inbox`);
      } else {
        // A member with no assigned alias has no domain. Only an admin can
        // do the onboarding.
        const me = await api.get<{ role: string }>("/api/users/me", { noLatch: true });
        if (me?.role === "admin") {
          router.replace("/onboarding");
        } else {
          setNoAccess(true);
        }
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

  if (noAccess) {
    return (
      <div className="flex flex-col items-center justify-center h-full gap-3 px-4 text-center text-sm text-muted-foreground">
        <p>No inbox is assigned to you. Ask your admin to add you to an alias.</p>
        <button
          onClick={async () => {
            try { await api.post("/api/auth/logout"); } catch { /* go to login in each case */ }
            window.location.href = "/login";
          }}
          className="text-primary underline underline-offset-4"
        >
          Log out
        </button>
      </div>
    );
  }

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
