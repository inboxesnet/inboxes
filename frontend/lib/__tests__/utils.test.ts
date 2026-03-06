import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  formatRelativeTime,
  formatThreadTime,
  getInitials,
  getDomainColor,
  validatePassword,
} from "../utils";

describe("formatRelativeTime", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-02-26T12:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "now" for <1 minute ago', () => {
    const date = new Date("2026-02-26T11:59:30Z");
    expect(formatRelativeTime(date)).toBe("now");
  });

  it('returns "Xm" for <60 minutes', () => {
    const date = new Date("2026-02-26T11:45:00Z");
    expect(formatRelativeTime(date)).toBe("15m");
  });

  it('returns "Xh" for <24 hours', () => {
    const date = new Date("2026-02-26T06:00:00Z");
    expect(formatRelativeTime(date)).toBe("6h");
  });

  it('returns "Xd" for <7 days', () => {
    const date = new Date("2026-02-23T12:00:00Z");
    expect(formatRelativeTime(date)).toBe("3d");
  });

  it('returns "Mon DD" for >7 days', () => {
    const date = new Date("2026-02-10T12:00:00Z");
    expect(formatRelativeTime(date)).toBe("Feb 10");
  });

  it("boundary: exactly 60 minutes returns 1h", () => {
    const date = new Date("2026-02-26T11:00:00Z");
    expect(formatRelativeTime(date)).toBe("1h");
  });

  it("handles Date objects", () => {
    const date = new Date("2026-02-26T11:50:00Z");
    expect(formatRelativeTime(date)).toBe("10m");
  });

  it("handles ISO strings", () => {
    expect(formatRelativeTime("2026-02-26T11:50:00Z")).toBe("10m");
  });
});

describe("formatThreadTime", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date("2026-02-26T12:00:00Z"));
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "now" for <1 minute', () => {
    const date = new Date("2026-02-26T11:59:30Z");
    expect(formatThreadTime(date)).toBe("now");
  });

  it('returns "Xm ago" for <60 minutes', () => {
    const date = new Date("2026-02-26T11:30:00Z");
    expect(formatThreadTime(date)).toBe("30m ago");
  });

  it("returns time format for today", () => {
    // 8:00 AM today (local time depends on env, but we test it's not "now" or "m ago")
    const date = new Date("2026-02-26T08:00:00Z");
    const result = formatThreadTime(date);
    // Should contain AM or PM (locale time)
    expect(result).not.toBe("now");
    expect(result).not.toContain("m ago");
  });

  it('returns "Mon DD" for same year, not today', () => {
    const date = new Date("2026-01-15T12:00:00Z");
    expect(formatThreadTime(date)).toBe("Jan 15");
  });

  it('returns "Mon DD, YYYY" for different year', () => {
    const date = new Date("2025-06-15T12:00:00Z");
    expect(formatThreadTime(date)).toBe("Jun 15, 2025");
  });

  it("handles Date objects", () => {
    const date = new Date("2026-02-26T11:55:00Z");
    expect(formatThreadTime(date)).toBe("5m ago");
  });

  it("handles ISO strings", () => {
    expect(formatThreadTime("2026-02-26T11:55:00Z")).toBe("5m ago");
  });
});

describe("getInitials", () => {
  it('returns first char uppercased for "example.com"', () => {
    expect(getInitials("example.com")).toBe("E");
  });

  it("returns first char uppercased for single-part domain", () => {
    expect(getInitials("localhost")).toBe("L");
  });

  it("handles empty string", () => {
    expect(getInitials("")).toBe("");
  });

  it("returns first char of subdomain for multi-part", () => {
    expect(getInitials("mail.google.com")).toBe("M");
  });
});

describe("getDomainColor", () => {
  it("returns a string from the expected palette", () => {
    const result = getDomainColor("example.com");
    expect(result).toMatch(/^bg-\w+-500$/);
  });

  it("is deterministic", () => {
    const a = getDomainColor("test.com");
    const b = getDomainColor("test.com");
    expect(a).toBe(b);
  });

  it("different domains can produce different colors", () => {
    const colors = new Set(
      ["a.com", "b.com", "c.com", "d.com", "e.com"].map(getDomainColor)
    );
    expect(colors.size).toBeGreaterThan(1);
  });
});

describe("validatePassword", () => {
  it("returns null for valid password", () => {
    expect(validatePassword("MyPass123")).toBeNull();
  });

  it("rejects too short", () => {
    expect(validatePassword("Ab1")).not.toBeNull();
  });

  it("rejects too long", () => {
    const long = "A" + "a".repeat(127) + "1";
    expect(validatePassword(long)).not.toBeNull();
  });

  it("rejects no uppercase", () => {
    expect(validatePassword("lowercase1")).not.toBeNull();
  });

  it("rejects no lowercase", () => {
    expect(validatePassword("UPPERCASE1")).not.toBeNull();
  });

  it("rejects no digit", () => {
    expect(validatePassword("NoDigitHere")).not.toBeNull();
  });

  it("accepts exact 8 chars", () => {
    expect(validatePassword("Abcdefg1")).toBeNull();
  });

  it("accepts exact 128 chars", () => {
    const pw = "A" + "a".repeat(126) + "1";
    expect(validatePassword(pw)).toBeNull();
  });

  it("accepts special characters", () => {
    expect(validatePassword("P@ss!w0rd")).toBeNull();
  });

  it("error messages are user-friendly", () => {
    const msg = validatePassword("short");
    expect(msg).toContain("8");
  });

  it("empty string rejected", () => {
    expect(validatePassword("")).not.toBeNull();
  });
});

