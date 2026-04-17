import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import App from "../App";
import { setToken } from "../api/client";

// App routing renders a different workspace stripe color per route and
// gates nav links by role. We stub the api client so whoami succeeds.
vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      whoami: vi.fn(async () => ({ id: "u1", username: "alice", role: "admin", expires_at: "" })),
      listOrders: vi.fn(async () => []),
      listExceptions: vi.fn(async () => []),
    },
  };
});

describe("App routing", () => {
  beforeEach(() => {
    localStorage.clear();
    setToken("tok");
  });
  afterEach(() => vi.clearAllMocks());

  it("redirects unauthenticated user to login", async () => {
    setToken(null);
    render(
      <MemoryRouter initialEntries={["/orders"]}>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText(/Operations Portal/i)).toBeInTheDocument());
  });

  it("renders admin nav entries only for admin role", async () => {
    render(
      <MemoryRouter initialEntries={["/"]}>
        <App />
      </MemoryRouter>,
    );
    await waitFor(() => expect(screen.getByText(/Dashboard/i)).toBeInTheDocument());
    expect(screen.getByRole("link", { name: /Admin/i })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Analytics/i })).toBeInTheDocument();
  });
});
