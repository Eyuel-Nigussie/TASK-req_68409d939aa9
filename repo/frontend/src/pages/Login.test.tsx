import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter, Routes, Route } from "react-router-dom";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { AuthProvider } from "../hooks/useAuth";
import { LoginPage } from "./Login";

vi.mock("../api/client", async () => {
  const actual = await vi.importActual<typeof import("../api/client")>("../api/client");
  return {
    ...actual,
    api: {
      ...actual.api,
      login: vi.fn(),
      whoami: vi.fn(async () => ({ id: "u1", username: "a", role: "admin", expires_at: "" })),
    },
  };
});
import { api } from "../api/client";

function ui() {
  return render(
    <MemoryRouter initialEntries={["/login"]}>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/" element={<div>HOME</div>} />
        </Routes>
      </AuthProvider>
    </MemoryRouter>,
  );
}

describe("LoginPage", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.clearAllMocks();
  });
  afterEach(() => vi.restoreAllMocks());

  it("rejects short passwords without contacting the server", () => {
    ui();
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: "alice" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "short" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    expect(screen.getByRole("alert")).toHaveTextContent(/at least 10/i);
    expect(api.login).not.toHaveBeenCalled();
  });

  it("surfaces API errors", async () => {
    (api.login as unknown as ReturnType<typeof vi.fn>).mockRejectedValueOnce(new Error("bad creds"));
    ui();
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: "alice" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "long-enough-12" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => expect(screen.getByRole("alert")).toHaveTextContent(/bad creds/));
  });

  it("navigates to the intended destination on success", async () => {
    (api.login as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      token: "tok", user: { id: "u1", username: "alice", role: "admin" }, expires_at: "",
    });
    ui();
    fireEvent.change(screen.getByLabelText(/username/i), { target: { value: "alice" } });
    fireEvent.change(screen.getByLabelText(/password/i), { target: { value: "long-enough-12" } });
    fireEvent.click(screen.getByRole("button", { name: /sign in/i }));
    await waitFor(() => expect(screen.getByText("HOME")).toBeInTheDocument());
  });
});
