import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AdminPage } from "./Admin";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      adminListUsers: vi.fn(async () => [
        { id: "u1", username: "admin", role: "admin", disabled: false, created_at: "", lock_until: "", failures: 0 },
      ]),
      adminCreateUser: vi.fn(),
      adminUpdateUser: vi.fn(async () => ({ id: "u1", role: "admin", disabled: true })),
      adminListRefRanges: vi.fn(async () => [
        { TestCode: "GLU", Units: "mg/dL", Demographic: "", LowNormal: 70, HighNormal: 99 },
      ]),
      adminPutRefRanges: vi.fn(async () => ({ count: 1 })),
      adminListRoutes: vi.fn(async () => [{ FromID: "A", ToID: "B", Miles: 5 }]),
      adminPutRoutes: vi.fn(async () => ({ count: 1 })),
      adminListPermissions: vi.fn(async () => [
        { ID: "analytics.view", Description: "View analytics" },
      ]),
      adminListRolePermissions: vi.fn(async () => [
        { Role: "admin", PermissionID: "analytics.view" },
      ]),
      adminSetRolePermissions: vi.fn(async () => ({ role: "front_desk", permissions: [] })),
      getMapConfig: vi.fn(async () => ({ map_image_url: "" })),
      adminPutMapConfig: vi.fn(async () => ({ map_image_url: "/static/map.png" })),
    },
  };
});
import { api } from "../api/client";

function withFetchStub() {
  vi.stubGlobal("fetch", vi.fn(async () => ({
    ok: true,
    json: async () => [],
  } as unknown as Response)));
}

describe("AdminPage", () => {
  beforeEach(() => {
    withFetchStub();
    vi.clearAllMocks();
  });
  afterEach(() => vi.restoreAllMocks());

  it("loads all admin data and shows the users table", async () => {
    render(<AdminPage />);
    await waitFor(() => expect(api.adminListUsers).toHaveBeenCalled());
    // The users table has at least one row with the seeded admin.
    expect(screen.getAllByText("admin").length).toBeGreaterThan(0);
    expect(screen.getByText(/Role permissions/i)).toBeInTheDocument();
  });

  it("rejects new-user with short password", async () => {
    render(<AdminPage />);
    await waitFor(() => expect(api.adminListUsers).toHaveBeenCalled());
    // Username + short password.
    const usernameInput = screen.getAllByRole("textbox")[0];
    fireEvent.change(usernameInput, { target: { value: "newuser" } });
    fireEvent.click(screen.getByRole("button", { name: /create user/i }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/≥10 characters/i));
    expect(api.adminCreateUser).not.toHaveBeenCalled();
  });

  it("toggles disable for a user", async () => {
    render(<AdminPage />);
    await waitFor(() => expect(api.adminListUsers).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /^disable$/i }));
    await waitFor(() => expect(api.adminUpdateUser).toHaveBeenCalledWith("u1", { disabled: true }));
  });

  it("saves role permissions from the matrix", async () => {
    render(<AdminPage />);
    await waitFor(() => expect(api.adminListRolePermissions).toHaveBeenCalled());
    const saveButtons = screen.getAllByRole("button", { name: /^save$/i });
    fireEvent.click(saveButtons[0]);
    await waitFor(() => expect(api.adminSetRolePermissions).toHaveBeenCalled());
  });

  it("saves reference ranges", async () => {
    render(<AdminPage />);
    await waitFor(() => expect(api.adminListRefRanges).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /save ranges/i }));
    await waitFor(() => expect(api.adminPutRefRanges).toHaveBeenCalled());
  });

  it("saves route table", async () => {
    render(<AdminPage />);
    await waitFor(() => expect(api.adminListRoutes).toHaveBeenCalled());
    fireEvent.click(screen.getByRole("button", { name: /save route table/i }));
    await waitFor(() => expect(api.adminPutRoutes).toHaveBeenCalled());
  });

  it("persists the service-area map image URL", async () => {
    render(<AdminPage />);
    await waitFor(() => expect(api.getMapConfig).toHaveBeenCalled());
    const input = screen.getByLabelText(/map image url/i);
    fireEvent.change(input, { target: { value: "/static/map.png" } });
    fireEvent.click(screen.getByRole("button", { name: /save map image/i }));
    await waitFor(() => expect(api.adminPutMapConfig).toHaveBeenCalledWith("/static/map.png"));
  });
});
