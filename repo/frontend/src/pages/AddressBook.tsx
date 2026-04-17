import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { AddressBookEntry, CustomerView, OrderView } from "../api/client";

// AddressBookPage supports two discovery flows keyed off the same address
// inputs:
//   1. Find customers by delivery/mailing address
//   2. Find jobs/orders by delivery address (the prompt's "jobs by address")
//
// This keeps the mental model for operators simple: type the location,
// pick the entity type.
export function AddressBookPage() {
  const [entries, setEntries] = useState<AddressBookEntry[]>([]);
  const [street, setStreet] = useState("");
  const [city, setCity] = useState("");
  const [zip, setZip] = useState("");
  const [customerResults, setCustomerResults] = useState<CustomerView[]>([]);
  const [orderResults, setOrderResults] = useState<OrderView[]>([]);
  const [label, setLabel] = useState("");
  const [err, setErr] = useState("");

  async function refresh() {
    try {
      setEntries(await api.listAddressBook());
    } catch {
      setEntries([]);
    }
  }
  useEffect(() => {
    refresh();
  }, []);

  async function lookupCustomers() {
    setErr("");
    if (!city && !zip) {
      setErr("Enter at least a city or ZIP");
      return;
    }
    try {
      setCustomerResults(await api.customersByAddress({ street, city, zip }));
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function lookupOrders() {
    setErr("");
    if (!city && !zip) {
      setErr("Enter at least a city or ZIP");
      return;
    }
    try {
      setOrderResults(await api.ordersByAddress({ street, city, zip }));
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function save() {
    if (!label) return;
    await api.saveAddress({ label, street, city, zip });
    setLabel("");
    refresh();
  }

  async function remove(id: string) {
    await api.deleteAddress(id);
    refresh();
  }

  return (
    <div>
      <div className="card">
        <h2>Lookup by address</h2>
        <div className="filter-grid">
          <div><label>Street</label><input value={street} onChange={(e) => setStreet(e.target.value)} /></div>
          <div><label>City</label><input value={city} onChange={(e) => setCity(e.target.value)} /></div>
          <div><label>ZIP</label><input value={zip} onChange={(e) => setZip(e.target.value)} /></div>
        </div>
        <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.5rem", flexWrap: "wrap" }}>
          <button className="btn" onClick={lookupCustomers}>Find customers</button>
          <button className="btn" onClick={lookupOrders}>Find jobs</button>
          <input placeholder="Save this address as…" value={label} onChange={(e) => setLabel(e.target.value)} />
          <button className="btn secondary" onClick={save} disabled={!label.trim()}>Save</button>
        </div>
        {err && <div className="error-banner" role="alert" style={{ marginTop: "0.5rem" }}>{err}</div>}
      </div>

      {customerResults.length > 0 && (
        <div className="card">
          <h3>Matching customers ({customerResults.length})</h3>
          <ul>
            {customerResults.map((c) => (
              <li key={c.id}>
                <Link to={`/customers/${c.id}`}><strong>{c.name}</strong></Link> — {c.street}, {c.city}, {c.state} {c.zip}
              </li>
            ))}
          </ul>
        </div>
      )}

      {orderResults.length > 0 && (
        <div className="card">
          <h3>Matching jobs ({orderResults.length})</h3>
          <table>
            <thead><tr><th>Order</th><th>Status</th><th>Placed</th><th>Address</th></tr></thead>
            <tbody>
              {orderResults.map((o) => (
                <tr key={o.ID}>
                  <td><Link to={`/orders/${o.ID}`}>#{o.ID.slice(0, 8)}</Link></td>
                  <td>{o.Status}</td>
                  <td>{new Date(o.PlacedAt).toLocaleString()}</td>
                  <td>
                    {/* TS may not know about delivery fields; read dynamically */}
                    {(o as unknown as { DeliveryStreet?: string }).DeliveryStreet || "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      <div className="card">
        <h2>Saved addresses</h2>
        {entries.length === 0 ? (
          <p className="muted">No saved addresses.</p>
        ) : (
          <table>
            <thead>
              <tr><th>Label</th><th>Street</th><th>City</th><th>ZIP</th><th></th></tr>
            </thead>
            <tbody>
              {entries.map((e) => (
                <tr key={e.id}>
                  <td>{e.label}</td>
                  <td>{e.street}</td>
                  <td>{e.city}</td>
                  <td>{e.zip}</td>
                  <td><button className="btn danger" onClick={() => remove(e.id)}>Delete</button></td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
