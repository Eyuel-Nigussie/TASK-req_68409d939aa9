import { render, screen, fireEvent } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { AdvancedFilters } from "./AdvancedFilters";

describe("AdvancedFilters", () => {
  it("rejects non-MM/DD/YYYY dates", () => {
    const onApply = vi.fn();
    render(<AdvancedFilters entity="order" statuses={["placed"]} onApply={onApply} />);
    const inputs = screen.getAllByRole("textbox");
    // Find the Start field (first textbox is Keyword).
    const start = inputs.find((i) => (i as HTMLInputElement).placeholder === "01/01/2024");
    fireEvent.change(start!, { target: { value: "2024-01-01" } });
    fireEvent.click(screen.getByText("Apply"));
    expect(screen.getByRole("alert")).toHaveTextContent(/MM\/DD\/YYYY/i);
    expect(onApply).not.toHaveBeenCalled();
  });

  it("accepts well-formed input and emits the filter payload", () => {
    const onApply = vi.fn();
    render(<AdvancedFilters entity="order" statuses={["placed", "picking"]} onApply={onApply} />);
    const inputs = screen.getAllByRole("textbox");
    const start = inputs.find((i) => (i as HTMLInputElement).placeholder === "01/01/2024")!;
    fireEvent.change(start, { target: { value: "01/01/2024" } });
    fireEvent.click(screen.getByText("Apply"));
    expect(onApply).toHaveBeenCalledTimes(1);
    const payload = onApply.mock.calls[0][0];
    expect(payload.entity).toBe("order");
    expect(payload.start_date).toBe("01/01/2024");
  });
});
