import React from "react";
import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { EmailWindowProvider, useEmailWindow } from "../email-window-context";
import type { Draft } from "@/lib/types";

function wrapper({ children }: { children: React.ReactNode }) {
  return <EmailWindowProvider>{children}</EmailWindowProvider>;
}

describe("useEmailWindow", () => {
  it("throws outside provider", () => {
    expect(() => {
      renderHook(() => useEmailWindow());
    }).toThrow("useEmailWindow must be used within EmailWindowProvider");
  });

  it("initial state is closed with null data", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    expect(result.current.composeState).toBe("closed");
    expect(result.current.composeData).toBeNull();
    expect(result.current.currentDraftId).toBeUndefined();
  });

  it("openCompose transitions to open", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose();
    });
    expect(result.current.composeState).toBe("open");
    expect(result.current.composeData).toEqual({});
  });

  it("openCompose with data sets composeData", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose({
        subject: "Hello",
        toAddresses: ["test@example.com"],
      });
    });
    expect(result.current.composeState).toBe("open");
    expect(result.current.composeData?.subject).toBe("Hello");
    expect(result.current.composeData?.toAddresses).toEqual([
      "test@example.com",
    ]);
  });

  it("minimize/restore cycle works", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose({ subject: "Test" });
    });
    expect(result.current.composeState).toBe("open");

    act(() => {
      result.current.minimizeCompose();
    });
    expect(result.current.composeState).toBe("minimized");

    act(() => {
      result.current.restoreCompose();
    });
    expect(result.current.composeState).toBe("open");
    // Data should be preserved
    expect(result.current.composeData?.subject).toBe("Test");
  });

  it("closeCompose clears data and sets closed", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose({ subject: "Test" });
    });
    act(() => {
      result.current.closeCompose();
    });
    expect(result.current.composeState).toBe("closed");
    expect(result.current.composeData).toBeNull();
  });

  it("openDraft populates compose data from draft", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    const draft: Draft = {
      id: "draft-1",
      domain_id: "d1",
      kind: "compose",
      subject: "Draft Subject",
      from_address: "me@test.com",
      to_addresses: ["to@test.com"],
      cc_addresses: [],
      bcc_addresses: [],
      body_html: "<p>Hi</p>",
      body_plain: "Hi",
      attachment_ids: ["att-1"],
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    act(() => {
      result.current.openDraft(draft);
    });
    expect(result.current.composeState).toBe("open");
    expect(result.current.composeData?.subject).toBe("Draft Subject");
    expect(result.current.composeData?.fromAddress).toBe("me@test.com");
    expect(result.current.composeData?.toAddresses).toEqual(["to@test.com"]);
    expect(result.current.composeData?.attachmentIds).toEqual(["att-1"]);
  });

  it("currentDraftId tracks draftId from composeData", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    const draft: Draft = {
      id: "draft-42",
      domain_id: "d1",
      kind: "compose",
      subject: "",
      from_address: "",
      to_addresses: [],
      cc_addresses: [],
      bcc_addresses: [],
      body_html: "",
      body_plain: "",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    act(() => {
      result.current.openDraft(draft);
    });
    expect(result.current.currentDraftId).toBe("draft-42");
  });

  it("full lifecycle: open → minimize → restore → close", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });

    // open
    act(() => {
      result.current.openCompose({ subject: "Lifecycle" });
    });
    expect(result.current.composeState).toBe("open");

    // minimize
    act(() => {
      result.current.minimizeCompose();
    });
    expect(result.current.composeState).toBe("minimized");
    expect(result.current.composeData?.subject).toBe("Lifecycle");

    // restore
    act(() => {
      result.current.restoreCompose();
    });
    expect(result.current.composeState).toBe("open");

    // close
    act(() => {
      result.current.closeCompose();
    });
    expect(result.current.composeState).toBe("closed");
    expect(result.current.composeData).toBeNull();
  });

  it("re-open after close starts fresh", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose({ subject: "First" });
    });
    act(() => {
      result.current.closeCompose();
    });
    act(() => {
      result.current.openCompose();
    });
    expect(result.current.composeData).toEqual({});
    expect(result.current.composeData?.subject).toBeUndefined();
  });

  it("openCompose without data defaults to empty object", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose();
    });
    expect(result.current.composeData).toEqual({});
  });

  it("openDraft sets bodyHtml and bodyPlain", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    const draft: Draft = {
      id: "d1",
      domain_id: "dom1",
      kind: "reply",
      subject: "Re: Hello",
      from_address: "a@b.com",
      to_addresses: [],
      cc_addresses: [],
      bcc_addresses: [],
      body_html: "<b>bold</b>",
      body_plain: "bold",
      created_at: "2026-01-01T00:00:00Z",
      updated_at: "2026-01-01T00:00:00Z",
    };
    act(() => {
      result.current.openDraft(draft);
    });
    expect(result.current.composeData?.bodyHtml).toBe("<b>bold</b>");
    expect(result.current.composeData?.bodyPlain).toBe("bold");
  });

  it("minimizeCompose preserves composeData", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose({ subject: "Keep me" });
    });
    act(() => {
      result.current.minimizeCompose();
    });
    expect(result.current.composeData?.subject).toBe("Keep me");
  });

  it("openCompose replaces existing compose data", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose({ subject: "First" });
    });
    act(() => {
      result.current.openCompose({ subject: "Second" });
    });
    expect(result.current.composeData?.subject).toBe("Second");
  });

  it("currentDraftId is undefined for non-draft composes", () => {
    const { result } = renderHook(() => useEmailWindow(), { wrapper });
    act(() => {
      result.current.openCompose({ subject: "New" });
    });
    expect(result.current.currentDraftId).toBeUndefined();
  });
});
