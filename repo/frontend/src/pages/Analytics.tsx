import { useEffect, useState } from "react";
import { api } from "../api/client";
import { BarChart, LineChart } from "../components/BarChart";

// AnalyticsPage is the analyst workspace. It pulls an aggregated summary
// of order/sample/report/exception KPIs within an optional date range and
// renders them as charts and tables.
export function AnalyticsPage() {
  const [summary, setSummary] = useState<Awaited<ReturnType<typeof api.analyticsSummary>> | null>(null);
  const [err, setErr] = useState("");
  const [from, setFrom] = useState("");
  const [to, setTo] = useState("");

  async function refresh() {
    setErr("");
    try {
      const f = from ? new Date(from).getTime() / 1000 : undefined;
      const t = to ? new Date(to).getTime() / 1000 + 86399 : undefined;
      setSummary(await api.analyticsSummary(f, t));
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  useEffect(() => {
    refresh();
  }, []);

  if (err) return <div className="error-banner">{err}</div>;
  if (!summary) return <div className="muted">Loading analytics…</div>;

  const orderStatusData = Object.entries(summary.order_status).map(([label, value]) => ({ label, value }));
  const sampleStatusData = Object.entries(summary.sample_status).map(([label, value]) => ({ label, value }));
  const exceptionData = Object.entries(summary.exceptions).map(([label, value]) => ({ label, value }));
  const seriesData = (summary.orders_per_day ?? []).map((d) => ({ label: d.Day, value: d.Count }));

  return (
    <div>
      <div className="card">
        <h2>Operational analytics</h2>
        <p className="muted">
          Aggregations are always computed server-side and bounded by the date range;
          use Export CSV from any listing page for row-level analysis.
        </p>
        <div style={{ display: "flex", gap: "0.5rem", alignItems: "end", flexWrap: "wrap" }}>
          <div><label>From (YYYY-MM-DD)</label><input type="date" value={from} onChange={(e) => setFrom(e.target.value)} /></div>
          <div><label>To (YYYY-MM-DD)</label><input type="date" value={to} onChange={(e) => setTo(e.target.value)} /></div>
          <button className="btn" onClick={refresh}>Recompute</button>
        </div>
      </div>

      <div className="card">
        <h3>Orders by status</h3>
        <BarChart data={orderStatusData} ariaLabel="Orders by status" />
      </div>

      <div className="card">
        <h3>Orders per day</h3>
        <LineChart data={seriesData} ariaLabel="Orders placed per day" />
      </div>

      <div className="card">
        <h3>Samples by status</h3>
        <BarChart data={sampleStatusData} ariaLabel="Samples by status" color="#7c3aed" />
      </div>

      <div className="card">
        <h3>Open exceptions</h3>
        <BarChart data={exceptionData} ariaLabel="Open exception queue by kind" color="#dc2626" />
      </div>

      <div className="card">
        <h3>Abnormal result rate</h3>
        <table>
          <tbody>
            <tr><th>Total measurements</th><td>{summary.abnormal_rate.TotalMeasurements}</td></tr>
            <tr><th>Abnormal</th><td>{summary.abnormal_rate.AbnormalMeasurements}</td></tr>
            <tr><th>Rate</th><td>{(summary.abnormal_rate.Rate * 100).toFixed(2)}%</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  );
}
