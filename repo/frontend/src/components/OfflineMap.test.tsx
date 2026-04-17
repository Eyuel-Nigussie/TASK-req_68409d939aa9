import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { OfflineMap } from "./OfflineMap";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      listRegions: vi.fn(async () => [
        { Polygon: { ID: "zoneA", Vertices: [{ Lat: 0, Lng: 0 }, { Lat: 0, Lng: 10 }, { Lat: 10, Lng: 10 }, { Lat: 10, Lng: 0 }] }, BaseFeeCents: 100, PerMileFeeCents: 10 },
      ]),
      validatePin: vi.fn(async () => ({ valid: true, region_id: "zoneA" })),
      getMapConfig: vi.fn(async () => ({ map_image_url: "" })),
    },
  };
});
import { api } from "../api/client";

describe("OfflineMap", () => {
  beforeEach(() => localStorage.clear());
  afterEach(() => vi.clearAllMocks());

  it("loads regions and renders an svg", async () => {
    render(<OfflineMap />);
    await waitFor(() => expect(api.listRegions).toHaveBeenCalled());
    const svg = screen.getByRole("img", { name: /service-area/i });
    expect(svg).toBeInTheDocument();
  });

  it("validates a pin on click and shows success banner", async () => {
    const onPinned = vi.fn();
    render(<OfflineMap onPinned={onPinned} />);
    await waitFor(() => expect(api.listRegions).toHaveBeenCalled());
    const svg = screen.getByRole("img", { name: /service-area/i });
    // jsdom doesn't compute getBoundingClientRect meaningfully; we still
    // fire a click so the handler path runs.
    fireEvent.click(svg);
    await waitFor(() => expect(api.validatePin).toHaveBeenCalled());
    expect(onPinned).toHaveBeenCalled();
    expect(screen.getByRole("status")).toBeInTheDocument();
  });

  it("shows error banner when pin falls outside the service area", async () => {
    (api.validatePin as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({ valid: false });
    render(<OfflineMap />);
    await waitFor(() => expect(api.listRegions).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("img", { name: /service-area/i }));
    await waitFor(() => expect(screen.getByText(/outside configured service area/i)).toBeInTheDocument());
  });

  it("handles listRegions failure gracefully", async () => {
    (api.listRegions as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("offline"));
    render(<OfflineMap />);
    // The component swallows the error and renders an empty svg still.
    await waitFor(() => expect(api.listRegions).toHaveBeenCalled());
  });

  it("renders the admin-configured map backdrop when a URL is returned", async () => {
    (api.getMapConfig as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      map_image_url: "/static/svc-area.png",
    });
    render(<OfflineMap />);
    const img = await screen.findByTestId("map-backdrop");
    expect(img.getAttribute("href")).toBe("/static/svc-area.png");
  });
});
