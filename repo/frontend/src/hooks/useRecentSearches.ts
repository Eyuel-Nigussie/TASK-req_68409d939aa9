import { useCallback, useEffect, useState } from "react";

// The product requirement says "remembers the last 20 searches per user on
// the device". We scope the key by user ID so multiple operators sharing a
// workstation don't see each other's history.

const LIMIT = 20;

function keyFor(userID: string) {
  return `oops.recent-searches.${userID || "anon"}`;
}

export function useRecentSearches(userID: string) {
  const [items, setItems] = useState<string[]>([]);

  useEffect(() => {
    const raw = localStorage.getItem(keyFor(userID));
    if (!raw) {
      setItems([]);
      return;
    }
    try {
      const parsed = JSON.parse(raw);
      if (Array.isArray(parsed)) setItems(parsed.slice(0, LIMIT));
    } catch {
      setItems([]);
    }
  }, [userID]);

  const add = useCallback(
    (q: string) => {
      const trimmed = q.trim();
      if (!trimmed) return;
      setItems((prev) => {
        const next = [trimmed, ...prev.filter((x) => x !== trimmed)].slice(0, LIMIT);
        localStorage.setItem(keyFor(userID), JSON.stringify(next));
        return next;
      });
    },
    [userID],
  );

  const clear = useCallback(() => {
    localStorage.removeItem(keyFor(userID));
    setItems([]);
  }, [userID]);

  return { items, add, clear };
}

// Exposed for tests.
export const _testing = { LIMIT, keyFor };
