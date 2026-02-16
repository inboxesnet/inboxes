"use client";

import { cn, getInitials, getDomainColor } from "@/lib/utils";

interface DomainIconProps {
  domain: string;
  active?: boolean;
  hasUnread?: boolean;
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
  size = "md",
  onClick,
}: DomainIconProps) {
  return (
    <div className="relative group">
      <button
        onClick={onClick}
        className={cn(
          "flex items-center justify-center font-semibold text-white transition-all duration-200",
          getDomainColor(domain),
          sizeClasses[size],
          active ? "rounded-2xl" : "rounded-[24px] hover:rounded-2xl"
        )}
        title={domain}
      >
        {getInitials(domain)}
      </button>

      {/* Active indicator pill */}
      {active && (
        <div className="absolute left-0 top-1/2 -translate-x-[calc(100%+4px)] -translate-y-1/2 w-1 h-8 bg-foreground rounded-r-full" />
      )}

      {/* Unread dot */}
      {hasUnread && !active && (
        <div className="absolute -bottom-0.5 -right-0.5 w-3 h-3 bg-red-500 rounded-full border-2 border-background" />
      )}

      {/* Tooltip */}
      <div className="absolute left-full ml-3 top-1/2 -translate-y-1/2 px-2 py-1 bg-foreground text-background text-xs rounded whitespace-nowrap opacity-0 group-hover:opacity-100 pointer-events-none transition-opacity z-50">
        {domain}
      </div>
    </div>
  );
}
