import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { OrderView } from "../api/client";
import { OrderTimeline } from "../components/OrderTimeline";

// Dashboard shows exception queues and recently placed orders so operators
// see what needs their attention when they sit down.
export function Dashboard() {
  const [exceptions, setExceptions] = useState<Array<{ OrderID: string; Kind: string; DetectedAt: string; Description: string }>>([]);
  const [recent, setRecent] = useState<OrderView[]>([]);

  async function refresh() {
    try {
      const [exc, ord] = await Promise.all([api.listExceptions(), api.listOrders({ limit: 10 })]);
      setExceptions(exc);
      setRecent(ord);
    } catch {
      // noop - we'll retry on next mount.
    }
  }

  useEffect(() => {
    refresh();
    const t = setInterval(refresh, 30_000);
    return () => clearInterval(t);
  }, []);

  return (
    <div>
      <div className="card">
        <h2>Exception queue</h2>
        {exceptions.length === 0 ? (
          <p className="muted">No active exceptions.</p>
        ) : (
          <table>
            <thead>
              <tr><th>Order</th><th>Kind</th><th>Detected</th><th>Description</th></tr>
            </thead>
            <tbody>
              {exceptions.map((x) => (
                <tr key={x.OrderID + x.Kind}>
                  <td>{x.OrderID}</td>
                  <td>{x.Kind}</td>
                  <td>{new Date(x.DetectedAt).toLocaleString()}</td>
                  <td>{x.Description}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      <div className="card">
        <h2>Recent orders</h2>
        {recent.length === 0 ? (
          <p className="muted">No orders yet.</p>
        ) : (
          recent.map((o) => (
            <div key={o.ID} style={{ marginBottom: "1rem" }}>
              <div>
                <strong>#{o.ID.slice(0, 8)}</strong> · ${(o.TotalCents / 100).toFixed(2)} · {new Date(o.PlacedAt).toLocaleString()}
              </div>
              <OrderTimeline order={o} />
            </div>
          ))
        )}
      </div>
    </div>
  );
}
