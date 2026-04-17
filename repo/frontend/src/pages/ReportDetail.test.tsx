import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ReportDetailPage } from "./ReportDetail";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      getReport: vi.fn(),
    },
  };
});
import { api } from "../api/client";

function rendered() {
  return render(
    <MemoryRouter initialEntries={["/reports/r1"]}>
      <Routes>
        <Route path="/reports/:id" element={<ReportDetailPage />} />
      </Routes>
    </MemoryRouter>,
  );
}

describe("ReportDetailPage", () => {
  beforeEach(() => vi.clearAllMocks());
  afterEach(() => vi.restoreAllMocks());

  it("loads and renders the report", async () => {
    (api.getReport as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ID: "r1", SampleID: "s1", Version: 1, Status: "issued", Title: "CBC", Narrative: "n",
      Measurements: [{ TestCode: "GLU", Value: 85, Flag: "normal" }], AuthorID: "u1",
    });
    rendered();
    await waitFor(() => expect(screen.getByText(/CBC/)).toBeInTheDocument());
  });

  it("shows an error banner on failure", async () => {
    (api.getReport as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("gone"));
    rendered();
    await waitFor(() => expect(screen.getByText(/gone/)).toBeInTheDocument());
  });
});
