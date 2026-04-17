import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { CustomerView } from "../api/client";

// CustomersPage lets front-desk staff register a new customer and then
// find/select existing ones. Registration mirrors the POST /api/customers
// contract; the backend encrypts identifier and street at rest.
export function CustomersPage() {
  const [results, setResults] = useState<CustomerView[]>([]);
  const [query, setQuery] = useState("");
  const [form, setForm] = useState({
    name: "", identifier: "", street: "", city: "", state: "", zip: "",
    phone: "", email: "", tags: "",
  });
  const [banner, setBanner] = useState("");
  const [err, setErr] = useState("");

  async function search() {
    try {
      setResults(await api.searchCustomers(query));
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  useEffect(() => {
    api.searchCustomers("").then(setResults).catch(() => setResults([]));
  }, []);

  async function register(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setBanner("");
    if (!form.name.trim()) {
      setErr("Name is required.");
      return;
    }
    try {
      const cu = await api.createCustomer({
        name: form.name,
        identifier: form.identifier || undefined,
        street: form.street || undefined,
        city: form.city || undefined,
        state: form.state || undefined,
        zip: form.zip || undefined,
        phone: form.phone || undefined,
        email: form.email || undefined,
        tags: form.tags ? form.tags.split(",").map((t) => t.trim()).filter(Boolean) : undefined,
      });
      setBanner(`Registered ${cu.name} (${cu.id.slice(0, 8)})`);
      setForm({ name: "", identifier: "", street: "", city: "", state: "", zip: "", phone: "", email: "", tags: "" });
      setResults((prev) => [cu, ...prev]);
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  return (
    <div>
      <div className="card">
        <h2>Register customer</h2>
        <form onSubmit={register}>
          <div className="filter-grid">
            <div><label>Name *</label><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} /></div>
            <div><label>Identifier</label><input value={form.identifier} onChange={(e) => setForm({ ...form, identifier: e.target.value })} /></div>
            <div><label>Phone</label><input value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} /></div>
            <div><label>Email</label><input value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} /></div>
            <div><label>Street</label><input value={form.street} onChange={(e) => setForm({ ...form, street: e.target.value })} /></div>
            <div><label>City</label><input value={form.city} onChange={(e) => setForm({ ...form, city: e.target.value })} /></div>
            <div><label>State</label><input value={form.state} onChange={(e) => setForm({ ...form, state: e.target.value })} /></div>
            <div><label>ZIP</label><input value={form.zip} onChange={(e) => setForm({ ...form, zip: e.target.value })} /></div>
            <div><label>Tags (comma)</label><input value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} /></div>
          </div>
          {err && <div className="error-banner" role="alert">{err}</div>}
          {banner && <div className="ok-banner">{banner}</div>}
          <button className="btn" type="submit" style={{ marginTop: "0.5rem" }}>Register</button>
        </form>
      </div>

      <div className="card">
        <h2>Find customer</h2>
        <div style={{ display: "flex", gap: "0.5rem", marginBottom: "0.5rem" }}>
          <input value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Name, phone, city, ZIP…" />
          <button className="btn" onClick={search}>Search</button>
        </div>
        <table>
          <thead><tr><th>Name</th><th>City</th><th>ZIP</th><th>Phone</th><th></th></tr></thead>
          <tbody>
            {results.map((c) => (
              <tr key={c.id}>
                <td>{c.name}</td>
                <td>{c.city}</td>
                <td>{c.zip}</td>
                <td>{c.phone}</td>
                <td><Link className="btn secondary" to={`/customers/${c.id}`}>Open</Link></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
