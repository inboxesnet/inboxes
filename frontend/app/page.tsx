"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { api } from "@/lib/api";
import { MarketingPage } from "@/components/marketing-page";
import { Spinner } from "@/components/ui/spinner";

interface AppConfig {
  commercial: boolean;
}

interface SetupStatus {
  needs_setup: boolean;
}

export default function Home() {
  const router = useRouter();
  const [showMarketing, setShowMarketing] = useState(false);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    async function check() {
      try {
        const config = await api.get<AppConfig>("/api/config");
        if (config.commercial) {
          setShowMarketing(true);
          setLoading(false);
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
        // Fallback to login on error
        router.replace("/login");
      }
    }
    check();
  }, [router]);

  if (showMarketing) {
    return <MarketingPage />;
  }

  if (loading) {
    return (
      <div className="min-h-screen flex items-center justify-center">
        <Spinner className="h-6 w-6" />
      </div>
    );
  }

  return null;
}
