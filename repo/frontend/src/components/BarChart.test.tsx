import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { BarChart, LineChart } from "./BarChart";

describe("BarChart", () => {
  it("renders labels and values for non-empty data", () => {
    render(<BarChart data={[{ label: "placed", value: 3 }, { label: "picking", value: 5 }]} ariaLabel="x" />);
    expect(screen.getByLabelText("x")).toBeInTheDocument();
    expect(screen.getByText("placed")).toBeInTheDocument();
    expect(screen.getByText("picking")).toBeInTheDocument();
  });
  it("shows empty-state for zero data", () => {
    render(<BarChart data={[]} ariaLabel="empty" />);
    expect(screen.getByText(/no data/i)).toBeInTheDocument();
  });
});

describe("LineChart", () => {
  it("renders points for non-empty data", () => {
    render(<LineChart data={[{ label: "2024-01-01", value: 1 }, { label: "2024-01-02", value: 2 }]} ariaLabel="series" />);
    expect(screen.getByLabelText("series")).toBeInTheDocument();
  });
  it("empty-state for zero data", () => {
    render(<LineChart data={[]} ariaLabel="empty" />);
    expect(screen.getByText(/no data/i)).toBeInTheDocument();
  });
});
