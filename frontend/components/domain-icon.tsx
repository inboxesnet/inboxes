"use client";

import { cn, getInitials, getDomainColor } from "@/lib/utils";

interface DomainIconProps {
  domain: string;
  active?: boolean;
  hasUnread?: boolean;
  status?: string;
  size?: "sm" | "md" | "lg";
  onClick?: () => void;
}

const sizeClasses = {
  sm: "h-8 w-8 text-xs",
  md: "h-12 w-12 text-base",
  lg: "h-16 w-16 text-xl",
};

export function DomainIcon({
  domain,
  active = false,
  hasUnread = false,
  status,
  size = "md",
  onClick,
}: DomainIconProps) {
  const degraded = status === "disconnected" || status === "pending";
  const statusLabel =
    status === "disconnected"
      ? " (disconnected)"
      : status === "pending"
        ? " (pending verification)"
        : "";
  return (
    <div className="relative group">
      <button
        onClick={onClick}
        className={cn(
          "flex items-center justify-center font-semibold text-white transition-all duration-200",
          getDomainColor(domain),
          sizeClasses[size],
          degraded && "opacity-50 saturate-50",
          active ? "rounded-2xl ring-2 ring-foreground ring-offset-2 ring-offset-background" : "rounded-[24px] hover:rounded-2xl"
        )}
        title={domain + statusLabel}
      >
        {getInitials(domain)}
      </button>

      {/* Status dot: disconnected wins over pending, which wins over unread */}
      {status === "disconnected" ? (
        <div
          className="absolute -bottom-0.5 -right-0.5 w-3 h-3 bg-destructive rounded-full border-2 border-background"
          aria-label={`${domain} is disconnected`}
        />
      ) : status === "pending" ? (
        <div
          className="absolute -bottom-0.5 -right-0.5 w-3 h-3 bg-amber-500 rounded-full border-2 border-background"
          aria-label={`${domain} is pending verification`}
        />
      ) : hasUnread ? (
        <div className="absolute -bottom-0.5 -right-0.5 w-3 h-3 bg-red-500 rounded-full border-2 border-background" />
      ) : null}

      {/* Tooltip */}
      <div className="absolute left-full ml-3 top-1/2 -translate-y-1/2 px-2 py-1 bg-foreground text-background text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity z-50">
        {domain}
        {statusLabel}
      </div>
    </div>
  );
}
