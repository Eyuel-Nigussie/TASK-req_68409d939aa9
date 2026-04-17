import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OrderTimeline } from "./OrderTimeline";
import type { OrderView } from "../api/client";

function mk(status: string, events: Array<{ From: string; To: string }> = []): OrderView {
  return {
    ID: "o1",
    Status: status,
    PlacedAt: new Date().toISOString(),
    UpdatedAt: new Date().toISOString(),
    TotalCents: 1000,
    Events: events.map((e, i) => ({
      ID: "e" + i,
      At: new Date().toISOString(),
      From: e.From,
      To: e.To,
      Actor: "u1",
    })),
  };
}

describe("OrderTimeline", () => {
  it("highlights the active status", () => {
    render(<OrderTimeline order={mk("picking", [{ From: "placed", To: "picking" }])} />);
    const active = screen.getByText("picking");
    expect(active.className).toContain("active");
  });

  it("marks earlier steps done", () => {
    render(<OrderTimeline order={mk("dispatched", [
      { From: "placed", To: "picking" },
      { From: "picking", To: "dispatched" },
    ])} />);
    const placed = screen.getByText("placed");
    expect(placed.className).toContain("done");
  });

  it("shows exception status for refund", () => {
    render(<OrderTimeline order={mk("refunded", [
      { From: "delivered", To: "refunded" },
    ])} />);
    const refund = screen.getByText("refunded");
    expect(refund.className).toContain("exception");
  });
});
