"use client";

import { useState } from "react";
import { Menu } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Sheet, SheetTrigger, SheetContent } from "@/components/ui/sheet";
import { AppSidebar } from "@/components/app-sidebar";
import { ComposeModal } from "@/components/compose-modal";
import { ToastProvider } from "@/components/ui/toast";

interface AppShellProps {
  user: {
    name: string;
    email: string;
    role: string;
  };
  children: React.ReactNode;
}

export function AppShell({ user, children }: AppShellProps) {
  const [mobileOpen, setMobileOpen] = useState(false);
  const [composeOpen, setComposeOpen] = useState(false);

  return (
    <ToastProvider>
      <div className="flex h-screen">
        {/* Desktop sidebar */}
        <aside className="hidden w-64 border-r bg-background md:block">
          <AppSidebar user={user} onCompose={() => setComposeOpen(true)} />
        </aside>

        {/* Mobile sidebar */}
        <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
          <SheetContent side="left" className="w-64 p-0">
            <AppSidebar
              user={user}
              onNavigate={() => setMobileOpen(false)}
              onCompose={() => setComposeOpen(true)}
            />
          </SheetContent>
        </Sheet>

        {/* Main content */}
        <div className="flex flex-1 flex-col overflow-hidden">
          {/* Mobile header */}
          <header className="flex h-14 items-center border-b px-4 md:hidden">
            <Sheet open={mobileOpen} onOpenChange={setMobileOpen}>
              <SheetTrigger asChild>
                <Button variant="ghost" size="icon">
                  <Menu className="h-5 w-5" />
                  <span className="sr-only">Toggle menu</span>
                </Button>
              </SheetTrigger>
            </Sheet>
            <h1 className="ml-3 text-lg font-semibold">Inboxes.net</h1>
          </header>

          {/* Page content */}
          <main className="flex-1 overflow-y-auto p-4 md:p-6">
            {children}
          </main>
        </div>

        {/* Compose modal */}
        <ComposeModal open={composeOpen} onOpenChange={setComposeOpen} />
      </div>
    </ToastProvider>
  );
}
