import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { ReportView } from "../api/client";
import { ReportWorkspace } from "./ReportWorkspace";

// Mock the api module before importing the component's usage.
vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      correctReport: vi.fn(),
    },
  };
});

import { api } from "../api/client";

function makeReport(overrides: Partial<ReportView> = {}): ReportView {
  return {
    ID: "r1",
    SampleID: "s1",
    Version: 1,
    Status: "issued",
    Title: "CBC",
    Narrative: "narr",
    Measurements: [
      { TestCode: "GLU", Value: 85, Flag: "normal" },
      { TestCode: "LIP", Value: 200, Flag: "high" },
    ],
    AuthorID: "u1",
    ...overrides,
  };
}

describe("ReportWorkspace", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("highlights abnormal measurements in red", () => {
    render(<ReportWorkspace report={makeReport()} />);
    const rows = screen.getAllByRole("row");
    // Header + 2 data rows.
    expect(rows.length).toBe(3);
    // LIP row should have abnormal class.
    const lipRow = rows.find((r) => r.textContent?.includes("LIP"))!;
    expect(lipRow.className).toContain("abnormal");
  });

  it("shows superseded banner and hides Correct button for superseded reports", () => {
    render(<ReportWorkspace report={makeReport({ Status: "superseded" })} />);
    expect(screen.getByText(/has been superseded/i)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /correct/i })).not.toBeInTheDocument();
  });

  it("requires reason before submitting correction", async () => {
    const onUpdated = vi.fn();
    render(<ReportWorkspace report={makeReport()} onUpdated={onUpdated} />);
    fireEvent.click(screen.getByRole("button", { name: /correct report/i }));
    fireEvent.click(screen.getByRole("button", { name: /issue correction/i }));
    await waitFor(() => expect(screen.getByText(/reason note is required/i)).toBeInTheDocument());
    expect(api.correctReport).not.toHaveBeenCalled();
  });

  it("submits a correction when reason is provided", async () => {
    const updated = makeReport({ ID: "r2", Version: 2 });
    (api.correctReport as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce(updated);
    const onUpdated = vi.fn();
    render(<ReportWorkspace report={makeReport()} onUpdated={onUpdated} />);
    fireEvent.click(screen.getByRole("button", { name: /correct report/i }));
    fireEvent.change(screen.getByPlaceholderText(/clinical correction/i), { target: { value: "typo" } });
    fireEvent.click(screen.getByRole("button", { name: /issue correction/i }));
    await waitFor(() => expect(api.correctReport).toHaveBeenCalled());
    expect(onUpdated).toHaveBeenCalledWith(updated);
  });

  it("surfaces backend errors in the banner", async () => {
    (api.correctReport as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("boom"));
    render(<ReportWorkspace report={makeReport()} />);
    fireEvent.click(screen.getByRole("button", { name: /correct report/i }));
    fireEvent.change(screen.getByPlaceholderText(/clinical correction/i), { target: { value: "r" } });
    fireEvent.click(screen.getByRole("button", { name: /issue correction/i }));
    await waitFor(() => expect(screen.getByText(/boom/)).toBeInTheDocument());
  });
});
