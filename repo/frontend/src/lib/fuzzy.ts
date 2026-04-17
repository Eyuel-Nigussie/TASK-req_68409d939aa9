// Light-weight fuzzy matching used by the global search dropdown so suggestion
// typos are tolerated even when the backend returned no rows. Mirrors the
// scoring behavior of the Go `search` package but simplified for UI use.

export function levenshtein(a: string, b: string): number {
  if (a === b) return 0;
  if (!a.length) return b.length;
  if (!b.length) return a.length;
  let prev = new Array(b.length + 1);
  let curr = new Array(b.length + 1);
  for (let j = 0; j <= b.length; j++) prev[j] = j;
  for (let i = 1; i <= a.length; i++) {
    curr[0] = i;
    for (let j = 1; j <= b.length; j++) {
      const cost = a[i - 1] === b[j - 1] ? 0 : 1;
      curr[j] = Math.min(prev[j] + 1, curr[j - 1] + 1, prev[j - 1] + cost);
    }
    [prev, curr] = [curr, prev];
  }
  return prev[b.length];
}

export function fuzzyScore(query: string, candidate: string): number {
  const q = query.trim().toLowerCase();
  const c = candidate.trim().toLowerCase();
  if (!q || !c) return 0;
  if (q === c) return 1;
  let score = 0;
  if (c.includes(q)) score += 0.6;
  if (c.startsWith(q)) score += 0.3;
  const qTokens = q.split(/\s+/);
  const cTokens = c.split(/\s+/);
  let tokenScore = 0;
  for (const qt of qTokens) {
    let best = 0;
    for (const ct of cTokens) {
      if (qt === ct) {
        best = 1;
        break;
      }
      const d = levenshtein(qt, ct);
      const maxLen = Math.max(qt.length, ct.length);
      const tol = Math.max(1, Math.floor(maxLen / 5));
      if (d <= tol) {
        const s = 1 - d / (maxLen + 1);
        if (s > best) best = s;
      }
    }
    tokenScore += best;
  }
  tokenScore /= qTokens.length || 1;
  score += tokenScore * 0.4;
  return Math.min(1, score);
}
