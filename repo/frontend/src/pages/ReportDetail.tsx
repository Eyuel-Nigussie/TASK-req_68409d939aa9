import { useEffect, useState } from "react";
import { useParams, Link } from "react-router-dom";
import { api } from "../api/client";
import type { ReportView } from "../api/client";
import { ReportWorkspace } from "../components/ReportWorkspace";

export function ReportDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const [report, setReport] = useState<ReportView | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    api.getReport(id).then(setReport).catch((e: unknown) => setErr((e as Error).message));
  }, [id]);

  if (err) return <div className="error-banner">{err}</div>;
  if (!report) return <div className="muted">Loading…</div>;

  return (
    <div>
      <ReportWorkspace report={report} onUpdated={setReport} />
      <Link to="/lab" className="btn secondary" style={{ display: "inline-block", marginTop: "0.5rem" }}>
        ← Back to lab
      </Link>
    </div>
  );
}
