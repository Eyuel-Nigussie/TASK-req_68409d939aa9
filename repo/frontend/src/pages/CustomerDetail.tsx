import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api } from "../api/client";
import type { CustomerView } from "../api/client";

export function CustomerDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [customer, setCustomer] = useState<CustomerView | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    let cancelled = false;
    api
      .getCustomer(id)
      .then((c) => {
        if (!cancelled) setCustomer(c);
      })
      .catch((e: unknown) => {
        if (!cancelled) setErr((e as Error).message);
      });
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (err) return <div className="error-banner">{err}</div>;
  if (!customer) return <div className="muted">Loading…</div>;

  return (
    <div className="card">
      <h2>{customer.name}</h2>
      <p className="muted">
        Customer {customer.id.slice(0, 8)} · created {new Date(customer.created_at).toLocaleString()}
      </p>
      <table>
        <tbody>
          <tr><th>Identifier</th><td>{customer.identifier || "—"}</td></tr>
          <tr><th>Address</th><td>{customer.street || "—"}, {customer.city || "—"}, {customer.state || "—"} {customer.zip || ""}</td></tr>
          <tr><th>Phone</th><td>{customer.phone || "—"}</td></tr>
          <tr><th>Email</th><td>{customer.email || "—"}</td></tr>
          <tr><th>Tags</th><td>{(customer.tags ?? []).join(", ") || "—"}</td></tr>
        </tbody>
      </table>
      <Link to="/customers" className="btn secondary" style={{ marginTop: "0.5rem", display: "inline-block" }}>
        ← Back to customers
      </Link>
    </div>
  );
}
