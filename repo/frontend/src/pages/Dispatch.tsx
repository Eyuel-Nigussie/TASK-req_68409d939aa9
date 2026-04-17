import { useState } from "react";
import { OfflineMap } from "../components/OfflineMap";
import { api } from "../api/client";

interface Pin {
  lat: number;
  lng: number;
  valid: boolean;
  regionID?: string;
}

interface FeeQuote {
  region_id: string;
  miles: number;
  method: string;
  fee_cents: number;
  fee_usd: number;
}

// DispatchPage drives two linked workflows:
//   1. Place a pin inside the service area and see immediate validation.
//   2. Quote a delivery fee using two named waypoints + their coordinates;
//      the server chooses route-table or haversine and returns method +
//      miles + fee in both cents and USD.
export function DispatchPage() {
  const [lastPin, setLastPin] = useState<Pin | null>(null);
  const [quote, setQuote] = useState<FeeQuote | null>(null);
  const [err, setErr] = useState("");

  const [form, setForm] = useState({
    fromID: "",
    toID: "",
    fromLat: "",
    fromLng: "",
    toLat: "",
    toLng: "",
  });

  async function submitQuote(e: React.FormEvent) {
    e.preventDefault();
    setErr("");
    try {
      const q = await api.quoteFee({
        from_id: form.fromID,
        to_id: form.toID,
        from: { lat: Number(form.fromLat) || 0, lng: Number(form.fromLng) || 0 },
        to: { lat: Number(form.toLat) || 0, lng: Number(form.toLng) || 0 },
      });
      setQuote(q);
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  function useLastPinAsDestination() {
    if (!lastPin) return;
    setForm((f) => ({ ...f, toLat: String(lastPin.lat), toLng: String(lastPin.lng) }));
  }

  return (
    <div>
      <div className="card">
        <h2>Service-area pin</h2>
        <p className="muted">
          Click inside the map to place a pin. The system validates the location
          against locally stored service-area polygons and shows immediate feedback.
        </p>
        <OfflineMap onPinned={setLastPin} />
      </div>

      {lastPin && (
        <div className="card">
          <h3>Last pin</h3>
          <table>
            <tbody>
              <tr><th>Latitude</th><td>{lastPin.lat.toFixed(5)}</td></tr>
              <tr><th>Longitude</th><td>{lastPin.lng.toFixed(5)}</td></tr>
              <tr><th>In service area</th><td>{lastPin.valid ? "yes" : "no"}</td></tr>
              {lastPin.regionID && <tr><th>Region</th><td>{lastPin.regionID}</td></tr>}
            </tbody>
          </table>
          <button className="btn secondary" onClick={useLastPinAsDestination}>Use as delivery destination</button>
        </div>
      )}

      <div className="card">
        <h2>Fee quote</h2>
        <p className="muted">
          Compute the delivery fee for an origin/destination pair. The server
          uses the preloaded route table when an entry exists, otherwise it
          falls back to straight-line (haversine) distance.
        </p>
        <form onSubmit={submitQuote} className="filter-grid">
          <div><label>From waypoint ID</label><input value={form.fromID} onChange={(e) => setForm({ ...form, fromID: e.target.value })} placeholder="e.g., depot-A" /></div>
          <div><label>To waypoint ID</label><input value={form.toID} onChange={(e) => setForm({ ...form, toID: e.target.value })} placeholder="e.g., drop-42" /></div>
          <div><label>From lat</label><input value={form.fromLat} onChange={(e) => setForm({ ...form, fromLat: e.target.value })} /></div>
          <div><label>From lng</label><input value={form.fromLng} onChange={(e) => setForm({ ...form, fromLng: e.target.value })} /></div>
          <div><label>To lat</label><input value={form.toLat} onChange={(e) => setForm({ ...form, toLat: e.target.value })} /></div>
          <div><label>To lng</label><input value={form.toLng} onChange={(e) => setForm({ ...form, toLng: e.target.value })} /></div>
          <div style={{ gridColumn: "1 / -1" }}>
            <button className="btn" type="submit">Quote fee</button>
          </div>
        </form>
        {err && <div className="error-banner" role="alert">{err}</div>}
        {quote && (
          <table style={{ marginTop: "0.75rem" }}>
            <tbody>
              <tr><th>Region</th><td>{quote.region_id}</td></tr>
              <tr><th>Miles</th><td>{quote.miles.toFixed(2)}</td></tr>
              <tr><th>Method</th><td>{quote.method}</td></tr>
              <tr><th>Fee</th><td>${quote.fee_usd.toFixed(2)} ({quote.fee_cents}¢)</td></tr>
            </tbody>
          </table>
        )}
      </div>
    </div>
  );
}
