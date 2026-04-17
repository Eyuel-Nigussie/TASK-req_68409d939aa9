import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { useRecentSearches } from "./useRecentSearches";

function resetStorage() {
  localStorage.clear();
}

describe("useRecentSearches", () => {
  beforeEach(resetStorage);

  it("adds items to the front and dedupes", () => {
    const { result } = renderHook(() => useRecentSearches("u1"));
    act(() => result.current.add("jane"));
    act(() => result.current.add("john"));
    act(() => result.current.add("jane")); // duplicate is moved to top
    expect(result.current.items).toEqual(["jane", "john"]);
  });

  it("caps at 20 entries", () => {
    const { result } = renderHook(() => useRecentSearches("u1"));
    act(() => {
      for (let i = 0; i < 25; i++) result.current.add(`q${i}`);
    });
    expect(result.current.items).toHaveLength(20);
    expect(result.current.items[0]).toBe("q24");
    expect(result.current.items.at(-1)).toBe("q5");
  });

  it("keeps entries scoped to user id", () => {
    const { result: a } = renderHook(() => useRecentSearches("alice"));
    act(() => a.current.add("jane"));
    const { result: b } = renderHook(() => useRecentSearches("bob"));
    expect(b.current.items).toEqual([]);
  });

  it("persists across mounts", () => {
    const first = renderHook(() => useRecentSearches("u1"));
    act(() => first.result.current.add("persisted"));
    first.unmount();
    const second = renderHook(() => useRecentSearches("u1"));
    expect(second.result.current.items).toEqual(["persisted"]);
  });

  it("clear removes history", () => {
    const { result } = renderHook(() => useRecentSearches("u1"));
    act(() => result.current.add("jane"));
    act(() => result.current.clear());
    expect(result.current.items).toEqual([]);
  });

  it("ignores empty/whitespace", () => {
    const { result } = renderHook(() => useRecentSearches("u1"));
    act(() => result.current.add("   "));
    expect(result.current.items).toEqual([]);
  });
});
