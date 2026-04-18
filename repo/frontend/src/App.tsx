import { NavLink, Navigate, Route, Routes, useLocation } from "react-router-dom";
import { AuthProvider, useAuth } from "./hooks/useAuth";
import { GlobalSearch } from "./components/GlobalSearch";
import { LoginPage } from "./pages/Login";
import { Dashboard } from "./pages/Dashboard";
import { OrdersPage } from "./pages/Orders";
import { OrderDetailPage } from "./pages/OrderDetail";
import { DispatchPage } from "./pages/Dispatch";
import { LabPage } from "./pages/Lab";
import { ReportDetailPage } from "./pages/ReportDetail";
import { AddressBookPage } from "./pages/AddressBook";
import { CustomersPage } from "./pages/Customers";
import { CustomerDetailPage } from "./pages/CustomerDetail";
import { AdminPage } from "./pages/Admin";
import { AnalyticsPage } from "./pages/Analytics";

export default function App() {
  return (
    <AuthProvider>
      <Shell />
    </AuthProvider>
  );
}

function Shell() {
  const { user, loading, logout } = useAuth();
  const loc = useLocation();

  if (loading) return <div style={{ padding: "2rem" }}>Loading…</div>;

  if (!user) {
    if (loc.pathname !== "/login") {
      return <Navigate to="/login" state={{ from: loc.pathname }} replace />;
    }
    return (
      <Routes>
        <Route path="/login" element={<LoginPage />} />
        <Route path="*" element={<LoginPage />} />
      </Routes>
    );
  }

  const role = user.role;
  const can = (roles: string[]) => roles.includes(role);

  // Derive a workspace key from the path so CSS can show the matching
  // accent stripe. Using an attribute instead of inline styles keeps the
  // styling declarative and easy to extend.
  const workspace = workspaceFor(loc.pathname);

  return (
    <div className="app-shell" data-workspace={workspace}>
      <div className="topbar">
        <div className="brand">OOP Portal</div>
        <GlobalSearch />
        <div style={{ marginLeft: "auto", display: "flex", alignItems: "center", gap: "0.5rem" }}>
          <span className="muted">
            {user.username} ({user.role})
          </span>
          <button className="btn secondary" onClick={logout}>Sign out</button>
        </div>
      </div>
      <div className="workspace-stripe" aria-hidden="true" />
      <div className="layout">
        <nav className="sidenav card" aria-label="Main">
          <NavLink to="/" end className={({ isActive }) => (isActive ? "active" : "")}>Dashboard</NavLink>
          {can(["front_desk", "admin", "analyst", "dispatch"]) && (
            <NavLink to="/customers" className={({ isActive }) => (isActive ? "active" : "")}>Customers</NavLink>
          )}
          {can(["front_desk", "admin", "analyst", "dispatch"]) && (
            <NavLink to="/orders" className={({ isActive }) => (isActive ? "active" : "")}>Orders</NavLink>
          )}
          {can(["lab_tech", "admin", "analyst"]) && (
            <NavLink to="/lab" className={({ isActive }) => (isActive ? "active" : "")}>Lab</NavLink>
          )}
          {can(["dispatch", "admin"]) && (
            <NavLink to="/dispatch" className={({ isActive }) => (isActive ? "active" : "")}>Dispatch</NavLink>
          )}
          {can(["analyst", "admin"]) && (
            <NavLink to="/analytics" className={({ isActive }) => (isActive ? "active" : "")}>Analytics</NavLink>
          )}
          <NavLink to="/address-book" className={({ isActive }) => (isActive ? "active" : "")}>Address book</NavLink>
          {can(["admin"]) && (
            <NavLink to="/admin" className={({ isActive }) => (isActive ? "active" : "")}>Admin</NavLink>
          )}
        </nav>
        <main>
          <Routes>
            <Route path="/" element={<Dashboard />} />
            <Route path="/customers" element={<CustomersPage />} />
            <Route path="/customers/:id" element={<CustomerDetailPage />} />
            <Route path="/orders" element={<OrdersPage />} />
            <Route path="/orders/:id" element={<OrderDetailPage />} />
            <Route path="/lab" element={<LabPage />} />
            <Route path="/reports/:id" element={<ReportDetailPage />} />
            <Route path="/dispatch" element={<DispatchPage />} />
            <Route path="/analytics" element={<AnalyticsPage />} />
            <Route path="/address-book" element={<AddressBookPage />} />
            <Route path="/admin" element={<AdminPage />} />
            <Route path="*" element={<Navigate to="/" replace />} />
          </Routes>
        </main>
      </div>
    </div>
  );
}

function workspaceFor(path: string): string {
  if (path.startsWith("/customers")) return "customers";
  if (path.startsWith("/orders")) return "orders";
  if (path.startsWith("/lab") || path.startsWith("/reports")) return "lab";
  if (path.startsWith("/dispatch")) return "dispatch";
  if (path.startsWith("/analytics")) return "analytics";
  if (path.startsWith("/admin")) return "admin";
  return "dashboard";
}
