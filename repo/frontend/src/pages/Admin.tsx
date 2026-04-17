import { useEffect, useState } from "react";
import { api } from "../api/client";

interface UserRow {
  id: string;
  username: string;
  role: string;
  disabled: boolean;
  lock_until?: string;
  failures?: number;
}

interface RefRangeRow {
  TestCode: string;
  Units: string;
  LowNormal?: number;
  HighNormal?: number;
  LowCritical?: number;
  HighCritical?: number;
  Demographic: string;
}

interface RouteRow {
  FromID: string;
  ToID: string;
  Miles: number;
}

interface PermRow {
  ID: string;
  Description: string;
}

const ROLES = ["front_desk", "lab_tech", "dispatch", "analyst", "admin"];

// AdminPage centralizes the operations an administrator needs:
//   - Create/edit users + roles + forced password reset + enable/disable
//   - Maintain the reference-range dictionary used for abnormal flagging
//   - Maintain the preloaded road-distance matrix
//   - View recent audit entries
export function AdminPage() {
  const [users, setUsers] = useState<UserRow[]>([]);
  const [newUser, setNewUser] = useState({ username: "", password: "", role: "front_desk" });
  const [banner, setBanner] = useState("");
  const [err, setErr] = useState("");

  const [ranges, setRanges] = useState<RefRangeRow[]>([]);
  const [routes, setRoutes] = useState<RouteRow[]>([]);
  const [audit, setAudit] = useState<Array<{ ID: string; At: string; ActorID: string; Entity: string; EntityID: string; Action: string; Reason: string }>>([]);
  const [permissions, setPermissions] = useState<PermRow[]>([]);
  const [rolePerms, setRolePerms] = useState<Record<string, Set<string>>>({});
  const [mapImage, setMapImage] = useState("");

  async function refresh() {
    try {
      setUsers(await api.adminListUsers());
      setRanges(await api.adminListRefRanges());
      setRoutes(await api.adminListRoutes());
      setPermissions(await api.adminListPermissions());
      const grants = await api.adminListRolePermissions();
      const map: Record<string, Set<string>> = {};
      for (const r of ROLES) map[r] = new Set();
      for (const g of grants) {
        if (!map[g.Role]) map[g.Role] = new Set();
        map[g.Role].add(g.PermissionID);
      }
      setRolePerms(map);
      // Listing audit directly uses fetch since the API client doesn't wrap it.
      const tok = localStorage.getItem("oops.session.token") ?? "";
      const r = await fetch(`/api/admin/audit`, { headers: { Authorization: `Bearer ${tok}` } });
      if (r.ok) setAudit(await r.json());
      try {
        const cfg = await api.getMapConfig();
        setMapImage(cfg.map_image_url ?? "");
      } catch {
        setMapImage("");
      }
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function saveMapImage() {
    setErr(""); setBanner("");
    try {
      await api.adminPutMapConfig(mapImage.trim());
      setBanner("Saved service-area map image");
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  function togglePerm(role: string, permID: string) {
    setRolePerms((prev) => {
      const next = { ...prev };
      const set = new Set(next[role] ?? []);
      if (set.has(permID)) set.delete(permID);
      else set.add(permID);
      next[role] = set;
      return next;
    });
  }

  async function savePerms(role: string) {
    try {
      const ids = Array.from(rolePerms[role] ?? []);
      await api.adminSetRolePermissions(role, ids);
      setBanner(`Saved permissions for ${role}`);
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }
  useEffect(() => {
    refresh();
  }, []);

  async function createUser(e: React.FormEvent) {
    e.preventDefault();
    setErr(""); setBanner("");
    if (newUser.password.length < 10) {
      setErr("Password must be ≥10 characters.");
      return;
    }
    try {
      await api.adminCreateUser(newUser);
      setBanner(`Created user "${newUser.username}"`);
      setNewUser({ username: "", password: "", role: "front_desk" });
      refresh();
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function toggleDisable(u: UserRow) {
    try {
      await api.adminUpdateUser(u.id, { disabled: !u.disabled });
      refresh();
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function changeRole(u: UserRow, role: string) {
    try {
      await api.adminUpdateUser(u.id, { role });
      refresh();
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function saveRanges() {
    try {
      await api.adminPutRefRanges(ranges.map((r) => ({
        TestCode: r.TestCode, Units: r.Units,
        LowNormal: r.LowNormal, HighNormal: r.HighNormal,
        LowCritical: r.LowCritical, HighCritical: r.HighCritical,
        Demographic: r.Demographic,
      })));
      setBanner(`Saved ${ranges.length} reference ranges`);
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  async function saveRoutes() {
    try {
      await api.adminPutRoutes(routes);
      setBanner(`Saved ${routes.length} route entries`);
    } catch (e: unknown) {
      setErr((e as Error).message);
    }
  }

  return (
    <div>
      {banner && <div className="ok-banner" role="status">{banner}</div>}
      {err && <div className="error-banner" role="alert">{err}</div>}

      <div className="card">
        <h2>Users</h2>
        <form onSubmit={createUser} className="filter-grid">
          <div><label>Username</label><input value={newUser.username} onChange={(e) => setNewUser({ ...newUser, username: e.target.value })} /></div>
          <div><label>Password (≥10)</label><input type="password" value={newUser.password} onChange={(e) => setNewUser({ ...newUser, password: e.target.value })} /></div>
          <div>
            <label>Role</label>
            <select value={newUser.role} onChange={(e) => setNewUser({ ...newUser, role: e.target.value })}>
              <option value="front_desk">Front desk</option>
              <option value="lab_tech">Lab technician</option>
              <option value="dispatch">Dispatch</option>
              <option value="analyst">Analyst</option>
              <option value="admin">Administrator</option>
            </select>
          </div>
          <div style={{ alignSelf: "end" }}><button className="btn" type="submit">Create user</button></div>
        </form>
        <table>
          <thead><tr><th>Username</th><th>Role</th><th>Status</th><th>Failures</th><th></th></tr></thead>
          <tbody>
            {users.map((u) => (
              <tr key={u.id}>
                <td>{u.username}</td>
                <td>
                  <select value={u.role} onChange={(e) => changeRole(u, e.target.value)}>
                    <option value="front_desk">front_desk</option>
                    <option value="lab_tech">lab_tech</option>
                    <option value="dispatch">dispatch</option>
                    <option value="analyst">analyst</option>
                    <option value="admin">admin</option>
                  </select>
                </td>
                <td>{u.disabled ? "disabled" : "active"}</td>
                <td>{u.failures ?? 0}</td>
                <td><button className="btn secondary" onClick={() => toggleDisable(u)}>{u.disabled ? "Enable" : "Disable"}</button></td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      <div className="card">
        <h2>Role permissions</h2>
        <p className="muted">
          Grants take effect immediately — no session reissue is required.
          Individual users can also receive additional grants from the
          users table.
        </p>
        <div style={{ overflowX: "auto" }}>
          <table>
            <thead>
              <tr>
                <th>Permission</th>
                {ROLES.map((r) => (
                  <th key={r} style={{ textAlign: "center" }}>{r}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {permissions.map((p) => (
                <tr key={p.ID}>
                  <td>
                    <span className="kbd">{p.ID}</span>
                    <div className="muted" style={{ fontSize: "0.85em" }}>{p.Description}</div>
                  </td>
                  {ROLES.map((r) => (
                    <td key={r} style={{ textAlign: "center" }}>
                      <input
                        type="checkbox"
                        checked={rolePerms[r]?.has(p.ID) ?? false}
                        onChange={() => togglePerm(r, p.ID)}
                      />
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
            <tfoot>
              <tr>
                <td></td>
                {ROLES.map((r) => (
                  <td key={r} style={{ textAlign: "center" }}>
                    <button className="btn secondary" onClick={() => savePerms(r)}>Save</button>
                  </td>
                ))}
              </tr>
            </tfoot>
          </table>
        </div>
      </div>

      <div className="card">
        <h2>Reference ranges</h2>
        <p className="muted">Numeric values may be left blank for no bound. Measurements outside <strong>critical</strong> bounds are shown as critical.</p>
        <table>
          <thead>
            <tr><th>Test code</th><th>Units</th><th>Low normal</th><th>High normal</th><th>Low critical</th><th>High critical</th><th>Demo</th><th></th></tr>
          </thead>
          <tbody>
            {ranges.map((r, i) => (
              <tr key={i}>
                <td><input value={r.TestCode} onChange={(e) => setRanges(ranges.map((x, j) => j === i ? { ...x, TestCode: e.target.value } : x))} /></td>
                <td><input value={r.Units} onChange={(e) => setRanges(ranges.map((x, j) => j === i ? { ...x, Units: e.target.value } : x))} /></td>
                <td><input type="number" value={r.LowNormal ?? ""} onChange={(e) => setRanges(ranges.map((x, j) => j === i ? { ...x, LowNormal: e.target.value === "" ? undefined : Number(e.target.value) } : x))} /></td>
                <td><input type="number" value={r.HighNormal ?? ""} onChange={(e) => setRanges(ranges.map((x, j) => j === i ? { ...x, HighNormal: e.target.value === "" ? undefined : Number(e.target.value) } : x))} /></td>
                <td><input type="number" value={r.LowCritical ?? ""} onChange={(e) => setRanges(ranges.map((x, j) => j === i ? { ...x, LowCritical: e.target.value === "" ? undefined : Number(e.target.value) } : x))} /></td>
                <td><input type="number" value={r.HighCritical ?? ""} onChange={(e) => setRanges(ranges.map((x, j) => j === i ? { ...x, HighCritical: e.target.value === "" ? undefined : Number(e.target.value) } : x))} /></td>
                <td><input value={r.Demographic} onChange={(e) => setRanges(ranges.map((x, j) => j === i ? { ...x, Demographic: e.target.value } : x))} /></td>
                <td><button className="btn danger" onClick={() => setRanges(ranges.filter((_, j) => j !== i))}>x</button></td>
              </tr>
            ))}
          </tbody>
        </table>
        <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.5rem" }}>
          <button className="btn secondary" onClick={() => setRanges([...ranges, { TestCode: "", Units: "", Demographic: "" }])}>+ Add row</button>
          <button className="btn" onClick={saveRanges}>Save ranges</button>
        </div>
      </div>

      <div className="card">
        <h2>Route table</h2>
        <p className="muted">Preloaded road distances. When an origin/destination pair is present, it is used instead of straight-line distance for fee quotes.</p>
        <table>
          <thead><tr><th>From</th><th>To</th><th>Miles</th><th></th></tr></thead>
          <tbody>
            {routes.map((r, i) => (
              <tr key={i}>
                <td><input value={r.FromID} onChange={(e) => setRoutes(routes.map((x, j) => j === i ? { ...x, FromID: e.target.value } : x))} /></td>
                <td><input value={r.ToID} onChange={(e) => setRoutes(routes.map((x, j) => j === i ? { ...x, ToID: e.target.value } : x))} /></td>
                <td><input type="number" value={r.Miles} onChange={(e) => setRoutes(routes.map((x, j) => j === i ? { ...x, Miles: Number(e.target.value) } : x))} /></td>
                <td><button className="btn danger" onClick={() => setRoutes(routes.filter((_, j) => j !== i))}>x</button></td>
              </tr>
            ))}
          </tbody>
        </table>
        <div style={{ display: "flex", gap: "0.5rem", marginTop: "0.5rem" }}>
          <button className="btn secondary" onClick={() => setRoutes([...routes, { FromID: "", ToID: "", Miles: 0 }])}>+ Add row</button>
          <button className="btn" onClick={saveRoutes}>Save route table</button>
        </div>
      </div>

      <div className="card">
        <h2>Service-area map image</h2>
        <p className="muted">
          URL of the raster image rendered behind the polygon overlay on
          the dispatch map. Supports http(s)://, origin-relative paths,
          and inline data: URIs. Clear the value to return to the
          polygon-only view.
        </p>
        <div style={{ display: "flex", gap: "0.5rem", alignItems: "center" }}>
          <input
            aria-label="Map image URL"
            placeholder="/static/service-area.png or https://..."
            value={mapImage}
            onChange={(e) => setMapImage(e.target.value)}
            style={{ flex: 1 }}
          />
          <button className="btn" onClick={saveMapImage}>Save map image</button>
        </div>
      </div>

      <div className="card">
        <h2>Recent audit entries</h2>
        <table>
          <thead><tr><th>Time</th><th>Actor</th><th>Entity</th><th>ID</th><th>Action</th><th>Reason</th></tr></thead>
          <tbody>
            {audit.slice(-30).reverse().map((a) => (
              <tr key={a.ID}>
                <td>{new Date(a.At).toLocaleString()}</td>
                <td>{a.ActorID || "—"}</td>
                <td>{a.Entity}</td>
                <td>{a.EntityID}</td>
                <td>{a.Action}</td>
                <td>{a.Reason || ""}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  );
}
