import { act, render, renderHook, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AuthProvider, useAuth } from "./useAuth";

// Mock api/client so we can drive the login/whoami/logout flows.
vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      whoami: vi.fn(async () => ({ id: "u1", username: "alice", role: "admin", expires_at: "" })),
      login: vi.fn(async () => ({
        token: "tok",
        user: { id: "u1", username: "alice", role: "admin" },
        expires_at: "",
      })),
      logout: vi.fn(async () => {}),
    },
  };
});

import { api, setToken } from "../api/client";

beforeEach(() => {
  localStorage.clear();
  vi.clearAllMocks();
});
afterEach(() => vi.restoreAllMocks());

const wrapper = ({ children }: { children: React.ReactNode }) => (
  <AuthProvider>{children}</AuthProvider>
);

describe("useAuth", () => {
  it("is loading while resolving whoami, then populates user", async () => {
    setToken("tok"); // triggers whoami call
    const { result } = renderHook(() => useAuth(), { wrapper });
    expect(result.current.loading).toBe(true);
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.user?.username).toBe("alice");
  });

  it("starts unauthenticated when no token is present", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.user).toBeNull();
  });

  it("login stores token and sets user", async () => {
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));
    await act(async () => {
      await result.current.login("alice", "whatever");
    });
    expect(result.current.user?.username).toBe("alice");
    expect(localStorage.getItem("oops.session.token")).toBe("tok");
  });

  it("logout clears token and user even when api call fails", async () => {
    (api.logout as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("network"));
    setToken("tok");
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.user?.username).toBe("alice"));
    await act(async () => {
      await result.current.logout();
    });
    expect(result.current.user).toBeNull();
    expect(localStorage.getItem("oops.session.token")).toBeNull();
  });

  it("clears stale token when whoami fails", async () => {
    (api.whoami as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("expired"));
    setToken("stale");
    const { result } = renderHook(() => useAuth(), { wrapper });
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(localStorage.getItem("oops.session.token")).toBeNull();
  });

  it("useAuth throws helpful error outside provider", () => {
    // Suppress React's uncaught-error console noise.
    const spy = vi.spyOn(console, "error").mockImplementation(() => {});
    function Broken() {
      useAuth();
      return null;
    }
    expect(() => render(<Broken />)).toThrow(/AuthProvider/);
    spy.mockRestore();
    // Ensure screen is at least importable without side effects.
    expect(screen).toBeDefined();
  });
});
