"use client";

import { useState, useEffect, useRef } from "react";
import { toast } from "sonner";
import { api } from "@/lib/api";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent } from "@/components/ui/dialog";
import { Spinner } from "@/components/ui/spinner";
import { CreditCard } from "lucide-react";
import type { User } from "@/lib/types";

export function PaymentWall() {
  const [open, setOpen] = useState(false);
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(false);
  const checkoutInFlight = useRef(false);
  const [polling, setPolling] = useState(false);

  // Suppress paywall after successful checkout — poll for plan activation
  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get("billing") === "success") {
      setPolling(true);
      let attempts = 0;
      const maxAttempts = 15;
      const interval = setInterval(async () => {
        attempts++;
        try {
          const billing = await api.get<{ plan_status: string }>("/api/billing");
          if (billing.plan_status === "pro" || billing.plan_status === "active") {
            clearInterval(interval);
            setPolling(false);
            // Clean up URL
            const url = new URL(window.location.href);
            url.searchParams.delete("billing");
            window.history.replaceState({}, "", url.toString());
            return;
          }
        } catch { /* ignore */ }
        if (attempts >= maxAttempts) {
          clearInterval(interval);
          setPolling(false);
        }
      }, 2000);
      return () => clearInterval(interval);
    }
  }, []);

  useEffect(() => {
    function handlePaymentRequired() {
      // Don't show paywall while polling for checkout completion
      if (polling) return;
      setOpen(true);
      api.get<User>("/api/users/me").then(setUser).catch(() => { toast.error("Failed to load user info"); });
    }
    window.addEventListener("payment-required", handlePaymentRequired);
    return () => window.removeEventListener("payment-required", handlePaymentRequired);
  }, [polling]);

  async function handleUpgrade() {
    // Prevent double checkout
    if (checkoutInFlight.current) return;
    checkoutInFlight.current = true;
    setLoading(true);
    try {
      const res = await api.post<{ url: string }>("/api/billing/checkout");
      const parsed = new URL(res.url);
      if (parsed.protocol !== "https:" || parsed.hostname !== "checkout.stripe.com") throw new Error("Invalid checkout URL");
      window.location.href = res.url;
    } catch {
      toast.error("Failed to start checkout. Please try again.");
      checkoutInFlight.current = false;
      setLoading(false);
    }
  }

  if (polling) {
    return (
      <Dialog open={true} onOpenChange={() => {}}>
        <DialogContent className="sm:max-w-md">
          <div className="flex flex-col items-center gap-4 py-4 text-center">
            <Spinner className="h-6 w-6" />
            <p className="text-sm text-muted-foreground">
              Your payment is being processed...
            </p>
          </div>
        </DialogContent>
      </Dialog>
    );
  }

  return (
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="sm:max-w-md">
        <div className="flex flex-col items-center gap-4 py-4 text-center">
          <div className="h-12 w-12 rounded-full bg-primary/10 flex items-center justify-center">
            <CreditCard className="h-6 w-6 text-primary" />
          </div>
          <h2 className="text-lg font-semibold">Upgrade Required</h2>
          {user?.role === "admin" ? (
            <>
              <p className="text-sm text-muted-foreground">
                Your workspace needs an active subscription to use this feature.
              </p>
              <Button onClick={handleUpgrade} disabled={loading} className="w-full">
                {loading ? "Redirecting..." : "Upgrade to Pro"}
              </Button>
            </>
          ) : (
            <p className="text-sm text-muted-foreground">
              Your workspace needs an active subscription. Please ask your admin to upgrade.
            </p>
          )}
          <Button variant="ghost" size="sm" onClick={() => setOpen(false)}>
            Dismiss
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}
