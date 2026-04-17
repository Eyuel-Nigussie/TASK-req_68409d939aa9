import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, getToken, request, setToken, api } from "./client";

// Minimal fetch stub that returns a specified response shape.
function stubFetch(status: number, body: unknown, headers: Record<string, string> = {}) {
  const text = body === undefined ? "" : JSON.stringify(body);
  return vi.fn(async () => ({
    status,
    ok: status >= 200 && status < 300,
    text: async () => text,
    headers: new Headers(headers),
  } as unknown as Response));
}

describe("api/client", () => {
  beforeEach(() => {
    localStorage.clear();
  });
  afterEach(() => {
    vi.restoreAllMocks();
    localStorage.clear();
  });

  it("getToken / setToken round-trip", () => {
    expect(getToken()).toBeNull();
    setToken("abc");
    expect(getToken()).toBe("abc");
    setToken(null);
    expect(getToken()).toBeNull();
  });

  it("request attaches workstation + client time headers and bearer token", async () => {
    setToken("tok-1");
    const fetchMock = stubFetch(200, { ok: true });
    vi.stubGlobal("fetch", fetchMock);
    const got = await request<{ ok: boolean }>("GET", "/api/x");
    expect(got.ok).toBe(true);
    const args = (fetchMock.mock.calls as unknown as Array<[string, RequestInit]>)[0];
    const headers = args[1].headers as Record<string, string>;
    expect(headers.Authorization).toBe("Bearer tok-1");
    expect(headers["X-Workstation"]).toMatch(/^ws-/);
    expect(headers["X-Workstation-Time"]).toMatch(/T/); // RFC3339 has a T
  });

  it("request returns undefined for 204 No Content", async () => {
    vi.stubGlobal("fetch", stubFetch(204, undefined));
    const r = await request<void>("DELETE", "/api/x");
    expect(r).toBeUndefined();
  });

  it("request raises ApiError for non-2xx with message from body", async () => {
    vi.stubGlobal("fetch", stubFetch(404, { message: "not found" }));
    await expect(request("GET", "/api/missing")).rejects.toBeInstanceOf(ApiError);
  });

  it("request falls back to HTTP status when body has no message", async () => {
    vi.stubGlobal("fetch", stubFetch(500, {}));
    try {
      await request("GET", "/api/err");
      throw new Error("should have thrown");
    } catch (e) {
      expect(e).toBeInstanceOf(ApiError);
      expect((e as ApiError).status).toBe(500);
      expect((e as Error).message).toContain("500");
    }
  });

  it("api.login posts the body and surfaces the token payload", async () => {
    vi.stubGlobal("fetch", stubFetch(200, {
      token: "tok", user: { id: "u1", username: "a", role: "admin" }, expires_at: "1",
    }));
    const r = await api.login("a", "b");
    expect(r.token).toBe("tok");
  });

  it("workstation id persists across calls", async () => {
    const fetchMock = stubFetch(200, { ok: true });
    vi.stubGlobal("fetch", fetchMock);
    await request("GET", "/api/a");
    await request("GET", "/api/b");
    const calls = fetchMock.mock.calls as unknown as Array<[string, RequestInit]>;
    const h1 = (calls[0][1].headers as Record<string, string>)["X-Workstation"];
    const h2 = (calls[1][1].headers as Record<string, string>)["X-Workstation"];
    expect(h1).toBe(h2);
  });
});
