import { useEffect } from "react";

// Minimal modal dialog. Kept dependency-free because the portal ships on a
// closed network and we avoid pulling a UI kit we can't update offline.
// The modal blocks keyboard/focus cues in jsdom-level tests; for full
// production use a focus-trap would be added in a follow-up.

interface Props {
  title: string;
  onClose: () => void;
  children: React.ReactNode;
  actions?: React.ReactNode;
}

export function Modal({ title, onClose, children, actions }: Props) {
  useEffect(() => {
    function esc(e: KeyboardEvent) {
      if (e.key === "Escape") onClose();
    }
    document.addEventListener("keydown", esc);
    return () => document.removeEventListener("keydown", esc);
  }, [onClose]);

  return (
    <div
      role="dialog"
      aria-modal="true"
      aria-label={title}
      onClick={(e) => {
        if (e.target === e.currentTarget) onClose();
      }}
      style={{
        position: "fixed",
        inset: 0,
        background: "rgba(17, 24, 39, 0.5)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        zIndex: 100,
      }}
    >
      <div
        style={{
          background: "var(--card)",
          borderRadius: 8,
          minWidth: 360,
          maxWidth: 560,
          padding: "1rem 1.25rem",
          boxShadow: "0 12px 32px rgba(0,0,0,0.3)",
        }}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: "0.75rem" }}>
          <h3 style={{ margin: 0 }}>{title}</h3>
          <button aria-label="Close" className="btn secondary" onClick={onClose}>×</button>
        </div>
        <div>{children}</div>
        {actions && <div style={{ display: "flex", gap: "0.5rem", justifyContent: "flex-end", marginTop: "1rem" }}>{actions}</div>}
      </div>
    </div>
  );
}
