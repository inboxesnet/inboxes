"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { Spinner } from "@/components/ui/spinner";
import type { Domain } from "@/lib/types";

export default function DomainRedirectPage() {
  const router = useRouter();

  useEffect(() => {
    async function redirect() {
      try {
        const domains = await api.get<Domain[]>("/api/domains");
        if (domains && domains.length > 0) {
          router.replace(`/d/${domains[0].id}/inbox`);
        } else {
          router.replace("/onboarding");
        }
      } catch {
        router.replace("/login");
      }
    }
    redirect();
  }, [router]);

  return (
    <div className="flex items-center justify-center h-screen">
      <Spinner className="h-8 w-8" />
    </div>
  );
}
