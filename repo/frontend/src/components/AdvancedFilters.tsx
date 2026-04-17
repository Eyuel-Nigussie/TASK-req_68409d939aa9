import { useState } from "react";
import type { FilterPayload } from "../api/client";

interface Props {
  entity: FilterPayload["entity"];
  statuses: string[];
  onApply: (f: FilterPayload) => void;
  onSave?: (name: string, f: FilterPayload) => void;
}

// The MM/DD/YYYY constraint is enforced client-side with a stricter regex
// than the HTML date input to match the backend validator, which refuses
// ISO inputs (2024-12-31) and EU inputs (31/12/2024).
const DATE_RE = /^(0[1-9]|1[0-2])\/(0[1-9]|[12][0-9]|3[01])\/\d{4}$/;

export function AdvancedFilters({ entity, statuses, onApply, onSave }: Props) {
  const [keyword, setKeyword] = useState("");
  const [start, setStart] = useState("");
  const [end, setEnd] = useState("");
  const [selStatuses, setSelStatuses] = useState<string[]>([]);
  const [tags, setTags] = useState("");
  const [priority, setPriority] = useState("");
  const [minP, setMinP] = useState("");
  const [maxP, setMaxP] = useState("");
  const [sortBy, setSortBy] = useState("");
  const [sortDesc, setSortDesc] = useState(false);
  const [page, setPage] = useState(1);
  const [size, setSize] = useState(25);
  const [name, setName] = useState("");
  const [error, setError] = useState("");

  function build(): FilterPayload | null {
    if (start && !DATE_RE.test(start)) {
      setError("Start date must be MM/DD/YYYY");
      return null;
    }
    if (end && !DATE_RE.test(end)) {
      setError("End date must be MM/DD/YYYY");
      return null;
    }
    const minN = minP === "" ? undefined : Number(minP);
    const maxN = maxP === "" ? undefined : Number(maxP);
    if ((minN !== undefined && isNaN(minN)) || (maxN !== undefined && isNaN(maxN))) {
      setError("Price must be numeric");
      return null;
    }
    setError("");
    const f: FilterPayload = {
      entity,
      keyword: keyword || undefined,
      statuses: selStatuses.length ? selStatuses : undefined,
      tags: tags ? tags.split(",").map((t) => t.trim()).filter(Boolean) : undefined,
      priority: priority || undefined,
      start_date: start || undefined,
      end_date: end || undefined,
      min_price_usd: minN,
      max_price_usd: maxN,
      sort_by: sortBy || undefined,
      sort_desc: sortDesc,
      page,
      size,
    };
    return f;
  }

  return (
    <div className="card" aria-label="Advanced filters">
      <div className="filter-grid">
        <div>
          <label>Keyword</label>
          <input value={keyword} onChange={(e) => setKeyword(e.target.value)} placeholder="Search terms" />
        </div>
        <div>
          <label>Status</label>
          <select
            multiple
            value={selStatuses}
            onChange={(e) =>
              setSelStatuses(Array.from(e.target.selectedOptions).map((o) => o.value))
            }
          >
            {statuses.map((s) => (
              <option key={s} value={s}>
                {s}
              </option>
            ))}
          </select>
        </div>
        <div>
          <label>Tags (comma)</label>
          <input value={tags} onChange={(e) => setTags(e.target.value)} placeholder="rush, inbound" />
        </div>
        <div>
          <label>Priority</label>
          <select value={priority} onChange={(e) => setPriority(e.target.value)}>
            <option value="">Any</option>
            <option value="standard">Standard</option>
            <option value="rush">Rush</option>
            <option value="critical">Critical</option>
          </select>
        </div>
        <div>
          <label>Start (MM/DD/YYYY)</label>
          <input value={start} onChange={(e) => setStart(e.target.value)} placeholder="01/01/2024" />
        </div>
        <div>
          <label>End (MM/DD/YYYY)</label>
          <input value={end} onChange={(e) => setEnd(e.target.value)} placeholder="12/31/2024" />
        </div>
        <div>
          <label>Min $USD</label>
          <input type="number" min={0} value={minP} onChange={(e) => setMinP(e.target.value)} />
        </div>
        <div>
          <label>Max $USD</label>
          <input type="number" min={0} value={maxP} onChange={(e) => setMaxP(e.target.value)} />
        </div>
        <div>
          <label>Sort by</label>
          <input value={sortBy} onChange={(e) => setSortBy(e.target.value)} placeholder="placed_at" />
          <label style={{ marginTop: "0.5rem" }}>
            <input type="checkbox" checked={sortDesc} onChange={(e) => setSortDesc(e.target.checked)} /> Desc
          </label>
        </div>
        <div>
          <label>Page / Size</label>
          <div style={{ display: "flex", gap: "0.5rem" }}>
            <input type="number" min={1} value={page} onChange={(e) => setPage(Number(e.target.value) || 1)} />
            <input type="number" min={1} max={500} value={size} onChange={(e) => setSize(Number(e.target.value) || 25)} />
          </div>
        </div>
      </div>
      {error && <div className="error-banner" role="alert">{error}</div>}
      <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.75rem" }}>
        <button
          className="btn"
          onClick={() => {
            const f = build();
            if (f) onApply(f);
          }}
        >
          Apply
        </button>
        {onSave && (
          <>
            <input
              placeholder="Save as…"
              value={name}
              onChange={(e) => setName(e.target.value)}
              style={{ maxWidth: "200px" }}
            />
            <button
              className="btn secondary"
              disabled={!name.trim()}
              onClick={() => {
                const f = build();
                if (!f || !name.trim()) return;
                onSave(name.trim(), f);
                setName("");
              }}
            >
              Save filter
            </button>
          </>
        )}
      </div>
    </div>
  );
}
