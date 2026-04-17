import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CustomerDetailPage } from "./CustomerDetail";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      getCustomer: vi.fn(),
    },
  };
});
import { api } from "../api/client";

function rendered(id = "c1") {
  return render(
    <MemoryRouter initialEntries={[`/customers/${id}`]}>
      <Routes>
        <Route path="/customers/:id" element={<CustomerDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("CustomerDetailPage", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("renders the customer on success", async () => {
    (api.getCustomer as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      id: "c1", name: "Jane", identifier: "ID", street: "1 Main", city: "C", state: "CA", zip: "1",
      phone: "555", email: "j@example.com", tags: ["vip"], created_at: "2024-01-01T00:00:00Z", updated_at: "",
    });
    rendered();
    await waitFor(() => expect(screen.getByText("Jane")).toBeInTheDocument());
    expect(screen.getByText("vip")).toBeInTheDocument();
  });

  it("surfaces an error banner when the fetch fails", async () => {
    (api.getCustomer as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("gone"));
    rendered();
    await waitFor(() => expect(screen.getByText(/gone/)).toBeInTheDocument());
  });
});
