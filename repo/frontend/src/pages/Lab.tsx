import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { api } from "../api/client";
import type { ReportView, SampleView } from "../api/client";
import { ReportWorkspace } from "../components/ReportWorkspace";
import { Modal } from "../components/Modal";

// LabPage is the primary workspace for lab technicians:
//   - Submit a new sample (test_codes are the ordered panel components)
//   - Advance samples through the lifecycle
//   - Create a v1 report for a sample in testing (abnormal flagging applied)
//   - Search the archive with full-text over titles + narrative notes
//   - Archive a report with a mandatory note; view archived separately
export function LabPage() {
  const [samples, setSamples] = useState<SampleView[]>([]);
  const [reports, setReports] = useState<ReportView[]>([]);
  const [archived, setArchived] = useState<ReportView[]>([]);
  const [showArchived, setShowArchived] = useState(false);
  const [q, setQ] = useState("");
  const [selected, setSelected] = useState<ReportView | null>(null);
  const [sampleForReport, setSampleForReport] = useState<SampleView | null>(null);
  const [reportForm, setReportForm] = useState({ title: "", narrative: "", measurements: "" });
  const [createForm, setCreateForm] = useState({ order_id: "", customer_id: "", test_codes: "", notes: "" });
  const [showCreate, setShowCreate] = useState(false);
  const [banner, setBanner] = useState("");

  const [archiveTarget, setArchiveTarget] = useState<ReportView | null>(null);
  const [archiveNote, setArchiveNote] = useState("");

  async function refresh() {
    api.listSamples().then(setSamples).catch(() => setSamples([]));
    api.listReports().then(setReports).catch(() => setReports([]));
    api.listArchivedReports().then(setArchived).catch(() => setArchived([]));
  }

  useEffect(() => {
    refresh();
  }, []);

  async function createSample(e: React.FormEvent) {
    e.preventDefault();
    setBanner("");
    const codes = createForm.test_codes.split(",").map((s) => s.trim()).filter(Boolean);
    if (codes.length === 0) {
      setBanner("At least one test code is required");
      return;
    }
    try {
      const s = await api.createSample({
        order_id: createForm.order_id || undefined,
        customer_id: createForm.customer_id || undefined,
        test_codes: codes,
        notes: createForm.notes || undefined,
      });
      setSamples((prev) => [s, ...prev]);
      setShowCreate(false);
      setCreateForm({ order_id: "", customer_id: "", test_codes: "", notes: "" });
    } catch (e: unknown) {
      setBanner((e as Error).message);
    }
  }

  async function advanceSample(id: string, to: string) {
    setBanner("");
    try {
      const next = await api.transitionSample(id, to);
      setSamples((prev) => prev.map((s) => (s.ID === id ? next : s)));
    } catch (e: unknown) {
      setBanner((e as Error).message);
    }
  }

  async function search() {
    if (!q.trim()) {
      setReports(await api.listReports());
      return;
    }
    setReports(await api.searchReports(q));
  }

  async function submitReport(e: React.FormEvent) {
    e.preventDefault();
    if (!sampleForReport) return;
    // measurements format: "GLU=85,LIP=50" (test_code=value pairs)
    const ms = reportForm.measurements
      .split(",")
      .map((pair) => pair.trim())
      .filter(Boolean)
      .map((pair) => {
        const [code, v] = pair.split("=").map((s) => s.trim());
        return { test_code: code, value: Number(v) };
      });
    for (const m of ms) {
      if (!m.test_code || isNaN(m.value)) {
        setBanner('Measurements must be "TESTCODE=number" comma-separated');
        return;
      }
    }
    try {
      const r = await api.createReport(sampleForReport.ID, {
        title: reportForm.title,
        narrative: reportForm.narrative,
        measurements: ms,
      });
      setReports((prev) => [r, ...prev]);
      setSampleForReport(null);
      setReportForm({ title: "", narrative: "", measurements: "" });
    } catch (e: unknown) {
      setBanner((e as Error).message);
    }
  }

  async function submitArchive() {
    if (!archiveTarget) return;
    if (!archiveNote.trim()) {
      setBanner("Archive note is required");
      return;
    }
    try {
      await api.archiveReport(archiveTarget.ID, archiveNote.trim());
      setBanner(`Archived report "${archiveTarget.Title}"`);
      setArchiveTarget(null);
      setArchiveNote("");
      await refresh();
    } catch (e: unknown) {
      setBanner((e as Error).message);
    }
  }

  return (
    <div>
      <div className="card" style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
        <h2 style={{ margin: 0 }}>Samples</h2>
        <button className="btn" onClick={() => setShowCreate((v) => !v)}>
          {showCreate ? "Close" : "+ New sample"}
        </button>
      </div>

      {showCreate && (
        <div className="card">
          <form onSubmit={createSample} className="filter-grid">
            <div><label>Order ID</label><input value={createForm.order_id} onChange={(e) => setCreateForm({ ...createForm, order_id: e.target.value })} /></div>
            <div><label>Customer ID</label><input value={createForm.customer_id} onChange={(e) => setCreateForm({ ...createForm, customer_id: e.target.value })} /></div>
            <div><label>Test codes (comma)</label><input value={createForm.test_codes} onChange={(e) => setCreateForm({ ...createForm, test_codes: e.target.value })} placeholder="GLU, LIP" /></div>
            <div><label>Notes</label><input value={createForm.notes} onChange={(e) => setCreateForm({ ...createForm, notes: e.target.value })} /></div>
            <div style={{ gridColumn: "1 / -1" }}>
              <button className="btn" type="submit">Submit sample</button>
            </div>
          </form>
        </div>
      )}

      <div className="card">
        {samples.length === 0 ? (
          <p className="muted">No samples in the queue.</p>
        ) : (
          <table>
            <thead>
              <tr><th>ID</th><th>Tests</th><th>Status</th><th>Updated</th><th></th></tr>
            </thead>
            <tbody>
              {samples.map((s) => (
                <tr key={s.ID}>
                  <td>{s.ID.slice(0, 8)}</td>
                  <td>{s.TestCodes.join(", ")}</td>
                  <td>{s.Status}</td>
                  <td>{new Date(s.UpdatedAt).toLocaleString()}</td>
                  <td style={{ display: "flex", gap: "0.25rem" }}>
                    {nextSampleStatuses(s.Status).map((to) => (
                      <button key={to} className="btn secondary" onClick={() => advanceSample(s.ID, to)}>
                        → {to}
                      </button>
                    ))}
                    {(s.Status === "in_testing" || s.Status === "reported") && (
                      <button className="btn" onClick={() => setSampleForReport(s)}>Create report</button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </div>

      {sampleForReport && (
        <div className="card">
          <h3>New report for sample {sampleForReport.ID.slice(0, 8)}</h3>
          <form onSubmit={submitReport}>
            <label>Title</label>
            <input value={reportForm.title} onChange={(e) => setReportForm({ ...reportForm, title: e.target.value })} required />
            <label>Narrative</label>
            <textarea rows={3} value={reportForm.narrative} onChange={(e) => setReportForm({ ...reportForm, narrative: e.target.value })} />
            <label>Measurements (TESTCODE=value, comma separated)</label>
            <input value={reportForm.measurements} onChange={(e) => setReportForm({ ...reportForm, measurements: e.target.value })} placeholder="GLU=85, LIP=95" />
            <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.5rem" }}>
              <button className="btn" type="submit">Issue report</button>
              <button type="button" className="btn secondary" onClick={() => setSampleForReport(null)}>Cancel</button>
            </div>
          </form>
        </div>
      )}

      {banner && <div className="ok-banner">{banner}</div>}

      <div className="card">
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          <h2 style={{ margin: 0 }}>Report archive</h2>
          <label className="muted">
            <input
              type="checkbox"
              checked={showArchived}
              onChange={(e) => setShowArchived(e.target.checked)}
            />{" "}
            Show archived ({archived.length})
          </label>
        </div>
        <div style={{ display: "flex", gap: "0.5rem", margin: "0.5rem 0" }}>
          <input value={q} onChange={(e) => setQ(e.target.value)} placeholder="Search titles + narrative notes…" />
          <button className="btn" onClick={search}>Search</button>
        </div>
        <table>
          <thead><tr><th>Title</th><th>Version</th><th>Status</th><th>Issued</th><th></th></tr></thead>
          <tbody>
            {(showArchived ? archived : reports).map((r) => (
              <tr key={r.ID} style={r.ArchivedAt ? { opacity: 0.65 } : undefined}>
                <td>{r.Title}{r.ArchivedAt ? " (archived)" : ""}</td>
                <td>{r.Version}</td>
                <td>{r.Status}</td>
                <td>{r.IssuedAt ? new Date(r.IssuedAt).toLocaleString() : "—"}</td>
                <td style={{ display: "flex", gap: "0.25rem" }}>
                  <button className="btn secondary" onClick={() => setSelected(r)}>Inspect</button>
                  <Link className="btn secondary" to={`/reports/${r.ID}`}>Open</Link>
                  {!r.ArchivedAt && (
                    <button className="btn danger" onClick={() => { setArchiveTarget(r); setArchiveNote(""); }}>Archive</button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {archiveTarget && (
        <Modal
          title={`Archive report "${archiveTarget.Title}"`}
          onClose={() => setArchiveTarget(null)}
          actions={
            <>
              <button className="btn secondary" onClick={() => setArchiveTarget(null)}>Cancel</button>
              <button className="btn danger" disabled={!archiveNote.trim()} onClick={submitArchive}>Archive</button>
            </>
          }
        >
          <p className="muted">
            Archived reports remain retrievable via full-text search and by the
            archive view. The archive action is one-way; include a clear note
            for the audit log.
          </p>
          <label>Archive note (required)</label>
          <textarea
            autoFocus
            rows={3}
            value={archiveNote}
            onChange={(e) => setArchiveNote(e.target.value)}
            placeholder="e.g., retention: 7-year retention reached"
          />
        </Modal>
      )}

      {selected && (
        <ReportWorkspace
          report={selected}
          onUpdated={(next) => {
            setReports((prev) => [next, ...prev]);
            setSelected(next);
          }}
        />
      )}
    </div>
  );
}

function nextSampleStatuses(status: string): string[] {
  switch (status) {
    case "sampling": return ["received", "rejected"];
    case "received": return ["in_testing", "rejected"];
    case "in_testing": return ["reported", "rejected"];
    default: return [];
  }
}
