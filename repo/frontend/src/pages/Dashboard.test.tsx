import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { Dashboard } from "./Dashboard";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      listExceptions: vi.fn(async () => [
        { OrderID: "o1", Kind: "picking_timeout", DetectedAt: "2024-01-01T00:00:00Z", Description: "stuck" },
      ]),
      listOrders: vi.fn(async () => [
        { ID: "o1", Status: "placed", PlacedAt: "2024-01-01T00:00:00Z", UpdatedAt: "", TotalCents: 1200, Events: [] },
      ]),
    },
  };
});
import { api } from "../api/client";

describe("Dashboard", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("renders exceptions and recent orders", async () => {
    render(<MemoryRouter><Dashboard /></MemoryRouter>);
    await waitFor(() => expect(api.listExceptions).toHaveBeenCalled());
    expect(screen.getByText(/picking_timeout/i)).toBeInTheDocument();
    expect(screen.getByText(/\$12\.00/)).toBeInTheDocument();
  });

  it("shows empty states when nothing is returned", async () => {
    (api.listExceptions as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    (api.listOrders as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce([]);
    render(<MemoryRouter><Dashboard /></MemoryRouter>);
    await waitFor(() => expect(screen.getByText(/no active exceptions/i)).toBeInTheDocument());
    expect(screen.getByText(/no orders yet/i)).toBeInTheDocument();
  });
});
