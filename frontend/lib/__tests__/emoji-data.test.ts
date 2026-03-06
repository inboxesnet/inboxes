import { describe, it, expect } from "vitest";
import { emojiKeys, lookupEmoji, filterEmoji } from "../emoji-data";

describe("emojiKeys", () => {
  it("is a non-empty array", () => {
    expect(Array.isArray(emojiKeys)).toBe(true);
    expect(emojiKeys.length).toBeGreaterThan(0);
  });

  it("is sorted alphabetically", () => {
    const sorted = [...emojiKeys].sort();
    expect(emojiKeys).toEqual(sorted);
  });

  it("contains well-known shortcodes", () => {
    expect(emojiKeys).toContain("smile");
    expect(emojiKeys).toContain("heart");
    expect(emojiKeys).toContain("thumbsup");
    expect(emojiKeys).toContain("fire");
  });

  it("entries are all lowercase strings", () => {
    for (const key of emojiKeys) {
      expect(typeof key).toBe("string");
      expect(key).toBe(key.toLowerCase());
    }
  });
});

describe("lookupEmoji", () => {
  it("returns the emoji for a known shortcode", () => {
    const result = lookupEmoji("smile");
    expect(result).toBeDefined();
    expect(typeof result).toBe("string");
    expect(result!.length).toBeGreaterThan(0);
  });

  it("returns correct emoji for 'heart'", () => {
    expect(lookupEmoji("heart")).toBe("\u2764\uFE0F");
  });

  it("returns correct emoji for 'thumbsup'", () => {
    const result = lookupEmoji("thumbsup");
    expect(result).toBeDefined();
    // thumbsup is the thumbs-up emoji
    expect(result).toBe("\uD83D\uDC4D");
  });

  it("returns undefined for unknown shortcode", () => {
    expect(lookupEmoji("not_a_real_emoji_shortcode_xyz")).toBeUndefined();
  });

  it("returns undefined for empty string", () => {
    expect(lookupEmoji("")).toBeUndefined();
  });
});

describe("filterEmoji", () => {
  it("returns results matching the query prefix", () => {
    const results = filterEmoji("smil");
    expect(results.length).toBeGreaterThan(0);
    for (const r of results) {
      expect(r.shortcode.startsWith("smil")).toBe(true);
    }
  });

  it("each result has shortcode and emoji properties", () => {
    const results = filterEmoji("heart");
    expect(results.length).toBeGreaterThan(0);
    for (const r of results) {
      expect(r).toHaveProperty("shortcode");
      expect(r).toHaveProperty("emoji");
      expect(typeof r.shortcode).toBe("string");
      expect(typeof r.emoji).toBe("string");
    }
  });

  it("respects the default limit of 8", () => {
    // Use a broad query that matches many emojis
    const results = filterEmoji("s");
    expect(results.length).toBeLessThanOrEqual(8);
  });

  it("respects a custom limit", () => {
    const results = filterEmoji("s", 3);
    expect(results.length).toBeLessThanOrEqual(3);
  });

  it("returns empty array when no matches", () => {
    const results = filterEmoji("zzznotanemoji");
    expect(results).toEqual([]);
  });

  it("is case-insensitive (lowercases query)", () => {
    const lower = filterEmoji("smile");
    const upper = filterEmoji("SMILE");
    expect(lower).toEqual(upper);
  });

  it("returns exact match for full shortcode", () => {
    const results = filterEmoji("smile");
    expect(results.length).toBeGreaterThan(0);
    expect(results[0].shortcode).toBe("smile");
  });
});
