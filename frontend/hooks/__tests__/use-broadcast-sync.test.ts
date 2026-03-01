import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { postInvalidation } from "../use-broadcast-sync";

describe("postInvalidation", () => {
  let originalBC: typeof globalThis.BroadcastChannel;

  beforeEach(() => {
    originalBC = globalThis.BroadcastChannel;
  });

  afterEach(() => {
    globalThis.BroadcastChannel = originalBC;
  });

  it("creates BroadcastChannel and posts message with keys", () => {
    const postMessageMock = vi.fn();
    const closeMock = vi.fn();

    globalThis.BroadcastChannel = class MockBC {
      name: string;
      postMessage = postMessageMock;
      close = closeMock;
      onmessage = null;
      onmessageerror = null;
      addEventListener = vi.fn();
      removeEventListener = vi.fn();
      dispatchEvent = vi.fn().mockReturnValue(true);
      constructor(name: string) {
        this.name = name;
      }
    } as unknown as typeof BroadcastChannel;

    const keys = [
      ["threads", "list"],
      ["domains", "unreadCounts"],
    ] as const;
    postInvalidation(keys);

    expect(postMessageMock).toHaveBeenCalledWith({ keys });
    expect(closeMock).toHaveBeenCalled();
  });

  it("no-op when BroadcastChannel is undefined", () => {
    // @ts-expect-error testing undefined
    globalThis.BroadcastChannel = undefined;

    // Should not throw
    expect(() => postInvalidation([["threads"]])).not.toThrow();
  });

  it("closes channel after posting", () => {
    const closeMock = vi.fn();

    globalThis.BroadcastChannel = class MockBC {
      name: string;
      postMessage = vi.fn();
      close = closeMock;
      onmessage = null;
      onmessageerror = null;
      addEventListener = vi.fn();
      removeEventListener = vi.fn();
      dispatchEvent = vi.fn().mockReturnValue(true);
      constructor(name: string) {
        this.name = name;
      }
    } as unknown as typeof BroadcastChannel;

    postInvalidation([["threads"]]);
    expect(closeMock).toHaveBeenCalled();
  });

  it("handles errors gracefully (no throw)", () => {
    globalThis.BroadcastChannel = class ThrowingBC {
      constructor() {
        throw new Error("not supported");
      }
    } as unknown as typeof BroadcastChannel;

    expect(() => postInvalidation([["threads"]])).not.toThrow();
  });
});
