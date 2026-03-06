import { describe, it, expect } from "vitest";
import { hasLabel, threadBelongsInView } from "../types";

describe("hasLabel", () => {
  it("returns true when label exists", () => {
    expect(hasLabel({ labels: ["inbox", "starred"] }, "inbox")).toBe(true);
  });

  it("returns false when label missing", () => {
    expect(hasLabel({ labels: ["inbox"] }, "trash")).toBe(false);
  });

  it("returns false when labels undefined", () => {
    expect(hasLabel({}, "inbox")).toBe(false);
    expect(hasLabel({ labels: undefined }, "inbox")).toBe(false);
  });
});

describe("threadBelongsInView", () => {
  it("trash view: only threads with trash label", () => {
    expect(threadBelongsInView({ labels: ["trash"] }, "trash")).toBe(true);
    expect(threadBelongsInView({ labels: ["inbox"] }, "trash")).toBe(false);
  });

  it("spam view: only threads with spam label", () => {
    expect(threadBelongsInView({ labels: ["spam"] }, "spam")).toBe(true);
    expect(threadBelongsInView({ labels: ["inbox"] }, "spam")).toBe(false);
  });

  it("inbox view: has inbox AND NOT trash/spam", () => {
    expect(threadBelongsInView({ labels: ["inbox"] }, "inbox")).toBe(true);
    expect(threadBelongsInView({ labels: ["inbox", "starred"] }, "inbox")).toBe(
      true
    );
  });

  it("archive view: NOT inbox AND NOT trash AND NOT spam", () => {
    expect(threadBelongsInView({ labels: [] }, "archive")).toBe(true);
    expect(threadBelongsInView({ labels: ["starred"] }, "archive")).toBe(true);
    expect(threadBelongsInView({ labels: ["inbox"] }, "archive")).toBe(false);
    expect(threadBelongsInView({ labels: ["trash"] }, "archive")).toBe(false);
    expect(threadBelongsInView({ labels: ["spam"] }, "archive")).toBe(false);
  });

  it("inbox + trash → NOT in inbox (trash takes precedence)", () => {
    expect(
      threadBelongsInView({ labels: ["inbox", "trash"] }, "inbox")
    ).toBe(false);
  });
});
