// Thin fetch wrapper that keeps auth + error handling in one place.
// The backend is exposed on the same origin via Vite's dev proxy or the
// production static-file server, so no cross-origin negotiation is needed.

type HttpMethod = "GET" | "POST" | "PUT" | "PATCH" | "DELETE";

const TOKEN_KEY = "oops.session.token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string | null) {
  if (token) localStorage.setItem(TOKEN_KEY, token);
  else localStorage.removeItem(TOKEN_KEY);
}

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

export async function request<T>(
  method: HttpMethod,
  path: string,
  body?: unknown,
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    "X-Workstation": workstationId(),
    // Emit the operator-local timestamp so the audit log can reconstruct
    // the user's clock independent of the server's.
    "X-Workstation-Time": new Date().toISOString(),
  };
  const tok = getToken();
  if (tok) headers["Authorization"] = `Bearer ${tok}`;

  const res = await fetch(path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (res.status === 204) return undefined as unknown as T;
  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;
  if (!res.ok) {
    const msg = (data && (data.message || data.error)) || `HTTP ${res.status}`;
    throw new ApiError(res.status, msg);
  }
  return data as T;
}

// workstationId returns a stable per-device identifier so the audit log
// can trace which workstation performed an action.
function workstationId(): string {
  const key = "oops.workstation";
  let v = localStorage.getItem(key);
  if (!v) {
    v =
      "ws-" +
      Math.random().toString(36).slice(2, 8) +
      "-" +
      Date.now().toString(36);
    localStorage.setItem(key, v);
  }
  return v;
}

// ---------- Typed endpoint wrappers ----------

export const api = {
  login(username: string, password: string) {
    return request<{
      token: string;
      user: { id: string; username: string; role: string; must_rotate_password?: boolean };
      expires_at: string;
      must_rotate_password?: boolean;
    }>("POST", "/api/auth/login", { username, password });
  },
  logout() {
    return request<void>("POST", "/api/auth/logout");
  },
  whoami() {
    return request<{
      id: string;
      username: string;
      role: string;
      expires_at: string;
      must_rotate_password?: boolean;
    }>("GET", "/api/auth/whoami");
  },
  // rotatePassword clears the must_rotate_password gate for sessions
  // seeded with the shared demo credential (L2). On success the same
  // session token can access the rest of the API without a re-login.
  rotatePassword(oldPassword: string, newPassword: string) {
    return request<void>("POST", "/api/auth/rotate-password", {
      old_password: oldPassword,
      new_password: newPassword,
    });
  },
  searchGlobal(q: string) {
    return request<Array<{ ID: string; Label: string; Kind: string; Score: number }>>(
      "GET",
      `/api/search?q=${encodeURIComponent(q)}`,
    );
  },
  listOrders(params: { status?: string; limit?: number; offset?: number }) {
    const qs = new URLSearchParams();
    if (params.status) qs.set("status", params.status);
    if (params.limit) qs.set("limit", String(params.limit));
    if (params.offset) qs.set("offset", String(params.offset));
    return request<OrderView[]>("GET", `/api/orders?${qs.toString()}`);
  },
  createOrder(body: {
    customer_id?: string;
    total_cents: number;
    priority?: string;
    tags?: string[];
    items?: Array<{ SKU: string; Description?: string; Qty: number; Backordered?: boolean }>;
    delivery_street?: string;
    delivery_city?: string;
    delivery_state?: string;
    delivery_zip?: string;
  }) {
    return request<OrderView>("POST", "/api/orders", body);
  },
  transitionOrder(id: string, to: string, reason = "", note = "") {
    return request<OrderView>("POST", `/api/orders/${id}/transitions`, { to, reason, note });
  },
  listExceptions() {
    return request<Array<{ OrderID: string; Kind: string; DetectedAt: string; Description: string }>>(
      "GET",
      "/api/exceptions",
    );
  },
  planOOS(orderID: string, available: string[], backordered: string[]) {
    return request<{ SuggestSplit: boolean; AvailableItems: string[]; BackorderedItems: string[]; Description: string }>(
      "POST",
      `/api/orders/${orderID}/out-of-stock/plan`,
      { available, backordered },
    );
  },
  validatePin(lat: number, lng: number) {
    return request<{ valid: boolean; region_id?: string; reason?: string }>(
      "POST",
      "/api/dispatch/validate-pin",
      { lat, lng },
    );
  },
  quoteFee(input: { from_id: string; to_id: string; from: { lat: number; lng: number }; to: { lat: number; lng: number } }) {
    return request<{ region_id: string; miles: number; method: string; fee_cents: number; fee_usd: number }>(
      "POST",
      "/api/dispatch/fee-quote",
      input,
    );
  },
  listRegions() {
    return request<Array<{ Polygon: { ID: string; Vertices: Array<{ Lat: number; Lng: number }> }; BaseFeeCents: number; PerMileFeeCents: number }>>(
      "GET",
      "/api/dispatch/regions",
    );
  },
  getMapConfig() {
    return request<{ map_image_url: string }>("GET", "/api/dispatch/map-config");
  },
  adminPutMapConfig(mapImageURL: string) {
    return request<{ map_image_url: string }>("PUT", "/api/admin/map-config", {
      map_image_url: mapImageURL,
    });
  },
  listTestItems(sampleID: string) {
    return request<Array<{ ID: string; SampleID: string; TestCode: string; Instructions: string; CreatedAt: string }>>(
      "GET",
      `/api/samples/${encodeURIComponent(sampleID)}/test-items`,
    );
  },
  exportOrdersCSV(filter: FilterPayload): Promise<Blob> {
    const tok = getToken();
    return fetch("/api/exports/orders.csv", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "X-Workstation": "browser",
        ...(tok ? { Authorization: `Bearer ${tok}` } : {}),
      },
      body: JSON.stringify(filter),
    }).then(async (res) => {
      if (!res.ok) {
        const text = await res.text();
        throw new ApiError(res.status, text || `HTTP ${res.status}`);
      }
      return res.blob();
    });
  },
  listSamples(status?: string) {
    const qs = status ? `?status=${encodeURIComponent(status)}` : "";
    return request<SampleView[]>("GET", `/api/samples${qs}`);
  },
  createSample(body: { order_id?: string; customer_id?: string; test_codes: string[]; notes?: string }) {
    return request<SampleView>("POST", "/api/samples", body);
  },
  transitionSample(id: string, to: string) {
    return request<SampleView>("POST", `/api/samples/${id}/transitions`, { to });
  },
  createReport(sampleID: string, body: { title: string; narrative: string; measurements: MeasurementInput[]; demographic?: string }) {
    return request<ReportView>("POST", `/api/samples/${sampleID}/report`, body);
  },
  correctReport(reportID: string, body: { expected_version: number; title: string; narrative: string; measurements: MeasurementInput[]; reason: string; demographic?: string }) {
    return request<ReportView>("POST", `/api/reports/${reportID}/correct`, body);
  },
  listReports(limit = 25, offset = 0) {
    return request<ReportView[]>("GET", `/api/reports?limit=${limit}&offset=${offset}`);
  },
  searchReports(q: string) {
    return request<ReportView[]>("GET", `/api/reports/search?q=${encodeURIComponent(q)}`);
  },
  listArchivedReports() {
    return request<ReportView[]>("GET", "/api/reports/archived");
  },
  archiveReport(id: string, note: string) {
    return request<ReportView>("POST", `/api/reports/${encodeURIComponent(id)}/archive`, { note });
  },
  updateInventory(orderID: string, sku: string, backordered: boolean, note = "") {
    return request<OrderView>("POST", `/api/orders/${encodeURIComponent(orderID)}/inventory`, { sku, backordered, note });
  },
  ordersByAddress(p: { street?: string; city?: string; zip?: string }) {
    const qs = new URLSearchParams();
    Object.entries(p).forEach(([k, v]) => v && qs.set(k, v));
    return request<OrderView[]>("GET", `/api/orders/by-address?${qs.toString()}`);
  },
  createCustomer(body: {
    name: string; identifier?: string; street?: string; city?: string; state?: string;
    zip?: string; phone?: string; email?: string; tags?: string[];
  }) {
    return request<CustomerView>("POST", "/api/customers", body);
  },
  getCustomer(id: string) {
    return request<CustomerView>("GET", `/api/customers/${encodeURIComponent(id)}`);
  },
  getOrder(id: string) {
    return request<OrderView>("GET", `/api/orders/${encodeURIComponent(id)}`);
  },
  getReport(id: string) {
    return request<ReportView>("GET", `/api/reports/${encodeURIComponent(id)}`);
  },
  queryOrders(body: FilterPayload) {
    return request<{ items: OrderView[]; total: number; page: number; size: number; has_next: boolean }>(
      "POST",
      "/api/orders/query",
      body,
    );
  },
  adminListUsers() {
    return request<Array<{ id: string; username: string; role: string; disabled: boolean; created_at: string; lock_until: string; failures: number }>>(
      "GET",
      "/api/admin/users",
    );
  },
  adminCreateUser(body: { username: string; password: string; role: string }) {
    return request<{ id: string; username: string; role: string }>("POST", "/api/admin/users", body);
  },
  adminUpdateUser(id: string, body: { role?: string; password?: string; disabled?: boolean }) {
    return request<{ id: string; role: string; disabled: boolean }>("PATCH", `/api/admin/users/${encodeURIComponent(id)}`, body);
  },
  adminListRefRanges() {
    return request<Array<{ TestCode: string; Units: string; LowNormal?: number; HighNormal?: number; LowCritical?: number; HighCritical?: number; Demographic: string }>>(
      "GET",
      "/api/admin/reference-ranges",
    );
  },
  adminPutRefRanges(ranges: Array<{ TestCode: string; Units?: string; LowNormal?: number; HighNormal?: number; LowCritical?: number; HighCritical?: number; Demographic?: string }>) {
    return request<{ count: number }>("PUT", "/api/admin/reference-ranges", { ranges });
  },
  adminListRoutes() {
    return request<Array<{ FromID: string; ToID: string; Miles: number }>>("GET", "/api/admin/route-table");
  },
  adminPutRoutes(routes: Array<{ FromID: string; ToID: string; Miles: number }>) {
    return request<{ count: number }>("PUT", "/api/admin/route-table", { routes });
  },
  adminPutServiceRegions(regions: Array<{ id: string; vertices: number[][]; base_fee_cents: number; per_mile_fee_cents: number }>) {
    return request<{ count: number }>("PUT", "/api/admin/service-regions", { regions });
  },
  adminListPermissions() {
    return request<Array<{ ID: string; Description: string }>>("GET", "/api/admin/permissions");
  },
  adminListRolePermissions() {
    return request<Array<{ Role: string; PermissionID: string }>>("GET", "/api/admin/role-permissions");
  },
  adminSetRolePermissions(role: string, permissionIDs: string[]) {
    return request<{ role: string; permissions: string[] }>(
      "PUT",
      `/api/admin/role-permissions/${encodeURIComponent(role)}`,
      { permission_ids: permissionIDs },
    );
  },
  adminListUserPermissions(userID: string) {
    return request<string[]>("GET", `/api/admin/users/${encodeURIComponent(userID)}/permissions`);
  },
  adminSetUserPermissions(userID: string, permissionIDs: string[]) {
    return request<{ user_id: string; permissions: string[] }>(
      "PUT",
      `/api/admin/users/${encodeURIComponent(userID)}/permissions`,
      { permission_ids: permissionIDs },
    );
  },
  analyticsSummary(fromUnix?: number, toUnix?: number) {
    const qs = new URLSearchParams();
    if (fromUnix !== undefined) qs.set("from", String(fromUnix));
    if (toUnix !== undefined) qs.set("to", String(toUnix));
    return request<{
      order_status: Record<string, number>;
      sample_status: Record<string, number>;
      orders_per_day: Array<{ Day: string; Count: number }>;
      abnormal_rate: { TotalMeasurements: number; AbnormalMeasurements: number; Rate: number };
      exceptions: Record<string, number>;
    }>("GET", `/api/analytics/summary?${qs.toString()}`);
  },
  searchCustomers(q: string) {
    return request<CustomerView[]>("GET", `/api/customers?q=${encodeURIComponent(q)}`);
  },
  customersByAddress(p: { street?: string; city?: string; zip?: string }) {
    const qs = new URLSearchParams();
    Object.entries(p).forEach(([k, v]) => v && qs.set(k, v));
    return request<CustomerView[]>("GET", `/api/customers/by-address?${qs.toString()}`);
  },
  listAddressBook() {
    return request<AddressBookEntry[]>("GET", "/api/address-book");
  },
  saveAddress(entry: Omit<AddressBookEntry, "id" | "created_at">) {
    return request<{ id: string; label: string }>("POST", "/api/address-book", entry);
  },
  deleteAddress(id: string) {
    return request<void>("DELETE", `/api/address-book/${id}`);
  },
  listSavedFilters() {
    return request<SavedFilter[]>("GET", "/api/saved-filters");
  },
  saveFilter(name: string, filter: FilterPayload) {
    return request<SavedFilter>("POST", "/api/saved-filters", { name, filter });
  },
};

// ---------- Shared types ----------

export interface CustomerView {
  id: string;
  name: string;
  identifier?: string;
  street?: string;
  city?: string;
  state?: string;
  zip?: string;
  phone?: string;
  email?: string;
  tags?: string[];
  created_at: string;
  updated_at: string;
}

export interface AddressBookEntry {
  id: string;
  label: string;
  customer_id?: string;
  street?: string;
  city?: string;
  state?: string;
  zip?: string;
  lat?: number;
  lng?: number;
  created_at?: string;
}

export interface OrderView {
  ID: string;
  CustomerID?: string;
  Status: string;
  PlacedAt: string;
  UpdatedAt: string;
  TotalCents: number;
  Priority?: string;
  Tags?: string[];
  Items?: Array<{ SKU: string; Description?: string; Qty: number; Backordered: boolean }>;
  DeliveryStreet?: string;
  DeliveryCity?: string;
  DeliveryState?: string;
  DeliveryZIP?: string;
  Events?: Array<{ ID: string; At: string; From: string; To: string; Actor: string; Reason?: string; Note?: string }>;
}

export interface SampleView {
  ID: string;
  OrderID?: string;
  CustomerID?: string;
  Status: string;
  CollectedAt: string;
  UpdatedAt: string;
  TestCodes: string[];
  Notes?: string;
}

export interface MeasurementInput {
  test_code: string;
  value: number;
  units?: string;
  unmeasurable?: boolean;
}

export interface ReportView {
  ID: string;
  SampleID: string;
  Version: number;
  Status: "draft" | "issued" | "superseded";
  Title: string;
  Narrative: string;
  Measurements: Array<{ TestCode: string; Value: number; Units?: string; Unmeasurable?: boolean; Flag: string }>;
  AuthorID: string;
  ReasonNote?: string;
  IssuedAt?: string;
  SupersededByID?: string;
  ArchivedAt?: string;
  ArchivedBy?: string;
  ArchiveNote?: string;
  SearchText?: string;
}

export interface FilterPayload {
  entity: "customer" | "order" | "sample" | "report";
  keyword?: string;
  statuses?: string[];
  tags?: string[];
  priority?: string;
  start_date?: string; // MM/DD/YYYY
  end_date?: string;
  min_price_usd?: number;
  max_price_usd?: number;
  sort_by?: string;
  sort_desc?: boolean;
  page?: number;
  size?: number;
}

export interface SavedFilter {
  ID: string;
  OwnerID: string;
  Name: string;
  Payload: string;
  Key: string;
  CreatedAt: string;
}
