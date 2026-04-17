import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { Modal } from "./Modal";

describe("Modal", () => {
  it("renders children and calls onClose on Escape and backdrop click", () => {
    const onClose = vi.fn();
    render(
      <Modal title="Test" onClose={onClose} actions={<button>Save</button>}>
        <div>body</div>
      </Modal>,
    );
    expect(screen.getByText("body")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /save/i })).toBeInTheDocument();

    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledTimes(1);

    // Close button
    fireEvent.click(screen.getByLabelText("Close"));
    expect(onClose).toHaveBeenCalledTimes(2);
  });

  it("backdrop click closes but inner click does not", () => {
    const onClose = vi.fn();
    render(
      <Modal title="Test" onClose={onClose}>
        <div data-testid="inner">body</div>
      </Modal>,
    );
    const dialog = screen.getByRole("dialog");
    fireEvent.click(dialog); // target == currentTarget → closes
    expect(onClose).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByTestId("inner"));
    // inner click should not have added another call
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
