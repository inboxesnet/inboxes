"use client";

import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/query-client";
import { DomainProvider } from "@/contexts/domain-context";
import { NotificationProvider } from "@/contexts/notification-context";
import { WSSync } from "@/hooks/use-ws-sync";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <NotificationProvider>
        <WSSync />
        <DomainProvider>{children}</DomainProvider>
      </NotificationProvider>
    </QueryClientProvider>
  );
}
