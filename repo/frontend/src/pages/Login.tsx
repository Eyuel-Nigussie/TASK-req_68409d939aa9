import { useState } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { useAuth } from "../hooks/useAuth";

export function LoginPage() {
  const { login } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const nav = useNavigate();
  const loc = useLocation();

  async function onSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (password.length < 10) {
      setErr("Password must be at least 10 characters.");
      return;
    }
    setBusy(true);
    try {
      await login(username, password);
      const dest = (loc.state as { from?: string } | null)?.from ?? "/";
      nav(dest, { replace: true });
    } catch (e: unknown) {
      setErr((e as Error).message);
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ maxWidth: 380, margin: "4rem auto" }}>
      <div className="card">
        <h1 className="brand">Operations Portal</h1>
        <form onSubmit={onSubmit}>
          <label htmlFor="login-username">Username</label>
          <input
            id="login-username"
            autoFocus
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            autoComplete="username"
          />
          <label htmlFor="login-password">Password (min 10 chars)</label>
          <input
            id="login-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
          {err && <div className="error-banner" role="alert" style={{ marginTop: "0.5rem" }}>{err}</div>}
          <button className="btn" type="submit" disabled={busy} style={{ marginTop: "0.75rem", width: "100%" }}>
            {busy ? "Signing in…" : "Sign in"}
          </button>
        </form>
        <p className="muted" style={{ marginTop: "0.75rem" }}>
          After 5 failed attempts an account is locked for 15 minutes.
        </p>
      </div>
    </div>
  );
}
