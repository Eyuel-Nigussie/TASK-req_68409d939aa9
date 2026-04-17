import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api, ApiError } from "../api/client";
import type { OrderView, FilterPayload } from "../api/client";
import { AdvancedFilters } from "../components/AdvancedFilters";
import { OrderTimeline } from "../components/OrderTimeline";
import { Modal } from "../components/Modal";

const ORDER_STATUSES = ["placed", "picking", "dispatched", "delivered", "received", "refunded", "canceled"];

export function OrdersPage() {
  const [orders, setOrders] = useState<OrderView[]>([]);
  const [banner, setBanner] = useState("");
  const [total, setTotal] = useState(0);
  const [showCreate, setShowCreate] = useState(false);
  const [form, setForm] = useState({
    customer_id: "",
    total_usd: "0",
    priority: "standard",
    tags: "",
    delivery_street: "",
    delivery_city: "",
    delivery_state: "",
    delivery_zip: "",
  });
  const [items, setItems] = useState<Array<{ SKU: string; Description: string; Qty: number; Backordered: boolean }>>([
    { SKU: "", Description: "", Qty: 1, Backordered: false },
  ]);
  const [refundTarget, setRefundTarget] = useState<{ id: string; to: string } | null>(null);
  const [refundReason, setRefundReason] = useState("");
  const [transitionErr, setTransitionErr] = useState("");

  async function apply(f: FilterPayload) {
    const res = await api.queryOrders(f);
    setOrders(res.items ?? []);
    setTotal(res.total);
  }

  async function saveFilter(name: string, f: FilterPayload) {
    try {
      await api.saveFilter(name, f);
      setBanner(`Saved filter "${name}"`);
    } catch (e: unknown) {
      setBanner((e as Error).message);
    }
  }

  async function loadDefault() {
    const list = await api.listOrders({ limit: 25 });
    setOrders(list);
    setTotal(list.length);
  }

  useEffect(() => {
    loadDefault().catch(() => setOrders([]));
  }, []);

  function requestTransition(id: string, to: string) {
    if (to === "refunded") {
      setRefundTarget({ id, to });
      setRefundReason("");
      setTransitionErr("");
      return;
    }
    doTransition(id, to, "");
  }

  async function doTransition(id: string, to: string, reason: string) {
    try {
      const next = await api.transitionOrder(id, to, reason);
      setOrders((prev) => prev.map((o) => (o.ID === id ? next : o)));
      setRefundTarget(null);
    } catch (e: unknown) {
      const msg = e instanceof ApiError ? e.message : (e as Error).message;
      setTransitionErr(msg);
    }
  }

  async function createOrder(e: React.FormEvent) {
    e.preventDefault();
    const cents = Math.round(Number(form.total_usd) * 100);
    if (isNaN(cents) || cents < 0) {
      setBanner("Total must be a non-negative number");
      return;
    }
    // Validate each line item: SKU required, Qty > 0.
    const cleanItems = items
      .filter((it) => it.SKU.trim() !== "" || it.Qty > 0)
      .map((it) => ({ ...it, SKU: it.SKU.trim() }));
    for (const it of cleanItems) {
      if (!it.SKU) {
        setBanner("Every line item needs a SKU");
        return;
      }
      if (!Number.isFinite(it.Qty) || it.Qty <= 0) {
        setBanner(`Line item ${it.SKU}: Qty must be > 0`);
        return;
      }
    }
    try {
      const o = await api.createOrder({
        customer_id: form.customer_id || undefined,
        total_cents: cents,
        priority: form.priority,
        tags: form.tags ? form.tags.split(",").map((t) => t.trim()).filter(Boolean) : undefined,
        items: cleanItems.length > 0 ? cleanItems : undefined,
        delivery_street: form.delivery_street || undefined,
        delivery_city: form.delivery_city || undefined,
        delivery_state: form.delivery_state || undefined,
        delivery_zip: form.delivery_zip || undefined,
      });
      setOrders((prev) => [o, ...prev]);
      setShowCreate(false);
      setForm({
        customer_id: "",
        total_usd: "0",
        priority: "standard",
        tags: "",
        delivery_street: "",
        delivery_city: "",
        delivery_state: "",
        delivery_zip: "",
      });
      setItems([{ SKU: "", Description: "", Qty: 1, Backordered: false }]);
      setBanner(`Created order ${o.ID.slice(0, 8)}`);
    } catch (e: unknown) {
      setBanner((e as Error).message);
    }
  }

  function updateItem(i: number, field: "SKU" | "Description" | "Qty" | "Backordered", val: string | number | boolean) {
    setItems(items.map((it, idx) => (idx === i ? { ...it, [field]: val } : it)));
  }

  return (
    <div>
      <div className="card" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h2 style={{ margin: 0 }}>Orders</h2>
        <button className="btn" onClick={() => setShowCreate((v) => !v)}>
          {showCreate ? "Close" : "+ New order"}
        </button>
      </div>

      {showCreate && (
        <div className="card">
          <form onSubmit={createOrder}>
            <h3 style={{ marginTop: 0 }}>Order details</h3>
            <div className="filter-grid">
              <div><label>Customer ID</label><input value={form.customer_id} onChange={(e) => setForm({ ...form, customer_id: e.target.value })} /></div>
              <div><label>Total (USD)</label><input value={form.total_usd} onChange={(e) => setForm({ ...form, total_usd: e.target.value })} /></div>
              <div>
                <label>Priority</label>
                <select value={form.priority} onChange={(e) => setForm({ ...form, priority: e.target.value })}>
                  <option value="standard">Standard</option>
                  <option value="rush">Rush</option>
                  <option value="critical">Critical</option>
                </select>
              </div>
              <div><label>Tags</label><input value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} placeholder="inbound, bulk" /></div>
            </div>

            <h3>Delivery address</h3>
            <div className="filter-grid">
              <div><label>Street</label><input value={form.delivery_street} onChange={(e) => setForm({ ...form, delivery_street: e.target.value })} /></div>
              <div><label>City</label><input value={form.delivery_city} onChange={(e) => setForm({ ...form, delivery_city: e.target.value })} /></div>
              <div><label>State</label><input value={form.delivery_state} onChange={(e) => setForm({ ...form, delivery_state: e.target.value })} /></div>
              <div><label>ZIP</label><input value={form.delivery_zip} onChange={(e) => setForm({ ...form, delivery_zip: e.target.value })} /></div>
            </div>

            <h3>Line items</h3>
            <table>
              <thead><tr><th>SKU</th><th>Description</th><th>Qty</th><th>Backordered</th><th></th></tr></thead>
              <tbody>
                {items.map((it, i) => (
                  <tr key={i}>
                    <td><input value={it.SKU} onChange={(e) => updateItem(i, "SKU", e.target.value)} placeholder="SKU-001" /></td>
                    <td><input value={it.Description} onChange={(e) => updateItem(i, "Description", e.target.value)} /></td>
                    <td><input type="number" min={1} value={it.Qty} onChange={(e) => updateItem(i, "Qty", Number(e.target.value))} /></td>
                    <td><input type="checkbox" checked={it.Backordered} onChange={(e) => updateItem(i, "Backordered", e.target.checked)} /></td>
                    <td><button type="button" className="btn danger" onClick={() => setItems(items.filter((_, j) => j !== i))}>×</button></td>
                  </tr>
                ))}
              </tbody>
            </table>
            <button type="button" className="btn secondary" onClick={() => setItems([...items, { SKU: "", Description: "", Qty: 1, Backordered: false }])} style={{ marginTop: "0.5rem" }}>
              + Add line item
            </button>

            <div style={{ marginTop: "1rem" }}>
              <button className="btn" type="submit">Create order</button>
            </div>
          </form>
        </div>
      )}

      <AdvancedFilters entity="order" statuses={ORDER_STATUSES} onApply={apply} onSave={saveFilter} />
      {banner && <div className="ok-banner">{banner}</div>}
      {transitionErr && <div className="error-banner">{transitionErr}</div>}
      {total > 0 && <p className="muted">{total} matching orders</p>}

      {orders.map((o) => (
        <div className="card" key={o.ID}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
            <div>
              <Link to={`/orders/${o.ID}`}><strong>#{o.ID.slice(0, 8)}</strong></Link> ·{" "}
              <span className="muted">{new Date(o.PlacedAt).toLocaleString()}</span> ·{" "}
              ${(o.TotalCents / 100).toFixed(2)}{o.Priority ? ` · ${o.Priority}` : ""}
            </div>
            <div style={{ display: "flex", gap: "0.25rem" }}>
              {nextStatuses(o.Status).map((s) => (
                <button key={s} className="btn secondary" onClick={() => requestTransition(o.ID, s)}>
                  → {s}
                </button>
              ))}
            </div>
          </div>
          <OrderTimeline order={o} />
        </div>
      ))}

      {refundTarget && (
        <Modal
          title="Refund reason required"
          onClose={() => setRefundTarget(null)}
          actions={
            <>
              <button className="btn secondary" onClick={() => setRefundTarget(null)}>Cancel</button>
              <button
                className="btn danger"
                disabled={!refundReason.trim()}
                onClick={() => doTransition(refundTarget.id, refundTarget.to, refundReason.trim())}
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
            placeholder="e.g., customer returned item, damaged in transit"
          />
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
