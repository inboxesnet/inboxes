"use client";

import { QueryClientProvider } from "@tanstack/react-query";
import { queryClient } from "@/lib/query-client";
import { AppConfigProvider } from "@/contexts/app-config-context";
import { DomainProvider } from "@/contexts/domain-context";
import { NotificationProvider } from "@/contexts/notification-context";
import { PreferencesProvider } from "@/contexts/preferences-context";
import { BillingBanner } from "@/components/billing-banner";
import { PaymentWall } from "@/components/payment-wall";
import { SessionExpiredModal } from "@/components/session-expired-modal";
import { WSSync } from "@/hooks/use-ws-sync";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <QueryClientProvider client={queryClient}>
      <AppConfigProvider>
        <NotificationProvider>
          <PreferencesProvider>
            <WSSync />
            <PaymentWall />
            <SessionExpiredModal />
            <DomainProvider>
              <div className="flex h-screen flex-col">
                <BillingBanner />
                <div className="min-h-0 flex-1">{children}</div>
              </div>
            </DomainProvider>
          </PreferencesProvider>
        </NotificationProvider>
      </AppConfigProvider>
    </QueryClientProvider>
  );
}
