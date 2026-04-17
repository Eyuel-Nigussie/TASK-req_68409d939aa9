import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DispatchPage } from "./Dispatch";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      listRegions: vi.fn(async () => []),
      quoteFee: vi.fn(async () => ({ region_id: "zoneA", miles: 4, method: "route_table", fee_cents: 600, fee_usd: 6 })),
      validatePin: vi.fn(async () => ({ valid: true, region_id: "zoneA" })),
    },
  };
});
import { api } from "../api/client";

describe("DispatchPage", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("renders the pin + fee-quote sections", async () => {
    render(<DispatchPage />);
    await waitFor(() => expect(api.listRegions).toHaveBeenCalled());
    expect(screen.getByText(/Service-area pin/i)).toBeInTheDocument();
    expect(screen.getByText(/Fee quote/i)).toBeInTheDocument();
  });

  it("submits a fee quote and shows the result", async () => {
    render(<DispatchPage />);
    await waitFor(() => expect(api.listRegions).toHaveBeenCalled());
    fireEvent.change(screen.getByPlaceholderText(/depot-A/i), { target: { value: "depot" } });
    fireEvent.change(screen.getByPlaceholderText(/drop-42/i), { target: { value: "drop" } });
    fireEvent.click(screen.getByRole("button", { name: /quote fee/i }));
    await waitFor(() => expect(screen.getByText(/route_table/)).toBeInTheDocument());
    expect(screen.getByText(/\$6\.00/)).toBeInTheDocument();
  });

  it("shows an error banner when the quote call fails", async () => {
    (api.quoteFee as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("outside"));
    render(<DispatchPage />);
    await waitFor(() => expect(api.listRegions).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /quote fee/i }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/outside/));
  });
});
