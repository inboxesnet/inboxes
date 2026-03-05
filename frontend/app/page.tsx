"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { Spinner } from "@/components/ui/spinner";

interface AppConfig {
  commercial: boolean;
}

interface SetupStatus {
  needs_setup: boolean;
}

export default function Home() {
  const router = useRouter();

  useEffect(() => {
    async function check() {
      try {
        const config = await api.get<AppConfig>("/api/config");
        if (config.commercial) {
          router.replace("/signup");
          return;
        }

        // Self-hosted: check if setup is needed
        const status = await api.get<SetupStatus>("/api/setup/status");
        if (status.needs_setup) {
          router.replace("/setup");
        } else {
          router.replace("/login");
        }
      } catch {
        router.replace("/login");
      }
    }
    check();
  }, [router]);

  return (
    <div className="min-h-screen flex items-center justify-center">
      <Spinner className="h-6 w-6" />
    </div>
  );
}
