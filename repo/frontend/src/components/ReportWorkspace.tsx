import { useState } from "react";
import { api } from "../api/client";
import type { ReportView } from "../api/client";

interface Props {
  report: ReportView;
  onUpdated?: (next: ReportView) => void;
}

// ReportWorkspace shows the issued report in a table with abnormal values
// highlighted red, and provides a "correct" workflow with a mandatory
// reason note and optimistic-concurrency handling.
export function ReportWorkspace({ report, onUpdated }: Props) {
  const [editing, setEditing] = useState(false);
  const [title, setTitle] = useState(report.Title);
  const [narrative, setNarrative] = useState(report.Narrative);
  const [measurements] = useState(
    report.Measurements.map((m) => ({ test_code: m.TestCode, value: m.Value, units: m.Units, unmeasurable: m.Unmeasurable })),
  );
  const [reason, setReason] = useState("");
  const [err, setErr] = useState("");

  const superseded = report.Status === "superseded";

  async function submitCorrection() {
    if (!reason.trim()) {
      setErr("Reason note is required for corrections");
      return;
    }
    try {
      const next = await api.correctReport(report.ID, {
        expected_version: report.Version,
        title,
        narrative,
        measurements,
        reason,
      });
      setEditing(false);
      setReason("");
      onUpdated?.(next);
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  return (
    <div className="card" aria-label="Report workspace">
      <h2>
        {report.Title}{" "}
        <span className="muted">v{report.Version} · {report.Status}</span>
      </h2>
      {superseded && (
        <div className="error-banner">This report has been superseded; it is read-only but retained for the record.</div>
      )}
      <p>{report.Narrative}</p>
      <table>
        <thead>
          <tr>
            <th>Test</th>
            <th>Value</th>
            <th>Units</th>
            <th>Flag</th>
          </tr>
        </thead>
        <tbody>
          {report.Measurements.map((m, i) => {
            const neutral = m.Flag === "normal" || m.Flag === "unmeasurable" || m.Flag === "uncategorized";
            const abnormal = !neutral;
            const uncategorized = m.Flag === "uncategorized";
            const rowClass = abnormal ? "abnormal cell" : uncategorized ? "uncategorized cell" : "";
            return (
              <tr key={i} className={rowClass}>
                <td>{m.TestCode}</td>
                <td className={abnormal ? "abnormal" : ""}>
                  {m.Unmeasurable ? "—" : m.Value}
                </td>
                <td>{m.Units || ""}</td>
                <td className={abnormal ? "abnormal" : uncategorized ? "muted" : ""}>{m.Flag}</td>
              </tr>
            );
          })}
        </tbody>
      </table>

      {!superseded && !editing && (
        <button className="btn" onClick={() => setEditing(true)} style={{ marginTop: "0.5rem" }}>
          Correct report
        </button>
      )}

      {editing && (
        <div style={{ marginTop: "1rem" }}>
          <label>Title</label>
          <input value={title} onChange={(e) => setTitle(e.target.value)} />
          <label>Narrative</label>
          <textarea rows={3} value={narrative} onChange={(e) => setNarrative(e.target.value)} />
          <label>Reason (required)</label>
          <input value={reason} onChange={(e) => setReason(e.target.value)} placeholder="e.g., clinical correction, typo" />
          <p className="muted">
            Measurements are unchanged in this simplified demo; a real correction flow offers per-row editing.
          </p>
          {err && <div className="error-banner">{err}</div>}
          <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.5rem" }}>
            <button className="btn" onClick={submitCorrection}>Issue correction</button>
            <button className="btn secondary" onClick={() => { setEditing(false); setErr(""); }}>Cancel</button>
          </div>
        </div>
      )}
      {/* Mark measurements variable as used to avoid TS noUnusedLocals when readonly view. */}
      <input type="hidden" value={JSON.stringify(measurements)} readOnly />
    </div>
  );
}
