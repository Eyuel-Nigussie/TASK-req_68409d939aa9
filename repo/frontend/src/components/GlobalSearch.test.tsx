import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { GlobalSearch } from "./GlobalSearch";
import { AuthProvider } from "../hooks/useAuth";
import { setToken } from "../api/client";

// Mock the api module so we can drive the search suggestion stream
// without a running backend. This covers the audit's coverage-table
// recommendation to add a GlobalSearch integration test.
vi.mock("../api/client", async () => {
  const mod = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...mod,
    api: {
      ...mod.api,
      searchGlobal: vi.fn(async (q: string) => {
        if (!q) return [];
        return [
          { ID: "c1", Label: "Jane Doe", Kind: "customer", Score: 0.9 },
          { ID: "o1", Label: "#123456 placed rush", Kind: "order", Score: 0.85 },
          { ID: "r1", Label: "CBC report", Kind: "report", Score: 0.4 },
        ];
      }),
      whoami: vi.fn(async () => ({ id: "u1", username: "alice", role: "front_desk", expires_at: "" })),
    },
  };
});

function ui() {
  return render(
    <MemoryRouter>
      <AuthProvider>
        <GlobalSearch />
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe("GlobalSearch", () => {
  beforeEach(() => {
    setToken("fake-token");
    localStorage.setItem("oops.recent-searches.u1", JSON.stringify([]));
  });
  afterEach(() => {
    setToken(null);
    localStorage.clear();
    vi.clearAllMocks();
  });

  it("surfaces suggestions across customer/order/report kinds after typing", async () => {
    ui();
    const input = await screen.findByLabelText("Global search");
    fireEvent.change(input, { target: { value: "jane" } });

    await waitFor(() => expect(screen.getByText(/Jane Doe/)).toBeInTheDocument(), {
      timeout: 1000,
    });
    // All three kinds should be visible as suggestions.
    expect(screen.getByText(/Jane Doe/)).toBeInTheDocument();
    expect(screen.getByText(/#123456 placed rush/)).toBeInTheDocument();
    expect(screen.getByText(/CBC report/)).toBeInTheDocument();
  });

  it("records the query into recent-searches when a suggestion is clicked", async () => {
    ui();
    const input = await screen.findByLabelText("Global search");
    fireEvent.change(input, { target: { value: "jane" } });
    const hit = await screen.findByText(/Jane Doe/);
    fireEvent.click(hit);

    const raw = localStorage.getItem("oops.recent-searches.u1") ?? "[]";
    const recent = JSON.parse(raw);
    expect(recent[0]).toBe("jane");
  });

  it("shows recent searches when focused with empty query", async () => {
    localStorage.setItem("oops.recent-searches.u1", JSON.stringify(["alice", "widget"]));
    ui();
    const input = await screen.findByLabelText("Global search");
    fireEvent.focus(input);
    await waitFor(() => expect(screen.getByText(/Recent searches/)).toBeInTheDocument());
    expect(screen.getByText(/alice/)).toBeInTheDocument();
    expect(screen.getByText(/widget/)).toBeInTheDocument();
  });
});
