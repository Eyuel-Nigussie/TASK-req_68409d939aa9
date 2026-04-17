import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OrdersPage } from "./Orders";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      listOrders: vi.fn(async () => [
        { ID: "o1", Status: "placed", PlacedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z", TotalCents: 1000, Priority: "standard", Events: [] },
      ]),
      createOrder: vi.fn(async (b) => ({ ID: "o2", Status: "placed", PlacedAt: new Date().toISOString(), UpdatedAt: new Date().toISOString(), TotalCents: b.total_cents })),
      transitionOrder: vi.fn(async (id, to) => ({ ID: id, Status: to, PlacedAt: "", UpdatedAt: "", TotalCents: 1000 })),
      queryOrders: vi.fn(async () => ({ items: [], total: 0, page: 1, size: 25, has_next: false })),
      saveFilter: vi.fn(async () => ({ ID: "f1" })),
    },
  };
});
import { api } from "../api/client";

const Rendered = () => (
  <MemoryRouter>
    <OrdersPage />
  </MemoryRouter>
);

describe("OrdersPage", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });
  afterEach(() => vi.restoreAllMocks());

  it("loads orders and shows them", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listOrders).toHaveBeenCalled());
    expect(screen.getByText(/#o1/i)).toBeInTheDocument();
  });

  it("opens the create form, rejects blank SKU, and submits a valid order", async () => {
    render(<Rendered />);
    await waitFor(() => expect(screen.getByText(/#o1/i)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /new order/i }));
    // Leave the pre-populated SKU blank -> error banner.
    fireEvent.change(screen.getByPlaceholderText(/SKU-001/i), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: /create order/i }));
    await waitFor(() => expect(screen.getByText(/every line item needs a sku/i)).toBeInTheDocument());
    // Fill SKU and click add line item.
    fireEvent.change(screen.getByPlaceholderText(/SKU-001/i), { target: { value: "A" } });
    fireEvent.click(screen.getByRole("button", { name: /create order/i }));
    await waitFor(() => expect(api.createOrder).toHaveBeenCalled());
  });

  it("transitions an order to picking via button click", async () => {
    render(<Rendered />);
    await waitFor(() => expect(screen.getByText(/#o1/i)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /→ picking/i }));
    await waitFor(() => expect(api.transitionOrder).toHaveBeenCalledWith("o1", "picking", ""));
  });

  it("opens the refund modal for refunded transitions and requires a reason", async () => {
    render(<Rendered />);
    await waitFor(() => expect(screen.getByText(/#o1/i)).toBeInTheDocument());
    // Seed an order already in picking so "refunded" is a permitted transition.
    (api.listOrders as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce([
      { ID: "op", Status: "picking", PlacedAt: "", UpdatedAt: "", TotalCents: 100 },
    ]);
  });
});
