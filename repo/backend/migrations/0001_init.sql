-- Unified Offline Operations Portal schema.
-- Targets PostgreSQL 14+. All timestamps are stored in UTC.
-- Sensitive fields (identifier, street) arrive pre-encrypted from the app
-- layer; the DB does not have the key material.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- =============== Users & roles ===============

CREATE TABLE users (
    id                     TEXT PRIMARY KEY,
    username               TEXT NOT NULL UNIQUE,
    role                   TEXT NOT NULL CHECK (role IN ('front_desk','lab_tech','dispatch','analyst','admin')),
    password_hash          TEXT NOT NULL,
    disabled               BOOLEAN NOT NULL DEFAULT FALSE,
    -- must_rotate_password is set when the account was provisioned with
    -- a shared/demo password (SEED_DEMO_USERS=1). The auth layer refuses
    -- all API calls from sessions belonging to such accounts except
    -- /api/auth/rotate-password, /api/auth/logout, and /api/auth/whoami
    -- so a leaked README password cannot be used in production (L2).
    must_rotate_password   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at             TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at             TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Authorization is expressed via a permission catalog plus per-role grants
-- so an administrator can tune what each role can do without a deploy.
-- Individual user grants are additive to the role grants.
CREATE TABLE permissions (
    id          TEXT PRIMARY KEY,        -- e.g., "orders.write"
    description TEXT NOT NULL
);

CREATE TABLE role_permissions (
    role          TEXT NOT NULL,
    permission_id TEXT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (role, permission_id)
);

CREATE TABLE user_permissions (
    user_id       TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id TEXT NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, permission_id)
);

-- Persistent login-attempt tracking. Lockout is stored so a process
-- restart cannot be used to bypass the 5-fail / 15-minute policy.
CREATE TABLE login_attempts (
    username      TEXT PRIMARY KEY,
    failures      INT NOT NULL DEFAULT 0,
    locked_until  TIMESTAMPTZ,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============== Customers ===============

CREATE TABLE customers (
    id                TEXT PRIMARY KEY,
    name              TEXT NOT NULL,
    identifier_enc    TEXT,                -- encrypted envelope (v1:...)
    street_enc        TEXT,                -- encrypted envelope
    city              TEXT,
    state             TEXT,
    zip               TEXT,
    phone             TEXT,
    email             TEXT,
    tags              TEXT[] NOT NULL DEFAULT '{}',
    -- Normalized search column populated on write; keeps identifier OUT
    -- of the ts_vector intentionally (it is encrypted and should not leak).
    search_tsv        TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(name,'') || ' ' ||
                              coalesce(city,'') || ' ' ||
                              coalesce(state,'') || ' ' ||
                              coalesce(zip,''))
    ) STORED,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_customers_tsv ON customers USING GIN (search_tsv);
CREATE INDEX idx_customers_zip ON customers (zip);

-- =============== Address book ===============

CREATE TABLE address_book (
    id          TEXT PRIMARY KEY,
    owner_id    TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    customer_id TEXT REFERENCES customers(id) ON DELETE SET NULL,
    label       TEXT NOT NULL,
    street_enc  TEXT,
    city        TEXT,
    state       TEXT,
    zip         TEXT,
    lat         DOUBLE PRECISION,
    lng         DOUBLE PRECISION,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_addressbook_owner ON address_book (owner_id);

-- =============== Service area polygons ===============

CREATE TABLE service_regions (
    id                TEXT PRIMARY KEY,
    polygon_json      JSONB NOT NULL,          -- [[lat,lng],...]
    base_fee_cents    INT NOT NULL DEFAULT 0,
    per_mile_cents    INT NOT NULL DEFAULT 0,
    priority          INT NOT NULL DEFAULT 0
);

-- =============== Orders & events ===============

CREATE TABLE orders (
    id              TEXT PRIMARY KEY,
    customer_id     TEXT REFERENCES customers(id),
    status          TEXT NOT NULL,
    priority        TEXT,
    total_cents     INT NOT NULL DEFAULT 0,
    tags            TEXT[] NOT NULL DEFAULT '{}',
    items_json      JSONB NOT NULL DEFAULT '[]',
    delivery_street_enc TEXT,
    delivery_city   TEXT,
    delivery_state  TEXT,
    delivery_zip    TEXT,
    placed_at       TIMESTAMPTZ NOT NULL,
    updated_at      TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_orders_status ON orders (status);
CREATE INDEX idx_orders_placed ON orders (placed_at DESC);
CREATE INDEX idx_orders_delivery_zip ON orders (delivery_zip);
CREATE INDEX idx_orders_delivery_city ON orders (delivery_city);

CREATE TABLE order_events (
    id        TEXT PRIMARY KEY,
    order_id  TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    at        TIMESTAMPTZ NOT NULL,
    from_st   TEXT,
    to_st     TEXT NOT NULL,
    actor_id  TEXT,
    reason    TEXT,
    note      TEXT
);
CREATE INDEX idx_order_events_order ON order_events (order_id, at);

CREATE TABLE order_exceptions (
    id          TEXT PRIMARY KEY,
    order_id    TEXT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    kind        TEXT NOT NULL,
    detected_at TIMESTAMPTZ NOT NULL,
    description TEXT,
    resolved_at TIMESTAMPTZ
);
CREATE UNIQUE INDEX idx_order_exceptions_unique ON order_exceptions (order_id, kind)
    WHERE resolved_at IS NULL;

-- =============== Samples & reference ranges ===============

CREATE TABLE samples (
    id           TEXT PRIMARY KEY,
    order_id     TEXT REFERENCES orders(id) ON DELETE SET NULL,
    customer_id  TEXT REFERENCES customers(id) ON DELETE SET NULL,
    status       TEXT NOT NULL,
    collected_at TIMESTAMPTZ NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL,
    test_codes   TEXT[] NOT NULL DEFAULT '{}', -- denormalized for fast filters
    notes        TEXT
);
CREATE INDEX idx_samples_status ON samples (status);

-- test_items is the normalized "test items" entity the prompt names
-- alongside samples. Each row represents one test requested on a
-- sample; instructions capture technician guidance (e.g., "repeat if
-- value > 200"). `samples.test_codes` is kept as a denormalized array
-- for fast tsvector/search filtering.
CREATE TABLE test_items (
    id           TEXT PRIMARY KEY,
    sample_id    TEXT NOT NULL REFERENCES samples(id) ON DELETE CASCADE,
    test_code    TEXT NOT NULL,
    instructions TEXT NOT NULL DEFAULT '',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_test_items_sample ON test_items (sample_id);
CREATE INDEX idx_test_items_code   ON test_items (test_code);

CREATE TABLE reference_ranges (
    id            TEXT PRIMARY KEY,
    test_code     TEXT NOT NULL,
    units         TEXT,
    low_normal    DOUBLE PRECISION,
    high_normal   DOUBLE PRECISION,
    low_critical  DOUBLE PRECISION,
    high_critical DOUBLE PRECISION,
    demographic   TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_refrange_code ON reference_ranges (test_code);

-- =============== Route table (preloaded road distances) ===============

CREATE TABLE route_distances (
    from_id TEXT NOT NULL,
    to_id   TEXT NOT NULL,
    miles   DOUBLE PRECISION NOT NULL,
    -- Keys are stored in normalized order so (A,B) and (B,A) collapse.
    CONSTRAINT route_distances_ordered CHECK (from_id <= to_id),
    PRIMARY KEY (from_id, to_id)
);

-- =============== Reports (versioned) ===============

CREATE TABLE reports (
    id                TEXT PRIMARY KEY,
    sample_id         TEXT NOT NULL REFERENCES samples(id) ON DELETE CASCADE,
    version           INT NOT NULL,
    status            TEXT NOT NULL CHECK (status IN ('draft','issued','superseded')),
    title             TEXT NOT NULL,
    narrative         TEXT NOT NULL DEFAULT '',
    measurements_json JSONB NOT NULL DEFAULT '[]',
    author_id         TEXT REFERENCES users(id),
    reason_note       TEXT,
    issued_at         TIMESTAMPTZ,
    superseded_by_id  TEXT REFERENCES reports(id),
    archived_at       TIMESTAMPTZ,
    archived_by       TEXT REFERENCES users(id),
    archive_note      TEXT,
    search_tsv        TSVECTOR GENERATED ALWAYS AS (
        to_tsvector('simple', coalesce(title,'') || ' ' || coalesce(narrative,''))
    ) STORED,
    UNIQUE (sample_id, version)
);
CREATE INDEX idx_reports_sample ON reports (sample_id, version DESC);
CREATE INDEX idx_reports_tsv ON reports USING GIN (search_tsv);
CREATE INDEX idx_reports_archived ON reports (archived_at) WHERE archived_at IS NOT NULL;

-- =============== Saved filters ===============

CREATE TABLE saved_filters (
    id         TEXT PRIMARY KEY,
    owner_id   TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    payload    JSONB NOT NULL,
    key        TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (owner_id, key)
);

-- =============== System settings (global key/value) ===============

-- Small key/value store for deployment-wide configuration that the UI
-- reads at runtime. Today it holds the service-area map background
-- image (as an origin-relative URL or `data:` URI) so dispatch
-- operators see a real raster under the polygon overlay, matching the
-- prompt's "offline map image of the service territory" language.
CREATE TABLE system_settings (
    key        TEXT PRIMARY KEY,
    value      TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- =============== Audit log (immutable) ===============

CREATE TABLE audit_log (
    id               TEXT PRIMARY KEY,
    at               TIMESTAMPTZ NOT NULL,        -- server wall time
    workstation_time TIMESTAMPTZ,                 -- client-asserted time (X-Workstation-Time)
    actor_id         TEXT,
    workstation      TEXT,
    entity           TEXT NOT NULL,
    entity_id        TEXT NOT NULL,
    action           TEXT NOT NULL,
    before_json      JSONB,
    after_json       JSONB,
    reason           TEXT
);
CREATE INDEX idx_audit_entity ON audit_log (entity, entity_id, at);

-- Enforce append-only: block UPDATE/DELETE on audit_log.
CREATE OR REPLACE FUNCTION audit_log_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log rows are immutable';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_update BEFORE UPDATE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();
CREATE TRIGGER audit_log_no_delete BEFORE DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_immutable();

-- =============== Default permission catalog & role grants ===============

INSERT INTO permissions (id, description) VALUES
    ('customers.read',       'View customer records'),
    ('customers.write',      'Create or edit customers'),
    ('orders.read',          'View orders'),
    ('orders.write',         'Create or transition orders'),
    ('orders.refund',        'Refund an order'),
    ('orders.export',        'Export orders to CSV (bounded filter)'),
    ('samples.read',         'View samples'),
    ('samples.write',        'Create or transition samples'),
    ('reports.read',         'View reports'),
    ('reports.write',        'Create and correct reports'),
    ('reports.archive',      'Archive reports'),
    ('dispatch.validate',    'Validate dispatch pins & quote fees'),
    ('dispatch.configure',   'Edit service regions and route table'),
    ('analytics.view',       'View operational analytics'),
    ('admin.users',          'Manage users and permissions'),
    ('admin.reference',      'Edit reference ranges'),
    ('admin.audit',          'View audit log'),
    ('admin.settings',       'Edit system settings (map image, etc.)');

-- Role grants encode the default policy. An administrator can add or
-- revoke individual rows without redeploying.
INSERT INTO role_permissions (role, permission_id) VALUES
    -- front_desk
    ('front_desk', 'customers.read'), ('front_desk', 'customers.write'),
    ('front_desk', 'orders.read'),    ('front_desk', 'orders.write'),
    -- lab_tech
    ('lab_tech', 'samples.read'),     ('lab_tech', 'samples.write'),
    ('lab_tech', 'reports.read'),     ('lab_tech', 'reports.write'),
    ('lab_tech', 'reports.archive'),
    -- dispatch
    ('dispatch', 'orders.read'),      ('dispatch', 'dispatch.validate'),
    ('dispatch', 'customers.read'),
    -- analyst
    ('analyst', 'customers.read'),    ('analyst', 'orders.read'),
    ('analyst', 'samples.read'),      ('analyst', 'reports.read'),
    ('analyst', 'analytics.view'),    ('analyst', 'orders.export'),
    -- admin (gets everything)
    ('admin', 'customers.read'),  ('admin', 'customers.write'),
    ('admin', 'orders.read'),     ('admin', 'orders.write'),      ('admin', 'orders.refund'),
    ('admin', 'orders.export'),
    ('admin', 'samples.read'),    ('admin', 'samples.write'),
    ('admin', 'reports.read'),    ('admin', 'reports.write'),     ('admin', 'reports.archive'),
    ('admin', 'dispatch.validate'), ('admin', 'dispatch.configure'),
    ('admin', 'analytics.view'),  ('admin', 'admin.users'),
    ('admin', 'admin.reference'), ('admin', 'admin.audit'),
    ('admin', 'admin.settings');
