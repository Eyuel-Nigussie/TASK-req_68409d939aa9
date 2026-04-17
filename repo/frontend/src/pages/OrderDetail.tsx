import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api } from "../api/client";
import type { OrderView } from "../api/client";
import { OrderTimeline } from "../components/OrderTimeline";
import { Modal } from "../components/Modal";

export function OrderDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [order, setOrder] = useState<OrderView | null>(null);
  const [err, setErr] = useState("");

  const [refundOpen, setRefundOpen] = useState(false);
  const [refundTo, setRefundTo] = useState("");
  const [refundReason, setRefundReason] = useState("");

  const [invOpen, setInvOpen] = useState(false);
  const [invSku, setInvSku] = useState("");
  const [invBackordered, setInvBackordered] = useState(true);
  const [invNote, setInvNote] = useState("");

  async function refresh() {
    try {
      setOrder(await api.getOrder(id));
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  useEffect(() => {
    refresh();
  }, [id]);

  function requestTransition(to: string) {
    if (to === "refunded") {
      setRefundTo(to);
      setRefundReason("");
      setRefundOpen(true);
      return;
    }
    submitTransition(to, "");
  }

  async function submitTransition(to: string, reason: string) {
    try {
      const next = await api.transitionOrder(id, to, reason);
      setOrder(next);
      setRefundOpen(false);
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function submitInventory(e: React.FormEvent) {
    e.preventDefault();
    if (!invSku.trim()) {
      setErr("SKU is required");
      return;
    }
    try {
      const next = await api.updateInventory(id, invSku.trim(), invBackordered, invNote);
      setOrder(next);
      setInvOpen(false);
      setInvSku("");
      setInvNote("");
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  if (err) return <div className="error-banner">{err}</div>;
  if (!order) return <div className="muted">Loading…</div>;

  return (
    <div className="card">
      <h2>Order #{order.ID.slice(0, 8)}</h2>
      <p className="muted">
        Placed {new Date(order.PlacedAt).toLocaleString()} · ${(order.TotalCents / 100).toFixed(2)} · priority {order.Priority || "standard"}
      </p>
      <OrderTimeline order={order} />
      <div style={{ display: "flex", gap: "0.25rem", flexWrap: "wrap", marginTop: "0.5rem" }}>
        {nextStatuses(order.Status).map((s) => (
          <button key={s} className="btn secondary" onClick={() => requestTransition(s)}>→ {s}</button>
        ))}
        <button className="btn secondary" onClick={() => setInvOpen(true)}>Flag inventory</button>
      </div>

      <h3 style={{ marginTop: "1rem" }}>History</h3>
      <table>
        <thead>
          <tr><th>At</th><th>From → To</th><th>Actor</th><th>Reason</th></tr>
        </thead>
        <tbody>
          {(order.Events ?? []).map((e) => (
            <tr key={e.ID}>
              <td>{new Date(e.At).toLocaleString()}</td>
              <td>{e.From} → {e.To}</td>
              <td>{e.Actor}</td>
              <td>{e.Reason || ""}</td>
            </tr>
          ))}
        </tbody>
      </table>
      <Link to="/orders" className="btn secondary" style={{ display: "inline-block", marginTop: "0.5rem" }}>
        ← All orders
      </Link>

      {refundOpen && (
        <Modal
          title="Refund reason required"
          onClose={() => setRefundOpen(false)}
          actions={
            <>
              <button className="btn secondary" onClick={() => setRefundOpen(false)}>Cancel</button>
              <button
                className="btn danger"
                disabled={!refundReason.trim()}
                onClick={() => submitTransition(refundTo, refundReason.trim())}
              >
                Confirm refund
              </button>
            </>
          }
        >
          <label>Reason (required)</label>
          <textarea
            autoFocus
            rows={3}
            value={refundReason}
            onChange={(e) => setRefundReason(e.target.value)}
            placeholder="e.g., damaged in transit, customer returned"
          />
        </Modal>
      )}

      {invOpen && (
        <Modal
          title="Flag inventory"
          onClose={() => setInvOpen(false)}
          actions={
            <>
              <button className="btn secondary" onClick={() => setInvOpen(false)}>Cancel</button>
              <button className="btn" onClick={submitInventory}>Save</button>
            </>
          }
        >
          <form onSubmit={submitInventory}>
            <label htmlFor="inv-sku">SKU</label>
            <input id="inv-sku" autoFocus value={invSku} onChange={(e) => setInvSku(e.target.value)} />
            <label style={{ marginTop: "0.5rem" }}>
              <input type="checkbox" checked={invBackordered} onChange={(e) => setInvBackordered(e.target.checked)} /> Backordered
            </label>
            <label htmlFor="inv-note">Note</label>
            <input id="inv-note" value={invNote} onChange={(e) => setInvNote(e.target.value)} />
            <p className="muted" style={{ marginTop: "0.5rem" }}>
              If this flips an item to backordered, an out-of-stock exception is
              automatically added to the exception queue.
            </p>
          </form>
        </Modal>
      )}
    </div>
  );
}

function nextStatuses(current: string): string[] {
  switch (current) {
    case "placed": return ["picking", "canceled"];
    case "picking": return ["dispatched", "canceled", "refunded"];
    case "dispatched": return ["delivered", "canceled"];
    case "delivered": return ["received", "refunded"];
    case "received": return ["refunded"];
    default: return [];
  }
}
