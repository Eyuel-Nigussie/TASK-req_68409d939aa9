import { useEffect, useRef, useState } from "react";
import { useNavigate } from "react-router-dom";
import { api } from "../api/client";
import { useAuth } from "../hooks/useAuth";
import { useRecentSearches } from "../hooks/useRecentSearches";
import { fuzzyScore } from "../lib/fuzzy";

interface Suggestion {
  id: string;
  label: string;
  kind: string;
  score: number;
}

export function GlobalSearch() {
  const { user } = useAuth();
  const recent = useRecentSearches(user?.id ?? "anon");
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);
  const [hits, setHits] = useState<Suggestion[]>([]);
  const boxRef = useRef<HTMLDivElement>(null);
  const nav = useNavigate();

  // Debounce queries to avoid hammering the server on every keystroke.
  useEffect(() => {
    if (!query) {
      setHits([]);
      return;
    }
    const t = setTimeout(async () => {
      try {
        const sugs = await api.searchGlobal(query);
        const scored = sugs
          .map((s) => ({ id: s.ID, label: s.Label, kind: s.Kind, score: fuzzyScore(query, s.Label) }))
          .sort((a, b) => b.score - a.score);
        setHits(scored);
      } catch {
        setHits([]);
      }
    }, 180);
    return () => clearTimeout(t);
  }, [query]);

  // Close the dropdown on outside click.
  useEffect(() => {
    function onClick(e: MouseEvent) {
      if (!boxRef.current?.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onClick);
    return () => document.removeEventListener("mousedown", onClick);
  }, []);

  function onSelect(s: Suggestion) {
    recent.add(query);
    setOpen(false);
    setQuery("");
    const dest =
      s.kind === "customer" ? `/customers/${s.id}` :
      s.kind === "report" ? `/reports/${s.id}` :
      s.kind === "order" ? `/orders/${s.id}` :
      "/";
    nav(dest);
  }

  function onRecent(q: string) {
    setQuery(q);
    setOpen(true);
  }

  return (
    <div className="global-search" ref={boxRef}>
      <input
        aria-label="Global search"
        placeholder="Search customers, orders, reports…"
        value={query}
        onFocus={() => setOpen(true)}
        onChange={(e) => {
          setQuery(e.target.value);
          setOpen(true);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter" && hits[0]) onSelect(hits[0]);
        }}
      />
      {open && (
        <div className="dropdown" role="listbox">
          {hits.length === 0 && query === "" && recent.items.length === 0 && (
            <div className="recent">No recent searches yet</div>
          )}
          {hits.length === 0 && recent.items.length > 0 && query === "" && (
            <>
              <div className="recent">Recent searches</div>
              {recent.items.map((q) => (
                <div key={q} role="option" onClick={() => onRecent(q)}>
                  <span className="muted">↺</span> {q}
                </div>
              ))}
            </>
          )}
          {hits.map((h) => (
            <div key={h.kind + ":" + h.id} role="option" onClick={() => onSelect(h)}>
              <span className="muted">{h.kind}</span> · {h.label}
            </div>
          ))}
          {hits.length === 0 && query !== "" && <div className="recent">No suggestions (try fewer keywords)</div>}
        </div>
      )}
    </div>
  );
}
