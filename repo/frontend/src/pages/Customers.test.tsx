import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { CustomersPage } from "./Customers";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      searchCustomers: vi.fn(async () => [
        { id: "c1", name: "Jane", created_at: "2024-01-01T00:00:00Z", updated_at: "" },
      ]),
      createCustomer: vi.fn(async () => ({ id: "c2", name: "Bob", created_at: "", updated_at: "" })),
    },
  };
});
import { api } from "../api/client";

describe("CustomersPage", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("rejects a registration with no name", async () => {
    render(<MemoryRouter><CustomersPage /></MemoryRouter>);
    await waitFor(() => expect(api.searchCustomers).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /^register$/i }));
    expect(screen.getByRole("alert")).toHaveTextContent(/name is required/i);
    expect(api.createCustomer).not.toHaveBeenCalled();
  });

  it("registers a customer and prepends to the list", async () => {
    render(<MemoryRouter><CustomersPage /></MemoryRouter>);
    await waitFor(() => expect(api.searchCustomers).toHaveBeenCalled());
    const inputs = screen.getAllByRole("textbox");
    fireEvent.change(inputs[0], { target: { value: "Bob" } }); // name
    fireEvent.click(screen.getByRole("button", { name: /^register$/i }));
    await waitFor(() => expect(api.createCustomer).toHaveBeenCalled());
  });

  it("searches and shows a result", async () => {
    render(<MemoryRouter><CustomersPage /></MemoryRouter>);
    await waitFor(() => expect(api.searchCustomers).toHaveBeenCalled());
    expect(screen.getByText("Jane")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^search$/i }));
    await waitFor(() => expect(api.searchCustomers).toHaveBeenCalledTimes(2));
  });
});
