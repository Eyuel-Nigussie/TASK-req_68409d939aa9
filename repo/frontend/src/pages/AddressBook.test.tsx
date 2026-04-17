import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AddressBookPage } from "./AddressBook";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      listAddressBook: vi.fn(async () => [
        { id: "a1", label: "home", street: "1 Main", city: "C", zip: "11" },
      ]),
      saveAddress: vi.fn(async () => ({ id: "a2", label: "work" })),
      deleteAddress: vi.fn(async () => {}),
      customersByAddress: vi.fn(async () => [
        { id: "c1", name: "Jane", street: "1 Main", city: "C", state: "", zip: "11" } as any,
      ]),
      ordersByAddress: vi.fn(async () => [
        { ID: "o1", Status: "placed", PlacedAt: "2024-01-01T00:00:00Z", UpdatedAt: "", TotalCents: 100, DeliveryStreet: "1 Main" },
      ]),
    },
  };
});
import { api } from "../api/client";

const Rendered = () => (
  <MemoryRouter>
    <AddressBookPage />
  </MemoryRouter>
);

describe("AddressBookPage", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });
  afterEach(() => vi.restoreAllMocks());

  it("lists saved addresses", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listAddressBook).toHaveBeenCalled());
    expect(screen.getByText("home")).toBeInTheDocument();
  });

  it("rejects lookup without city or zip", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listAddressBook).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /find customers/i }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/enter at least/i));
    expect(api.customersByAddress).not.toHaveBeenCalled();
  });

  it("finds customers by address", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listAddressBook).toHaveBeenCalled());
    const inputs = screen.getAllByRole("textbox");
    fireEvent.change(inputs[2], { target: { value: "11" } }); // zip field
    fireEvent.click(screen.getByRole("button", { name: /find customers/i }));
    await waitFor(() => expect(screen.getByText(/Matching customers/i)).toBeInTheDocument());
  });

  it("finds jobs by address", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listAddressBook).toHaveBeenCalled());
    const inputs = screen.getAllByRole("textbox");
    fireEvent.change(inputs[2], { target: { value: "11" } });
    fireEvent.click(screen.getByRole("button", { name: /find jobs/i }));
    await waitFor(() => expect(screen.getByText(/Matching jobs/i)).toBeInTheDocument());
  });

  it("saves a new address and refreshes", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listAddressBook).toHaveBeenCalled());
    const labelInput = screen.getByPlaceholderText(/save this address as/i);
    fireEvent.change(labelInput, { target: { value: "work" } });
    fireEvent.click(screen.getByRole("button", { name: /^save$/i }));
    await waitFor(() => expect(api.saveAddress).toHaveBeenCalled());
  });

  it("deletes a saved address", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listAddressBook).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /delete/i }));
    await waitFor(() => expect(api.deleteAddress).toHaveBeenCalled());
  });
});
