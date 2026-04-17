import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LabPage } from "./Lab";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      listSamples: vi.fn(async () => [
        { ID: "s1", Status: "in_testing", CollectedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z", TestCodes: ["GLU"] },
      ]),
      listReports: vi.fn(async () => [
        { ID: "r1", SampleID: "s1", Version: 1, Status: "issued", Title: "CBC", Narrative: "", Measurements: [], AuthorID: "u1", IssuedAt: "2024-01-01T00:00:00Z" } as any,
      ]),
      listArchivedReports: vi.fn(async () => []),
      searchReports: vi.fn(async () => []),
      createSample: vi.fn(async (b) => ({ ID: "snew", Status: "sampling", CollectedAt: "", UpdatedAt: "", TestCodes: b.test_codes })),
      transitionSample: vi.fn(async (id, to) => ({ ID: id, Status: to, CollectedAt: "", UpdatedAt: "", TestCodes: ["GLU"] })),
      createReport: vi.fn(async () => ({ ID: "rnew", SampleID: "s1", Version: 1, Status: "issued", Title: "x", Narrative: "", Measurements: [], AuthorID: "u1" } as any)),
      archiveReport: vi.fn(async () => ({} as any)),
    },
  };
});
import { api } from "../api/client";

const Rendered = () => (
  <MemoryRouter>
    <LabPage />
  </MemoryRouter>
);

describe("LabPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });
  afterEach(() => vi.restoreAllMocks());

  it("lists samples and reports", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listSamples).toHaveBeenCalled());
    expect(screen.getByText(/s1/)).toBeInTheDocument();
    expect(screen.getByText(/CBC/)).toBeInTheDocument();
  });

  it("opens the new sample form and rejects empty test codes", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listSamples).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /new sample/i }));
    fireEvent.click(screen.getByRole("button", { name: /submit sample/i }));
    expect(screen.getByText(/at least one test code/i)).toBeInTheDocument();
  });

  it("creates a sample with valid test codes", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listSamples).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /new sample/i }));
    const inputs = screen.getAllByRole("textbox");
    // test_codes is the third textbox (order_id, customer_id, test_codes, notes)
    fireEvent.change(inputs[2], { target: { value: "GLU" } });
    fireEvent.click(screen.getByRole("button", { name: /submit sample/i }));
    await waitFor(() => expect(api.createSample).toHaveBeenCalled());
  });

  it("advances a sample status", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listSamples).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /→ reported/i }));
    await waitFor(() => expect(api.transitionSample).toHaveBeenCalledWith("s1", "reported"));
  });

  it("opens the archive modal and rejects empty note", async () => {
    render(<Rendered />);
    await waitFor(() => expect(api.listReports).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /^archive$/i }));
    // Archive button is disabled until note is non-empty; verify.
    const modalArchive = screen.getAllByRole("button", { name: /^archive$/i });
    const submitBtn = modalArchive[modalArchive.length - 1];
    expect((submitBtn as HTMLButtonElement).disabled).toBe(true);
  });
});
