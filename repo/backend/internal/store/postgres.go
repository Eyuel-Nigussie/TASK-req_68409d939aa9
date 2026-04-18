package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
	"github.com/lib/pq"
)

// Postgres is a PostgreSQL-backed Store. Callers supply a *sql.DB that
// has already been configured with connection pool limits. All methods
// accept a context.Context; query cancellation is honored.
type Postgres struct {
	db *sql.DB
}

// NewPostgres wraps db and returns a Store implementation. It does not
// create the schema; use `migrations/0001_init.sql` during deployment.
func NewPostgres(db *sql.DB) *Postgres { return &Postgres{db: db} }

// mapErr translates pq sentinel errors into package-level errors handlers
// know how to react to.
func mapErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

// ---------- Users ----------

func (p *Postgres) CreateUser(ctx context.Context, u models.User) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO users (id, username, role, password_hash, disabled, must_rotate_password, created_at, updated_at)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		u.ID, u.Username, string(u.Role), u.PasswordHash, u.Disabled, u.MustRotatePassword, u.CreatedAt, u.UpdatedAt)
	return mapErr(err)
}

func (p *Postgres) GetUserByUsername(ctx context.Context, name string) (models.User, error) {
	var u models.User
	var role string
	err := p.db.QueryRowContext(ctx, `
	    SELECT id, username, role, password_hash, disabled, must_rotate_password, created_at, updated_at
	    FROM users WHERE username = $1`, name).
		Scan(&u.ID, &u.Username, &role, &u.PasswordHash, &u.Disabled, &u.MustRotatePassword, &u.CreatedAt, &u.UpdatedAt)
	u.Role = models.Role(role)
	return u, mapErr(err)
}

func (p *Postgres) GetUserByID(ctx context.Context, id string) (models.User, error) {
	var u models.User
	var role string
	err := p.db.QueryRowContext(ctx, `
	    SELECT id, username, role, password_hash, disabled, must_rotate_password, created_at, updated_at
	    FROM users WHERE id = $1`, id).
		Scan(&u.ID, &u.Username, &role, &u.PasswordHash, &u.Disabled, &u.MustRotatePassword, &u.CreatedAt, &u.UpdatedAt)
	u.Role = models.Role(role)
	return u, mapErr(err)
}

func (p *Postgres) ListUsers(ctx context.Context) ([]models.User, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, username, role, password_hash, disabled, must_rotate_password, created_at, updated_at
	    FROM users ORDER BY username`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []models.User
	for rows.Next() {
		var u models.User
		var role string
		if err := rows.Scan(&u.ID, &u.Username, &role, &u.PasswordHash, &u.Disabled, &u.MustRotatePassword, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		u.Role = models.Role(role)
		out = append(out, u)
	}
	return out, rows.Err()
}

func (p *Postgres) UpdateUser(ctx context.Context, u models.User) error {
	res, err := p.db.ExecContext(ctx, `
	    UPDATE users SET role=$2, password_hash=$3, disabled=$4, must_rotate_password=$5, updated_at=$6
	    WHERE id=$1`,
		u.ID, string(u.Role), u.PasswordHash, u.Disabled, u.MustRotatePassword, u.UpdatedAt)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- Customers (encrypted identifier and street are passed in as envelopes) ----------

func (p *Postgres) CreateCustomer(ctx context.Context, c models.Customer) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO customers (id, name, identifier_enc, street_enc, city, state, zip, phone, email, tags, created_at, updated_at)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`,
		c.ID, c.Name, c.Identifier, c.Street, c.City, c.State, c.ZIP, c.Phone, c.Email, pq.Array(c.Tags), c.CreatedAt, c.UpdatedAt)
	return mapErr(err)
}

func (p *Postgres) GetCustomer(ctx context.Context, id string) (models.Customer, error) {
	var c models.Customer
	var tags pq.StringArray
	err := p.db.QueryRowContext(ctx, `
	    SELECT id, name, identifier_enc, street_enc, city, state, zip, phone, email, tags, created_at, updated_at
	    FROM customers WHERE id=$1`, id).
		Scan(&c.ID, &c.Name, &c.Identifier, &c.Street, &c.City, &c.State, &c.ZIP, &c.Phone, &c.Email, &tags, &c.CreatedAt, &c.UpdatedAt)
	c.Tags = []string(tags)
	return c, mapErr(err)
}

func (p *Postgres) UpdateCustomer(ctx context.Context, c models.Customer) error {
	res, err := p.db.ExecContext(ctx, `
	    UPDATE customers SET name=$2, identifier_enc=$3, street_enc=$4, city=$5, state=$6, zip=$7, phone=$8, email=$9, tags=$10, updated_at=$11
	    WHERE id=$1`,
		c.ID, c.Name, c.Identifier, c.Street, c.City, c.State, c.ZIP, c.Phone, c.Email, pq.Array(c.Tags), c.UpdatedAt)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SearchCustomers does a plainto_tsquery match then falls back to prefix
// matching so operators can still find customers when they type partial
// tokens ("Jan" for "Jane"). Identifier is not searched because it is
// encrypted at rest.
func (p *Postgres) SearchCustomers(ctx context.Context, query string, limit int) ([]models.Customer, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, name, identifier_enc, street_enc, city, state, zip, phone, email, tags, created_at, updated_at
	    FROM customers
	    WHERE search_tsv @@ plainto_tsquery('simple', $1)
	       OR name ILIKE $2
	    ORDER BY ts_rank(search_tsv, plainto_tsquery('simple', $1)) DESC, name
	    LIMIT $3`, query, query+"%", limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	return scanCustomers(rows)
}

// FindByAddress filters by the non-encrypted columns (city/ZIP). The
// `street` parameter is intentionally ignored at this layer because the
// column is stored as an encryption envelope and cannot be searched with
// SQL LIKE. Callers that need street-substring matching must pass the
// result set through the vault in the service layer.
func (p *Postgres) FindByAddress(ctx context.Context, _street, city, zip string) ([]models.Customer, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, name, identifier_enc, street_enc, city, state, zip, phone, email, tags, created_at, updated_at
	    FROM customers
	    WHERE ($1 = '' OR lower(zip) = lower($1))
	      AND ($2 = '' OR lower(city) = lower($2))
	    ORDER BY name
	    LIMIT 500`, zip, city)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	return scanCustomers(rows)
}

func scanCustomers(rows *sql.Rows) ([]models.Customer, error) {
	var out []models.Customer
	for rows.Next() {
		var c models.Customer
		var tags pq.StringArray
		if err := rows.Scan(&c.ID, &c.Name, &c.Identifier, &c.Street, &c.City, &c.State, &c.ZIP, &c.Phone, &c.Email, &tags, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Tags = []string(tags)
		out = append(out, c)
	}
	return out, rows.Err()
}

// ---------- Address book ----------

func (p *Postgres) CreateAddress(ctx context.Context, a models.AddressBookEntry) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO address_book (id, owner_id, customer_id, label, street_enc, city, state, zip, lat, lng, created_at)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		a.ID, a.OwnerID, nullStr(a.CustomerID), a.Label, a.Street, a.City, a.State, a.ZIP, a.Lat, a.Lng, a.CreatedAt)
	return mapErr(err)
}

func (p *Postgres) ListAddresses(ctx context.Context, ownerID string) ([]models.AddressBookEntry, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, owner_id, coalesce(customer_id,''), label, coalesce(street_enc,''), coalesce(city,''), coalesce(state,''), coalesce(zip,''), coalesce(lat,0), coalesce(lng,0), created_at
	    FROM address_book WHERE owner_id=$1 ORDER BY label`, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make([]models.AddressBookEntry, 0)
	for rows.Next() {
		var a models.AddressBookEntry
		if err := rows.Scan(&a.ID, &a.OwnerID, &a.CustomerID, &a.Label, &a.Street, &a.City, &a.State, &a.ZIP, &a.Lat, &a.Lng, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (p *Postgres) DeleteAddress(ctx context.Context, ownerID, id string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM address_book WHERE id=$1 AND owner_id=$2`, id, ownerID)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- Service areas ----------

func (p *Postgres) ListRegions(ctx context.Context) ([]geo.Region, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, polygon_json, base_fee_cents, per_mile_cents FROM service_regions ORDER BY priority DESC, id`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []geo.Region
	for rows.Next() {
		var r geo.Region
		var polyJSON []byte
		var id string
		if err := rows.Scan(&id, &polyJSON, &r.BaseFeeCents, &r.PerMileFeeCents); err != nil {
			return nil, err
		}
		r.Polygon.ID = id
		var pairs [][2]float64
		if err := json.Unmarshal(polyJSON, &pairs); err != nil {
			return nil, fmt.Errorf("polygon %s: %w", id, err)
		}
		r.Polygon.Vertices = make([]geo.Point, len(pairs))
		for i, pair := range pairs {
			r.Polygon.Vertices[i] = geo.Point{Lat: pair[0], Lng: pair[1]}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) ReplaceRegions(ctx context.Context, regions []geo.Region) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM service_regions`); err != nil {
		return err
	}
	for i, r := range regions {
		pairs := make([][2]float64, len(r.Polygon.Vertices))
		for j, v := range r.Polygon.Vertices {
			pairs[j] = [2]float64{v.Lat, v.Lng}
		}
		polyJSON, _ := json.Marshal(pairs)
		if _, err := tx.ExecContext(ctx, `
		    INSERT INTO service_regions (id, polygon_json, base_fee_cents, per_mile_cents, priority)
		    VALUES ($1,$2,$3,$4,$5)`,
			r.Polygon.ID, polyJSON, r.BaseFeeCents, r.PerMileFeeCents, len(regions)-i); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---------- Orders ----------

func (p *Postgres) CreateOrder(ctx context.Context, o order.Order) error {
	items, _ := json.Marshal(o.Items)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO orders (id, customer_id, status, priority, total_cents, tags, items_json,
	        delivery_street_enc, delivery_city, delivery_state, delivery_zip, placed_at, updated_at)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		o.ID, nullStr(o.CustomerID), string(o.Status), o.Priority, o.TotalCents, pq.Array(o.Tags), items,
		nullStr(o.DeliveryStreet), nullStr(o.DeliveryCity), nullStr(o.DeliveryState), nullStr(o.DeliveryZIP),
		o.PlacedAt, o.UpdatedAt)
	return mapErr(err)
}

func (p *Postgres) GetOrder(ctx context.Context, id string) (order.Order, error) {
	var o order.Order
	var status, customer string
	var tags pq.StringArray
	var items []byte
	err := p.db.QueryRowContext(ctx, `
	    SELECT id, coalesce(customer_id,''), status, coalesce(priority,''), total_cents, tags, items_json,
	        coalesce(delivery_street_enc,''), coalesce(delivery_city,''), coalesce(delivery_state,''), coalesce(delivery_zip,''),
	        placed_at, updated_at
	    FROM orders WHERE id=$1`, id).
		Scan(&o.ID, &customer, &status, &o.Priority, &o.TotalCents, &tags, &items,
			&o.DeliveryStreet, &o.DeliveryCity, &o.DeliveryState, &o.DeliveryZIP,
			&o.PlacedAt, &o.UpdatedAt)
	o.Status = order.Status(status)
	o.CustomerID = customer
	o.Tags = []string(tags)
	if len(items) > 0 {
		_ = json.Unmarshal(items, &o.Items)
	}
	if err != nil {
		return o, mapErr(err)
	}
	// Load events.
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, order_id, at, coalesce(from_st,''), to_st, coalesce(actor_id,''), coalesce(reason,''), coalesce(note,'')
	    FROM order_events WHERE order_id=$1 ORDER BY at`, id)
	if err != nil {
		return o, mapErr(err)
	}
	defer rows.Close()
	for rows.Next() {
		var ev order.Event
		var from, to string
		if err := rows.Scan(&ev.ID, &ev.OrderID, &ev.At, &from, &to, &ev.Actor, &ev.Reason, &ev.Note); err != nil {
			return o, err
		}
		ev.From = order.Status(from)
		ev.To = order.Status(to)
		o.Events = append(o.Events, ev)
	}
	return o, rows.Err()
}

func (p *Postgres) UpdateOrder(ctx context.Context, o order.Order) error {
	items, _ := json.Marshal(o.Items)
	res, err := p.db.ExecContext(ctx, `
	    UPDATE orders SET status=$2, priority=$3, total_cents=$4, tags=$5, items_json=$6,
	        delivery_street_enc=$7, delivery_city=$8, delivery_state=$9, delivery_zip=$10, updated_at=$11
	    WHERE id=$1`,
		o.ID, string(o.Status), o.Priority, o.TotalCents, pq.Array(o.Tags), items,
		nullStr(o.DeliveryStreet), nullStr(o.DeliveryCity), nullStr(o.DeliveryState), nullStr(o.DeliveryZIP),
		o.UpdatedAt)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) AppendOrderEvent(ctx context.Context, ev order.Event) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO order_events (id, order_id, at, from_st, to_st, actor_id, reason, note)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		ev.ID, ev.OrderID, ev.At, string(ev.From), string(ev.To), nullStr(ev.Actor), nullStr(ev.Reason), nullStr(ev.Note))
	return mapErr(err)
}

func (p *Postgres) ListOrders(ctx context.Context, statuses []string, from, to *int64, limit, offset int) ([]order.Order, error) {
	args := []any{}
	q := `SELECT id, coalesce(customer_id,''), status, coalesce(priority,''), total_cents, tags, placed_at, updated_at FROM orders WHERE 1=1`
	if len(statuses) > 0 {
		args = append(args, pq.Array(statuses))
		q += fmt.Sprintf(" AND status = ANY($%d)", len(args))
	}
	if from != nil {
		args = append(args, *from)
		q += fmt.Sprintf(" AND extract(epoch from placed_at)::bigint >= $%d", len(args))
	}
	if to != nil {
		args = append(args, *to)
		q += fmt.Sprintf(" AND extract(epoch from placed_at)::bigint <= $%d", len(args))
	}
	q += " ORDER BY placed_at DESC"
	if limit > 0 {
		args = append(args, limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if offset > 0 {
		args = append(args, offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make([]order.Order, 0)
	for rows.Next() {
		var o order.Order
		var status string
		var tags pq.StringArray
		if err := rows.Scan(&o.ID, &o.CustomerID, &status, &o.Priority, &o.TotalCents, &tags, &o.PlacedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Status = order.Status(status)
		o.Tags = []string(tags)
		out = append(out, o)
	}
	return out, rows.Err()
}

// QueryOrders executes the full filter payload in SQL. Sort fields are
// validated against a static allowlist before being interpolated so this
// cannot be used to ORDER BY an arbitrary column.
func (p *Postgres) QueryOrders(ctx context.Context, q OrderQuery) ([]order.Order, int, error) {
	allowedSort := map[string]string{
		"placed_at": "placed_at", "status": "status",
		"total_cents": "total_cents", "priority": "priority",
	}
	args := []any{}
	where := " WHERE 1=1"
	if len(q.Statuses) > 0 {
		args = append(args, pq.Array(q.Statuses))
		where += fmt.Sprintf(" AND status = ANY($%d)", len(args))
	}
	if q.Priority != "" {
		args = append(args, q.Priority)
		where += fmt.Sprintf(" AND priority = $%d", len(args))
	}
	if q.StartUnix != nil {
		args = append(args, *q.StartUnix)
		where += fmt.Sprintf(" AND extract(epoch from placed_at)::bigint >= $%d", len(args))
	}
	if q.EndUnix != nil {
		args = append(args, *q.EndUnix)
		where += fmt.Sprintf(" AND extract(epoch from placed_at)::bigint <= $%d", len(args))
	}
	if q.MinCents != nil {
		args = append(args, *q.MinCents)
		where += fmt.Sprintf(" AND total_cents >= $%d", len(args))
	}
	if q.MaxCents != nil {
		args = append(args, *q.MaxCents)
		where += fmt.Sprintf(" AND total_cents <= $%d", len(args))
	}
	if len(q.Tags) > 0 {
		args = append(args, pq.Array(q.Tags))
		where += fmt.Sprintf(" AND tags && $%d", len(args))
	}
	if kw := q.Keyword; kw != "" {
		args = append(args, "%"+kw+"%")
		where += fmt.Sprintf(" AND (id ILIKE $%d OR status ILIKE $%[1]d OR coalesce(priority,'') ILIKE $%[1]d OR coalesce(customer_id,'') ILIKE $%[1]d)", len(args))
	}
	// Count.
	var total int
	if err := p.db.QueryRowContext(ctx, `SELECT count(*) FROM orders`+where, args...).Scan(&total); err != nil {
		return nil, 0, mapErr(err)
	}
	// Order by + page.
	sortCol := "placed_at"
	if c, ok := allowedSort[q.SortBy]; ok {
		sortCol = c
	}
	dir := "DESC"
	if !q.SortDesc && q.SortBy != "" {
		dir = "ASC"
	}
	limit := q.Limit
	if limit <= 0 {
		limit = 25
	}
	args = append(args, limit, q.Offset)
	sqlStr := fmt.Sprintf(`SELECT id, coalesce(customer_id,''), status, coalesce(priority,''), total_cents, tags, placed_at, updated_at FROM orders%s ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		where, sortCol, dir, len(args)-1, len(args))
	rows, err := p.db.QueryContext(ctx, sqlStr, args...)
	if err != nil {
		return nil, 0, mapErr(err)
	}
	defer rows.Close()
	out := make([]order.Order, 0)
	for rows.Next() {
		var o order.Order
		var status string
		var tags pq.StringArray
		if err := rows.Scan(&o.ID, &o.CustomerID, &status, &o.Priority, &o.TotalCents, &tags, &o.PlacedAt, &o.UpdatedAt); err != nil {
			return nil, 0, err
		}
		o.Status = order.Status(status)
		o.Tags = []string(tags)
		out = append(out, o)
	}
	return out, total, rows.Err()
}

// OrdersByAddress filters on the delivery-city and delivery-ZIP columns.
// Street is encrypted at rest and must be filtered post-decryption by the
// handler.
func (p *Postgres) OrdersByAddress(ctx context.Context, city, zip string) ([]order.Order, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, coalesce(customer_id,''), status, coalesce(priority,''), total_cents, tags, items_json,
	        coalesce(delivery_street_enc,''), coalesce(delivery_city,''), coalesce(delivery_state,''), coalesce(delivery_zip,''),
	        placed_at, updated_at
	    FROM orders
	    WHERE ($1 = '' OR lower(delivery_zip) = lower($1))
	      AND ($2 = '' OR lower(delivery_city) = lower($2))
	    ORDER BY placed_at DESC
	    LIMIT 500`, zip, city)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make([]order.Order, 0)
	for rows.Next() {
		var o order.Order
		var status string
		var tags pq.StringArray
		var items []byte
		if err := rows.Scan(&o.ID, &o.CustomerID, &status, &o.Priority, &o.TotalCents, &tags, &items,
			&o.DeliveryStreet, &o.DeliveryCity, &o.DeliveryState, &o.DeliveryZIP,
			&o.PlacedAt, &o.UpdatedAt); err != nil {
			return nil, err
		}
		o.Status = order.Status(status)
		o.Tags = []string(tags)
		if len(items) > 0 {
			_ = json.Unmarshal(items, &o.Items)
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (p *Postgres) ListExceptions(ctx context.Context) ([]order.Exception, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT order_id, kind, detected_at, coalesce(description,'')
	    FROM order_exceptions WHERE resolved_at IS NULL ORDER BY detected_at`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make([]order.Exception, 0)
	for rows.Next() {
		var ex order.Exception
		if err := rows.Scan(&ex.OrderID, &ex.Kind, &ex.DetectedAt, &ex.Description); err != nil {
			return nil, err
		}
		out = append(out, ex)
	}
	return out, rows.Err()
}

func (p *Postgres) PutException(ctx context.Context, ex order.Exception) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO order_exceptions (id, order_id, kind, detected_at, description)
	    VALUES ($1,$2,$3,$4,$5)
	    ON CONFLICT (order_id, kind) WHERE resolved_at IS NULL DO NOTHING`,
		ex.OrderID+"|"+ex.Kind, ex.OrderID, ex.Kind, ex.DetectedAt, ex.Description)
	return mapErr(err)
}

// ---------- Samples ----------

func (p *Postgres) CreateSample(ctx context.Context, s lab.Sample) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO samples (id, order_id, customer_id, status, collected_at, updated_at, test_codes, notes)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		s.ID, nullStr(s.OrderID), nullStr(s.CustomerID), string(s.Status), s.CollectedAt, s.UpdatedAt, pq.Array(s.TestCodes), s.Notes)
	return mapErr(err)
}

func (p *Postgres) GetSample(ctx context.Context, id string) (lab.Sample, error) {
	var s lab.Sample
	var status string
	var codes pq.StringArray
	err := p.db.QueryRowContext(ctx, `
	    SELECT id, coalesce(order_id,''), coalesce(customer_id,''), status, collected_at, updated_at, test_codes, coalesce(notes,'')
	    FROM samples WHERE id=$1`, id).
		Scan(&s.ID, &s.OrderID, &s.CustomerID, &status, &s.CollectedAt, &s.UpdatedAt, &codes, &s.Notes)
	s.Status = lab.SampleStatus(status)
	s.TestCodes = []string(codes)
	return s, mapErr(err)
}

func (p *Postgres) UpdateSample(ctx context.Context, s lab.Sample) error {
	res, err := p.db.ExecContext(ctx, `
	    UPDATE samples SET status=$2, updated_at=$3, test_codes=$4, notes=$5
	    WHERE id=$1`,
		s.ID, string(s.Status), s.UpdatedAt, pq.Array(s.TestCodes), s.Notes)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) ListSamples(ctx context.Context, statuses []string, limit, offset int) ([]lab.Sample, error) {
	args := []any{}
	q := `SELECT id, coalesce(order_id,''), coalesce(customer_id,''), status, collected_at, updated_at, test_codes, coalesce(notes,'') FROM samples WHERE 1=1`
	if len(statuses) > 0 {
		args = append(args, pq.Array(statuses))
		q += fmt.Sprintf(" AND status = ANY($%d)", len(args))
	}
	q += " ORDER BY collected_at DESC"
	if limit > 0 {
		args = append(args, limit)
		q += fmt.Sprintf(" LIMIT $%d", len(args))
	}
	if offset > 0 {
		args = append(args, offset)
		q += fmt.Sprintf(" OFFSET $%d", len(args))
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make([]lab.Sample, 0)
	for rows.Next() {
		var s lab.Sample
		var status string
		var codes pq.StringArray
		if err := rows.Scan(&s.ID, &s.OrderID, &s.CustomerID, &status, &s.CollectedAt, &s.UpdatedAt, &codes, &s.Notes); err != nil {
			return nil, err
		}
		s.Status = lab.SampleStatus(status)
		s.TestCodes = []string(codes)
		out = append(out, s)
	}
	return out, rows.Err()
}

// ---------- Reports ----------

func (p *Postgres) CreateReport(ctx context.Context, r lab.Report) error {
	msJSON, _ := json.Marshal(r.Measurements)
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO reports (id, sample_id, version, status, title, narrative, measurements_json, author_id, reason_note, issued_at, superseded_by_id, archived_at, archived_by, archive_note)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		r.ID, r.SampleID, r.Version, string(r.Status), r.Title, r.Narrative, msJSON,
		nullStr(r.AuthorID), nullStr(r.ReasonNote), nullTime(r.IssuedAt), nullStr(r.SupersededByID),
		nullTime(r.ArchivedAt), nullStr(r.ArchivedBy), nullStr(r.ArchiveNote))
	return mapErr(err)
}

func (p *Postgres) GetReport(ctx context.Context, id string) (lab.Report, error) {
	r, err := scanSingleReport(p.db.QueryRowContext(ctx, reportSelect+" WHERE id=$1", id))
	return r, mapErr(err)
}

func (p *Postgres) LatestReportForSample(ctx context.Context, sampleID string) (lab.Report, error) {
	r, err := scanSingleReport(p.db.QueryRowContext(ctx, reportSelect+" WHERE sample_id=$1 ORDER BY version DESC LIMIT 1", sampleID))
	return r, mapErr(err)
}

func (p *Postgres) UpdateReport(ctx context.Context, r lab.Report) error {
	msJSON, _ := json.Marshal(r.Measurements)
	res, err := p.db.ExecContext(ctx, `
	    UPDATE reports SET status=$2, title=$3, narrative=$4, measurements_json=$5, reason_note=$6,
	        issued_at=$7, superseded_by_id=$8, archived_at=$9, archived_by=$10, archive_note=$11
	    WHERE id=$1`,
		r.ID, string(r.Status), r.Title, r.Narrative, msJSON,
		nullStr(r.ReasonNote), nullTime(r.IssuedAt), nullStr(r.SupersededByID),
		nullTime(r.ArchivedAt), nullStr(r.ArchivedBy), nullStr(r.ArchiveNote))
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

func (p *Postgres) ReplaceWithCorrection(ctx context.Context, old, next lab.Report) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// Insert the new version FIRST so the old row's superseded_by_id FK
	// target exists when we update it. Archive columns default to NULL.
	nextJSON, _ := json.Marshal(next.Measurements)
	if _, err := tx.ExecContext(ctx, `
	    INSERT INTO reports (id, sample_id, version, status, title, narrative, measurements_json, author_id, reason_note, issued_at)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		next.ID, next.SampleID, next.Version, string(next.Status), next.Title, next.Narrative, nextJSON, nullStr(next.AuthorID), next.ReasonNote, nullTime(next.IssuedAt)); err != nil {
		return mapErr(err)
	}
	oldJSON, _ := json.Marshal(old.Measurements)
	if _, err := tx.ExecContext(ctx, `
	    UPDATE reports SET status=$2, measurements_json=$3, superseded_by_id=$4
	    WHERE id=$1`, old.ID, string(old.Status), oldJSON, nullStr(old.SupersededByID)); err != nil {
		return err
	}
	return tx.Commit()
}

func (p *Postgres) SearchReports(ctx context.Context, query string, limit int) ([]lab.Report, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := p.db.QueryContext(ctx, reportSelect+`
	    WHERE search_tsv @@ plainto_tsquery('simple', $1)
	    ORDER BY ts_rank(search_tsv, plainto_tsquery('simple', $1)) DESC, issued_at DESC NULLS LAST
	    LIMIT $2`, query, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	return scanReports(rows)
}

func (p *Postgres) ListReports(ctx context.Context, limit, offset int) ([]lab.Report, error) {
	if limit <= 0 {
		limit = 50
	}
	// Default list excludes archived reports. Retrieval of archived rows is
	// done via SearchReports or the dedicated archive listing endpoint.
	rows, err := p.db.QueryContext(ctx, reportSelect+" WHERE archived_at IS NULL ORDER BY issued_at DESC NULLS LAST LIMIT $1 OFFSET $2", limit, offset)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	return scanReports(rows)
}

const reportSelect = `SELECT id, sample_id, version, status, title, narrative, measurements_json,
	coalesce(author_id,''), coalesce(reason_note,''), issued_at, coalesce(superseded_by_id,''),
	archived_at, coalesce(archived_by,''), coalesce(archive_note,'') FROM reports`

func scanSingleReport(row *sql.Row) (lab.Report, error) {
	var r lab.Report
	var status string
	var ms []byte
	var issued, archived sql.NullTime
	if err := row.Scan(&r.ID, &r.SampleID, &r.Version, &status, &r.Title, &r.Narrative, &ms,
		&r.AuthorID, &r.ReasonNote, &issued, &r.SupersededByID,
		&archived, &r.ArchivedBy, &r.ArchiveNote); err != nil {
		return r, err
	}
	r.Status = lab.ReportStatus(status)
	if len(ms) > 0 {
		_ = json.Unmarshal(ms, &r.Measurements)
	}
	if issued.Valid {
		r.IssuedAt = issued.Time
	}
	if archived.Valid {
		r.ArchivedAt = archived.Time
	}
	return r, nil
}

func scanReports(rows *sql.Rows) ([]lab.Report, error) {
	out := make([]lab.Report, 0)
	for rows.Next() {
		var r lab.Report
		var status string
		var ms []byte
		var issued, archived sql.NullTime
		if err := rows.Scan(&r.ID, &r.SampleID, &r.Version, &status, &r.Title, &r.Narrative, &ms,
			&r.AuthorID, &r.ReasonNote, &issued, &r.SupersededByID,
			&archived, &r.ArchivedBy, &r.ArchiveNote); err != nil {
			return nil, err
		}
		r.Status = lab.ReportStatus(status)
		if len(ms) > 0 {
			_ = json.Unmarshal(ms, &r.Measurements)
		}
		if issued.Valid {
			r.IssuedAt = issued.Time
		}
		if archived.Valid {
			r.ArchivedAt = archived.Time
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ListArchivedReports returns the archived portion of the report set,
// sorted by archive time.
func (p *Postgres) ListArchivedReports(ctx context.Context) ([]lab.Report, error) {
	rows, err := p.db.QueryContext(ctx, reportSelect+" WHERE archived_at IS NOT NULL ORDER BY archived_at DESC LIMIT 500")
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	return scanReports(rows)
}

// ---------- Saved filters ----------

func (p *Postgres) CreateSavedFilter(ctx context.Context, f models.SavedFilter) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO saved_filters (id, owner_id, name, payload, key, created_at)
	    VALUES ($1,$2,$3,$4,$5,$6)`,
		f.ID, f.OwnerID, f.Name, f.Payload, f.Key, f.CreatedAt)
	return mapErr(err)
}

func (p *Postgres) ListSavedFilters(ctx context.Context, ownerID string) ([]models.SavedFilter, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, owner_id, name, payload, key, created_at
	    FROM saved_filters WHERE owner_id=$1 ORDER BY name`, ownerID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := make([]models.SavedFilter, 0)
	for rows.Next() {
		var f models.SavedFilter
		if err := rows.Scan(&f.ID, &f.OwnerID, &f.Name, &f.Payload, &f.Key, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (p *Postgres) DeleteSavedFilter(ctx context.Context, ownerID, id string) error {
	res, err := p.db.ExecContext(ctx, `DELETE FROM saved_filters WHERE id=$1 AND owner_id=$2`, id, ownerID)
	if err != nil {
		return mapErr(err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------- Reference ranges ----------

func (p *Postgres) ListRefRanges(ctx context.Context) ([]lab.RefRange, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, test_code, coalesce(units,''), low_normal, high_normal, low_critical, high_critical, coalesce(demographic,'')
	    FROM reference_ranges ORDER BY test_code, demographic`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []lab.RefRange
	for rows.Next() {
		var r lab.RefRange
		var id string
		var lowN, highN, lowC, highC sql.NullFloat64
		if err := rows.Scan(&id, &r.TestCode, &r.Units, &lowN, &highN, &lowC, &highC, &r.Demographic); err != nil {
			return nil, err
		}
		if lowN.Valid {
			v := lowN.Float64
			r.LowNormal = &v
		}
		if highN.Valid {
			v := highN.Float64
			r.HighNormal = &v
		}
		if lowC.Valid {
			v := lowC.Float64
			r.LowCritical = &v
		}
		if highC.Valid {
			v := highC.Float64
			r.HighCritical = &v
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) ReplaceRefRanges(ctx context.Context, rr []lab.RefRange) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM reference_ranges`); err != nil {
		return err
	}
	for i, r := range rr {
		_, err := tx.ExecContext(ctx, `
		    INSERT INTO reference_ranges (id, test_code, units, low_normal, high_normal, low_critical, high_critical, demographic)
		    VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			fmt.Sprintf("rr_%d_%s_%s", i, r.TestCode, r.Demographic),
			r.TestCode, r.Units,
			nullFloat(r.LowNormal), nullFloat(r.HighNormal),
			nullFloat(r.LowCritical), nullFloat(r.HighCritical),
			r.Demographic,
		)
		if err != nil {
			return mapErr(err)
		}
	}
	return tx.Commit()
}

func nullFloat(p *float64) any {
	if p == nil {
		return nil
	}
	return *p
}

// ---------- Route table ----------

func (p *Postgres) ListRoutes(ctx context.Context) ([]RouteRow, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT from_id, to_id, miles FROM route_distances`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []RouteRow
	for rows.Next() {
		var r RouteRow
		if err := rows.Scan(&r.FromID, &r.ToID, &r.Miles); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) ReplaceRoutes(ctx context.Context, rows []RouteRow) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM route_distances`); err != nil {
		return err
	}
	for _, r := range rows {
		a, b := r.FromID, r.ToID
		if a > b {
			a, b = b, a
		}
		if _, err := tx.ExecContext(ctx, `
		    INSERT INTO route_distances (from_id, to_id, miles) VALUES ($1,$2,$3)
		    ON CONFLICT (from_id, to_id) DO UPDATE SET miles = EXCLUDED.miles`,
			a, b, r.Miles); err != nil {
			return mapErr(err)
		}
	}
	return tx.Commit()
}

// ---------- TestItems ----------

func (p *Postgres) ListTestItems(ctx context.Context, sampleID string) ([]models.TestItem, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, sample_id, test_code, coalesce(instructions,''), created_at
	    FROM test_items WHERE sample_id=$1 ORDER BY created_at, id`, sampleID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []models.TestItem
	for rows.Next() {
		var t models.TestItem
		if err := rows.Scan(&t.ID, &t.SampleID, &t.TestCode, &t.Instructions, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (p *Postgres) ReplaceTestItems(ctx context.Context, sampleID string, items []models.TestItem) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM test_items WHERE sample_id=$1`, sampleID); err != nil {
		return mapErr(err)
	}
	for _, t := range items {
		if _, err := tx.ExecContext(ctx, `
		    INSERT INTO test_items (id, sample_id, test_code, instructions, created_at)
		    VALUES ($1,$2,$3,$4,$5)`,
			t.ID, sampleID, t.TestCode, t.Instructions, t.CreatedAt); err != nil {
			return mapErr(err)
		}
	}
	return tx.Commit()
}

// ---------- SystemSettings ----------

func (p *Postgres) GetSetting(ctx context.Context, key string) (string, error) {
	var v string
	err := p.db.QueryRowContext(ctx, `SELECT value FROM system_settings WHERE key=$1`, key).Scan(&v)
	return v, mapErr(err)
}

func (p *Postgres) PutSetting(ctx context.Context, key, value string) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO system_settings (key, value, updated_at) VALUES ($1,$2,NOW())
	    ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = NOW()`,
		key, value)
	return mapErr(err)
}

func (p *Postgres) ListSettings(ctx context.Context) (map[string]string, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT key, value FROM system_settings`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// ---------- Permissions ----------

func (p *Postgres) ListPermissions(ctx context.Context) ([]models.Permission, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT id, description FROM permissions ORDER BY id`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []models.Permission
	for rows.Next() {
		var m models.Permission
		if err := rows.Scan(&m.ID, &m.Description); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (p *Postgres) UpsertPermission(ctx context.Context, perm models.Permission) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO permissions (id, description) VALUES ($1,$2)
	    ON CONFLICT (id) DO UPDATE SET description = EXCLUDED.description`,
		perm.ID, perm.Description)
	return mapErr(err)
}

func (p *Postgres) ListRolePermissions(ctx context.Context) ([]models.RolePermission, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT role, permission_id FROM role_permissions ORDER BY role, permission_id`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []models.RolePermission
	for rows.Next() {
		var rp models.RolePermission
		if err := rows.Scan(&rp.Role, &rp.PermissionID); err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	return out, rows.Err()
}

func (p *Postgres) SetRolePermissions(ctx context.Context, role string, ids []string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM role_permissions WHERE role=$1`, role); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO role_permissions (role, permission_id) VALUES ($1,$2)`, role, id); err != nil {
			return mapErr(err)
		}
	}
	return tx.Commit()
}

func (p *Postgres) GrantsForUser(ctx context.Context, userID, role string) ([]string, error) {
	rows, err := p.db.QueryContext(ctx, `
	    SELECT permission_id FROM role_permissions WHERE role = $1
	    UNION
	    SELECT permission_id FROM user_permissions WHERE user_id = $2
	    ORDER BY 1`, role, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (p *Postgres) ListUserPermissions(ctx context.Context, userID string) ([]string, error) {
	rows, err := p.db.QueryContext(ctx,
		`SELECT permission_id FROM user_permissions WHERE user_id=$1 ORDER BY permission_id`, userID)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (p *Postgres) SetUserPermissions(ctx context.Context, userID string, ids []string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_permissions WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_permissions (user_id, permission_id) VALUES ($1,$2)`, userID, id); err != nil {
			return mapErr(err)
		}
	}
	return tx.Commit()
}

// ---------- Login attempts ----------

func (p *Postgres) GetLoginAttempt(ctx context.Context, username string) (models.LoginAttempt, error) {
	var a models.LoginAttempt
	var locked sql.NullTime
	err := p.db.QueryRowContext(ctx, `
	    SELECT username, failures, locked_until, updated_at FROM login_attempts WHERE username=$1`,
		username).Scan(&a.Username, &a.Failures, &locked, &a.UpdatedAt)
	if locked.Valid {
		a.LockedUntil = locked.Time
	}
	return a, mapErr(err)
}

func (p *Postgres) UpsertLoginAttempt(ctx context.Context, a models.LoginAttempt) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO login_attempts (username, failures, locked_until, updated_at)
	    VALUES ($1,$2,$3,$4)
	    ON CONFLICT (username) DO UPDATE
	      SET failures = EXCLUDED.failures,
	          locked_until = EXCLUDED.locked_until,
	          updated_at = EXCLUDED.updated_at`,
		a.Username, a.Failures, nullTime(a.LockedUntil), a.UpdatedAt)
	return mapErr(err)
}

func (p *Postgres) ClearLoginAttempt(ctx context.Context, username string) error {
	_, err := p.db.ExecContext(ctx, `DELETE FROM login_attempts WHERE username=$1`, username)
	return mapErr(err)
}

// ---------- Analytics ----------

func (p *Postgres) OrderStatusCounts(ctx context.Context, fromUnix, toUnix int64) (map[string]int, error) {
	q := `SELECT status, count(*) FROM orders WHERE 1=1`
	args := []any{}
	if fromUnix > 0 {
		args = append(args, fromUnix)
		q += fmt.Sprintf(" AND extract(epoch FROM placed_at)::bigint >= $%d", len(args))
	}
	if toUnix > 0 {
		args = append(args, toUnix)
		q += fmt.Sprintf(" AND extract(epoch FROM placed_at)::bigint <= $%d", len(args))
	}
	q += " GROUP BY status"
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

func (p *Postgres) OrdersPerDay(ctx context.Context, fromUnix, toUnix int64) ([]AnalyticsDayCount, error) {
	q := `SELECT to_char(placed_at AT TIME ZONE 'UTC', 'YYYY-MM-DD') AS day, count(*) FROM orders WHERE 1=1`
	args := []any{}
	if fromUnix > 0 {
		args = append(args, fromUnix)
		q += fmt.Sprintf(" AND extract(epoch FROM placed_at)::bigint >= $%d", len(args))
	}
	if toUnix > 0 {
		args = append(args, toUnix)
		q += fmt.Sprintf(" AND extract(epoch FROM placed_at)::bigint <= $%d", len(args))
	}
	q += " GROUP BY day ORDER BY day"
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []AnalyticsDayCount
	for rows.Next() {
		var r AnalyticsDayCount
		if err := rows.Scan(&r.Day, &r.Count); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (p *Postgres) SampleStatusCounts(ctx context.Context, fromUnix, toUnix int64) (map[string]int, error) {
	q := `SELECT status, count(*) FROM samples WHERE 1=1`
	args := []any{}
	if fromUnix > 0 {
		args = append(args, fromUnix)
		q += fmt.Sprintf(" AND extract(epoch FROM collected_at)::bigint >= $%d", len(args))
	}
	if toUnix > 0 {
		args = append(args, toUnix)
		q += fmt.Sprintf(" AND extract(epoch FROM collected_at)::bigint <= $%d", len(args))
	}
	q += " GROUP BY status"
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		out[st] = n
	}
	return out, rows.Err()
}

func (p *Postgres) AbnormalReportRate(ctx context.Context, fromUnix, toUnix int64) (AnalyticsAbnormalRate, error) {
	// Abnormal rate crosses JSON so we compute it in application code by
	// loading the measurements JSON for issued reports in window.
	q := `SELECT measurements_json FROM reports WHERE status IN ('issued','superseded')`
	args := []any{}
	if fromUnix > 0 {
		args = append(args, fromUnix)
		q += fmt.Sprintf(" AND issued_at >= to_timestamp($%d)", len(args))
	}
	if toUnix > 0 {
		args = append(args, toUnix)
		q += fmt.Sprintf(" AND issued_at <= to_timestamp($%d)", len(args))
	}
	rows, err := p.db.QueryContext(ctx, q, args...)
	if err != nil {
		return AnalyticsAbnormalRate{}, mapErr(err)
	}
	defer rows.Close()
	var total, abnormal int
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return AnalyticsAbnormalRate{}, err
		}
		var ms []lab.Measurement
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &ms)
		}
		for _, m := range ms {
			total++
			if m.Flag.IsAbnormal() {
				abnormal++
			}
		}
	}
	var rate float64
	if total > 0 {
		rate = float64(abnormal) / float64(total)
	}
	return AnalyticsAbnormalRate{TotalMeasurements: total, AbnormalMeasurements: abnormal, Rate: rate}, rows.Err()
}

func (p *Postgres) ExceptionCountsByKind(ctx context.Context) (map[string]int, error) {
	rows, err := p.db.QueryContext(ctx, `SELECT kind, count(*) FROM order_exceptions WHERE resolved_at IS NULL GROUP BY kind`)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var k string
		var n int
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

// ---------- Audit ----------

func (p *Postgres) AppendAudit(ctx context.Context, e models.AuditEntry) error {
	_, err := p.db.ExecContext(ctx, `
	    INSERT INTO audit_log (id, at, workstation_time, actor_id, workstation, entity, entity_id, action, before_json, after_json, reason)
	    VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		e.ID, e.At, nullTime(e.WorkstationTime), nullStr(e.ActorID), nullStr(e.Workstation),
		e.Entity, e.EntityID, e.Action, nullBytes(e.Before), nullBytes(e.After), nullStr(e.Reason))
	return mapErr(err)
}

func (p *Postgres) ListAudit(ctx context.Context, entity, entityID string, limit int) ([]models.AuditEntry, error) {
	if limit <= 0 {
		limit = 500
	}
	rows, err := p.db.QueryContext(ctx, `
	    SELECT id, at, workstation_time, coalesce(actor_id,''), coalesce(workstation,''), entity, entity_id, action, before_json, after_json, coalesce(reason,'')
	    FROM audit_log WHERE ($1 = '' OR entity=$1) AND ($2 = '' OR entity_id=$2)
	    ORDER BY at LIMIT $3`, entity, entityID, limit)
	if err != nil {
		return nil, mapErr(err)
	}
	defer rows.Close()
	var out []models.AuditEntry
	for rows.Next() {
		var e models.AuditEntry
		var before, after sql.NullString
		var wsTime sql.NullTime
		if err := rows.Scan(&e.ID, &e.At, &wsTime, &e.ActorID, &e.Workstation, &e.Entity, &e.EntityID, &e.Action, &before, &after, &e.Reason); err != nil {
			return nil, err
		}
		if wsTime.Valid {
			e.WorkstationTime = wsTime.Time
		}
		if before.Valid {
			e.Before = []byte(before.String)
		}
		if after.Valid {
			e.After = []byte(after.String)
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ---------- helpers ----------

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullTime(t interface{ IsZero() bool }) any {
	if t.IsZero() {
		return nil
	}
	return t
}

func nullBytes(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	return b
}
