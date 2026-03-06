import { describe, it, expect } from "vitest";
import {
  toggleStarredLabel,
  toggleMutedLabel,
} from "../use-threads";

describe("toggleStarredLabel", () => {
  it("adds starred when absent", () => {
    expect(toggleStarredLabel(["inbox"])).toContain("starred");
  });

  it("removes starred when present", () => {
    expect(toggleStarredLabel(["inbox", "starred"])).not.toContain("starred");
  });

  it("preserves other labels when adding", () => {
    const result = toggleStarredLabel(["inbox", "sent"]);
    expect(result).toContain("inbox");
    expect(result).toContain("sent");
    expect(result).toContain("starred");
  });

  it("preserves other labels when removing", () => {
    const result = toggleStarredLabel(["inbox", "starred", "sent"]);
    expect(result).toContain("inbox");
    expect(result).toContain("sent");
    expect(result).not.toContain("starred");
  });

  it("handles empty array", () => {
    const result = toggleStarredLabel([]);
    expect(result).toEqual(["starred"]);
  });
});

describe("toggleMutedLabel", () => {
  it("adds muted when absent", () => {
    expect(toggleMutedLabel(["inbox"])).toContain("muted");
  });

  it("removes muted when present", () => {
    expect(toggleMutedLabel(["inbox", "muted"])).not.toContain("muted");
  });

  it("preserves other labels when adding", () => {
    const result = toggleMutedLabel(["inbox", "starred"]);
    expect(result).toContain("inbox");
    expect(result).toContain("starred");
    expect(result).toContain("muted");
  });

  it("preserves other labels when removing", () => {
    const result = toggleMutedLabel(["inbox", "muted", "starred"]);
    expect(result).toContain("inbox");
    expect(result).toContain("starred");
    expect(result).not.toContain("muted");
  });

  it("handles empty array", () => {
    const result = toggleMutedLabel([]);
    expect(result).toEqual(["muted"]);
  });
});
