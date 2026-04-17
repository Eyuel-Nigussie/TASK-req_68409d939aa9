import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OrderDetailPage } from "./OrderDetail";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      getOrder: vi.fn(),
      transitionOrder: vi.fn(async (id, to) => ({ ID: id, Status: to, PlacedAt: "", UpdatedAt: "", TotalCents: 500, Events: [] })),
      updateInventory: vi.fn(async (id) => ({ ID: id, Status: "placed", PlacedAt: "", UpdatedAt: "", TotalCents: 500 })),
    },
  };
});
import { api } from "../api/client";

function rendered() {
  return render(
    <MemoryRouter initialEntries={["/orders/o1"]}>
      <Routes>
        <Route path="/orders/:id" element={<OrderDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("OrderDetailPage", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("renders the order on success with transition buttons", async () => {
    (api.getOrder as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ID: "o1", Status: "placed", PlacedAt: "2024-01-01T00:00:00Z", UpdatedAt: "", TotalCents: 5000, Events: [],
    });
    rendered();
    await waitFor(() => expect(screen.getByText(/#o1/i)).toBeInTheDocument());
    expect(screen.getByRole("button", { name: /→ picking/i })).toBeInTheDocument();
  });

  it("opens the refund modal with an existing delivered status and validates", async () => {
    (api.getOrder as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ID: "o1", Status: "delivered", PlacedAt: "", UpdatedAt: "", TotalCents: 500, Events: [],
    });
    rendered();
    await waitFor(() => expect(screen.getByText(/#o1/i)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /→ refunded/i }));
    expect(screen.getByText(/refund reason required/i)).toBeInTheDocument();
    const confirm = screen.getByRole("button", { name: /confirm refund/i });
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
  });

  it("flags inventory via the modal", async () => {
    (api.getOrder as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ID: "o1", Status: "picking", PlacedAt: "", UpdatedAt: "", TotalCents: 500, Events: [],
    });
    rendered();
    await waitFor(() => expect(screen.getByText(/#o1/i)).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /flag inventory/i }));
    fireEvent.change(screen.getByLabelText(/^SKU$/i), { target: { value: "A" } });
    // Use the Save button inside the modal.
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    await waitFor(() => expect(api.updateInventory).toHaveBeenCalledWith("o1", "A", true, ""));
  });

  it("renders an error banner on fetch failure", async () => {
    (api.getOrder as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("nope"));
    rendered();
    await waitFor(() => expect(screen.getByText(/nope/)).toBeInTheDocument());
  });
});
