import { describe, it, expect } from "vitest";
import {
  extractSender,
  decodeHtmlEntities,
  cleanSnippet,
  parseParticipants,
} from "../thread-helpers";

describe("extractSender", () => {
  it("returns local part before @ of first email", () => {
    expect(extractSender(["alice@example.com", "bob@test.com"])).toBe("alice");
  });

  it('returns "Unknown" for empty array', () => {
    expect(extractSender([])).toBe("Unknown");
  });

  it('returns "Unknown" for null/undefined', () => {
    expect(extractSender(null as unknown as string[])).toBe("Unknown");
    expect(extractSender(undefined as unknown as string[])).toBe("Unknown");
  });

  it("returns full string when no @ sign", () => {
    expect(extractSender(["noatsign"])).toBe("noatsign");
  });
});

describe("decodeHtmlEntities", () => {
  it("decodes hex entities", () => {
    expect(decodeHtmlEntities("&#x27;")).toBe("'");
  });

  it("decodes decimal entities", () => {
    expect(decodeHtmlEntities("&#39;")).toBe("'");
  });

  it("decodes named entities", () => {
    expect(decodeHtmlEntities("&amp; &lt; &gt; &quot; &apos;")).toBe(
      '& < > " \''
    );
  });

  it("decodes mixed entities in one string", () => {
    expect(decodeHtmlEntities("&amp;#x27; &lt;b&gt;")).toBe("&#x27; <b>");
  });
});

describe("cleanSnippet", () => {
  it("collapses whitespace and trims", () => {
    expect(cleanSnippet("  hello   world  ")).toBe("hello world");
  });

  it("decodes entities before cleaning", () => {
    expect(cleanSnippet("&amp;amp;  test")).toBe("&amp; test");
  });

  it("handles empty string", () => {
    expect(cleanSnippet("")).toBe("");
  });
});

describe("parseParticipants", () => {
  it("returns array input as-is", () => {
    const arr = ["a@b.com", "c@d.com"];
    expect(parseParticipants(arr)).toBe(arr);
  });

  it("parses valid JSON string into array", () => {
    expect(parseParticipants('["a@b.com","c@d.com"]')).toEqual([
      "a@b.com",
      "c@d.com",
    ]);
  });

  it("returns empty array for invalid JSON", () => {
    expect(parseParticipants("not-json")).toEqual([]);
  });

  it("returns empty array for non-string non-array", () => {
    expect(parseParticipants(42 as unknown as string[])).toEqual([]);
  });
});
