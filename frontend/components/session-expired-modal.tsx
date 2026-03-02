"use client";

import { useState, useEffect } from "react";
import { Button } from "@/components/ui/button";
import { LogIn } from "lucide-react";

export function SessionExpiredModal() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    function handler() {
      setOpen(true);
    }
    window.addEventListener("session-expired", handler);
    return () => window.removeEventListener("session-expired", handler);
  }, []);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-background">
      <div className="w-full max-w-md rounded-lg border bg-background p-6 shadow-lg">
        <div className="flex flex-col items-center gap-4 py-4 text-center">
          <div className="h-12 w-12 rounded-full bg-primary/10 flex items-center justify-center">
            <LogIn className="h-6 w-6 text-primary" />
          </div>
          <h2 className="text-lg font-semibold">Session Expired</h2>
          <p className="text-sm text-muted-foreground">
            Your session has expired. Please log in again to continue.
          </p>
          <Button
            onClick={() => (window.location.href = "/login")}
            className="w-full"
          >
            Log in
          </Button>
        </div>
      </div>
    </div>
  );
}
