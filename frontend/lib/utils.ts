import { type ClassValue, clsx } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

export function formatRelativeTime(date: string | Date): string {
  const now = new Date();
  const d = new Date(date);
  const diffMs = now.getTime() - d.getTime();
  const diffMins = Math.floor(diffMs / 60000);
  const diffHours = Math.floor(diffMs / 3600000);
  const diffDays = Math.floor(diffMs / 86400000);

  if (diffMins < 1) return "now";
  if (diffMins < 60) return `${diffMins}m`;
  if (diffHours < 24) return `${diffHours}h`;
  if (diffDays < 7) return `${diffDays}d`;

  return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
}

export function formatThreadTime(date: string | Date): string {
  const now = new Date();
  const d = new Date(date);
  const diffMs = now.getTime() - d.getTime();
  const diffMins = Math.floor(diffMs / 60000);

  if (diffMins < 1) return "now";
  if (diffMins < 60) return `${diffMins}m ago`;

  const isToday =
    d.getDate() === now.getDate() &&
    d.getMonth() === now.getMonth() &&
    d.getFullYear() === now.getFullYear();
  if (isToday) {
    return d.toLocaleTimeString("en-US", {
      hour: "numeric",
      minute: "2-digit",
      hour12: true,
    });
  }

  const isSameYear = d.getFullYear() === now.getFullYear();
  if (isSameYear) {
    return d.toLocaleDateString("en-US", { month: "short", day: "numeric" });
  }

  return d.toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}

export function getInitials(domain: string): string {
  const parts = domain.split(".");
  if (parts.length >= 2) {
    return parts[0].charAt(0).toUpperCase();
  }
  return domain.charAt(0).toUpperCase();
}

const DOMAIN_COLORS = [
  { bg: "bg-blue-500", text: "text-blue-500" },
  { bg: "bg-green-500", text: "text-green-500" },
  { bg: "bg-purple-500", text: "text-purple-500" },
  { bg: "bg-orange-500", text: "text-orange-500" },
  { bg: "bg-pink-500", text: "text-pink-500" },
  { bg: "bg-teal-500", text: "text-teal-500" },
  { bg: "bg-indigo-500", text: "text-indigo-500" },
  { bg: "bg-rose-500", text: "text-rose-500" },
  { bg: "bg-cyan-500", text: "text-cyan-500" },
  { bg: "bg-amber-500", text: "text-amber-500" },
];

function domainColorIndex(domain: string): number {
  let hash = 0;
  for (let i = 0; i < domain.length; i++) {
    hash = domain.charCodeAt(i) + ((hash << 5) - hash);
  }
  return Math.abs(hash) % DOMAIN_COLORS.length;
}

export function getDomainColor(domain: string): string {
  return DOMAIN_COLORS[domainColorIndex(domain)].bg;
}

export function getDomainTextColor(domain: string): string {
  return DOMAIN_COLORS[domainColorIndex(domain)].text;
}

/**
 * Validates password against backend rules.
 * Returns error message or null if valid.
 */
export function validatePassword(password: string): string | null {
  if (password.length < 8) return "Password must be at least 8 characters";
  if (password.length > 128) return "Password must be 128 characters or fewer";
  if (!/[A-Z]/.test(password)) return "Password must include an uppercase letter";
  if (!/[a-z]/.test(password)) return "Password must include a lowercase letter";
  if (!/[0-9]/.test(password)) return "Password must include a number";
  return null;
}
