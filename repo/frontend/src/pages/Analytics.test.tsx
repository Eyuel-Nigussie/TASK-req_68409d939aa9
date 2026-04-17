import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AnalyticsPage } from "./Analytics";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      analyticsSummary: vi.fn(async () => ({
        order_status: { placed: 3, picking: 1 },
        sample_status: { sampling: 2 },
        orders_per_day: [{ Day: "2024-01-01", Count: 4 }],
        abnormal_rate: { TotalMeasurements: 10, AbnormalMeasurements: 3, Rate: 0.3 },
        exceptions: { picking_timeout: 1 },
      })),
    },
  };
});
import { api } from "../api/client";

describe("AnalyticsPage", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("fetches and renders KPIs", async () => {
    render(<AnalyticsPage />);
    await waitFor(() => expect(api.analyticsSummary).toHaveBeenCalled());
    expect(screen.getByText(/Orders by status/i)).toBeInTheDocument();
    expect(screen.getByText(/Abnormal result rate/i)).toBeInTheDocument();
    // 30.00% rate is rendered.
    expect(screen.getByText(/30\.00%/)).toBeInTheDocument();
  });

  it("renders error banner on failure", async () => {
    (api.analyticsSummary as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("boom"));
    render(<AnalyticsPage />);
    await waitFor(() => expect(screen.getByText(/boom/)).toBeInTheDocument());
  });
});
