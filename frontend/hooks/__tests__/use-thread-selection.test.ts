import { describe, it, expect } from "vitest";
import { renderHook, act } from "@testing-library/react";
import { useThreadSelection } from "../use-thread-selection";

describe("useThreadSelection", () => {
  it("initial state: empty set, allSelected=false, someSelected=false", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    expect(result.current.selectedIds.size).toBe(0);
    expect(result.current.allSelected).toBe(false);
    expect(result.current.someSelected).toBe(false);
  });

  it("toggleSelect: adds id to selectedIds", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelect("a");
    });
    expect(result.current.selectedIds.has("a")).toBe(true);
    expect(result.current.selectedIds.size).toBe(1);
  });

  it("toggleSelect: removes id if already selected", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelect("a");
    });
    act(() => {
      result.current.toggleSelect("a");
    });
    expect(result.current.selectedIds.has("a")).toBe(false);
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("toggleSelectAll: selects all when none selected", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelectAll();
    });
    expect(result.current.selectedIds.size).toBe(3);
    expect(result.current.allSelected).toBe(true);
  });

  it("toggleSelectAll: deselects all when all selected", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelectAll();
    });
    act(() => {
      result.current.toggleSelectAll();
    });
    expect(result.current.selectedIds.size).toBe(0);
    expect(result.current.allSelected).toBe(false);
  });

  it("toggleSelectAll: selects all when some selected (partial → all)", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelect("a");
    });
    expect(result.current.someSelected).toBe(true);
    act(() => {
      result.current.toggleSelectAll();
    });
    expect(result.current.selectedIds.size).toBe(3);
    expect(result.current.allSelected).toBe(true);
  });

  it("clearSelection: empties selectedIds", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelectAll();
    });
    act(() => {
      result.current.clearSelection();
    });
    expect(result.current.selectedIds.size).toBe(0);
  });

  it("allSelected: true only when all ids selected", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b"])
    );
    act(() => {
      result.current.toggleSelect("a");
    });
    expect(result.current.allSelected).toBe(false);
    act(() => {
      result.current.toggleSelect("b");
    });
    expect(result.current.allSelected).toBe(true);
  });

  it("someSelected: true when some but not all", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelect("a");
    });
    expect(result.current.someSelected).toBe(true);
    act(() => {
      result.current.toggleSelect("b");
      result.current.toggleSelect("c");
    });
    // All selected now — someSelected should be false
    expect(result.current.someSelected).toBe(false);
  });

  it("selectIds: replaces entire selection with provided ids", () => {
    const { result } = renderHook(() =>
      useThreadSelection(["a", "b", "c"])
    );
    act(() => {
      result.current.toggleSelect("a");
    });
    act(() => {
      result.current.selectIds(["b", "c"]);
    });
    expect(result.current.selectedIds.has("a")).toBe(false);
    expect(result.current.selectedIds.has("b")).toBe(true);
    expect(result.current.selectedIds.has("c")).toBe(true);
    expect(result.current.selectedIds.size).toBe(2);
  });

  it("empty threadIds: allSelected=false, someSelected=false", () => {
    const { result } = renderHook(() => useThreadSelection([]));
    expect(result.current.allSelected).toBe(false);
    expect(result.current.someSelected).toBe(false);
  });

  it("selection state persists across re-renders with same threadIds", () => {
    const { result, rerender } = renderHook(
      ({ ids }) => useThreadSelection(ids),
      { initialProps: { ids: ["a", "b", "c"] } }
    );
    act(() => {
      result.current.toggleSelect("a");
    });
    rerender({ ids: ["a", "b", "c"] });
    expect(result.current.selectedIds.has("a")).toBe(true);
  });
});
