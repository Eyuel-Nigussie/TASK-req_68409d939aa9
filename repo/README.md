# Unified Offline Operations Portal

A full-stack operations portal for a regional laboratory and fulfillment
business that must run **without any internet dependency**. Front-desk
staff register customers and create orders, lab technicians manage
samples and issue versioned reports with abnormal-result flagging,
dispatch coordinators validate delivery locations against an offline
service-area map, analysts build operational reports, and administrators
configure users, permissions, reference ranges, service regions, and the
route table. Every state change writes an immutable audit entry, and
sensitive fields (customer identifiers, street addresses) are encrypted
at rest with per-environment keys.

## Architecture & Tech Stack

* **Frontend:** React 18, TypeScript 5, Vite 5, React Router v6 (served
  in production by nginx; the SPA proxies `/api` to the backend)
* **Backend:** Go 1.22 with the Echo v4 HTTP framework, Argon2id
  password hashing, AES-256-GCM at-rest encryption, structured JSON
  request logging
* **Database:** PostgreSQL 14 with tsvector full-text indexes, JSONB
  polygons + measurements, and an append-only audit-log trigger
* **Containerization:** Docker & Docker Compose (required)

## Project Structure

```text
.
├── backend/                # Go + Echo source, Dockerfile, entrypoint
│   ├── cmd/                # server, seed, keygen entrypoints
│   ├── internal/           # api, auth, audit, crypto, geo, lab, order,
│   │                       # runtime, search, filter, store, httpx
│   ├── migrations/         # 0001_init.sql (schema + seeded permissions)
│   ├── Dockerfile          # multi-stage: test / build / runtime
│   └── docker-entrypoint.sh
├── frontend/               # React + TypeScript + Vite source
│   ├── src/                # api, components, hooks, pages, styles
│   ├── Dockerfile          # multi-stage: test / build / nginx
│   └── nginx.conf          # static + /api reverse proxy
├── docs/                   # design.md, apispec.md, questions.md
├── .env.example            # environment variable template
├── docker-compose.yml      # runtime stack (db + backend + frontend)
├── docker-compose.test.yml # test stack (db + backend-test + frontend-test)
├── start.sh                # one-command launcher
├── run_tests.sh            # standardized test runner
└── README.md
```

## Prerequisites

To ensure a consistent environment, this project is designed to run
entirely within containers. You must have the following installed:

* [Docker](https://docs.docker.com/get-docker/) 24+
* [Docker Compose](https://docs.docker.com/compose/install/) v2 (the
  `docker compose` plugin; the legacy `docker-compose` binary is also
  supported transparently)

No Go, Node.js, or PostgreSQL installation is required on the host —
every language toolchain and the database run inside containers.

## Running the Application

1. **Build and start the containers.**
   The included `start.sh` launches the whole stack, copies
   `.env.example` to `.env` on first run, applies the migration,
   seeds the demo users, and blocks until `/api/health` reports OK:

   ```bash
   chmod +x start.sh
   ./start.sh
   ```

   If you prefer the raw Docker Compose command, it is equivalent to:

   ```bash
   cp .env.example .env   # first run only
   docker compose up --build -d
   ```

2. **Access the app.**

   * Frontend:     `http://localhost:3000`
   * Backend API:  `http://localhost:3000/api` (via the SPA proxy) or
     `http://localhost:8080/api` (direct to the Go process)
   * Health probe: `http://localhost:8080/api/health`
   * API reference: see [`docs/apispec.md`](docs/apispec.md)

3. **Stop the application.**

   ```bash
   docker compose down -v
   ```

   `-v` removes the `pgdata` volume so the next `./start.sh` starts
   from a fresh database. Omit `-v` to preserve customer/order data
   between runs.

## Testing

Every unit, integration, and E2E test is executed via a single,
standardized shell script. The script spins up a disposable Postgres
container for the backend integration suite and tears everything down
when it finishes. No host-side dependencies beyond Docker are needed.

Make sure the script is executable, then run it:

```bash
chmod +x run_tests.sh
./run_tests.sh
```

The script runs:

* **Backend** — `go test -race -cover ./...` inside the
  `backend-test` container, with `INTEGRATION_DB` pointed at the
  disposable Postgres service so the Postgres-backed store is also
  exercised.
* **Frontend** — `npm test` (vitest) inside the `frontend-test`
  container.

The exit code is `0` if every suite passes, non-zero otherwise, so
the script integrates cleanly with CI/CD validators.

## Seeded Credentials

The database is pre-seeded with the following test users **only when
`SEED_DEMO_USERS=1` is set in `.env`** (the shipped default is `0` so
no published credentials are ever installed automatically). Flip the
flag to `1` for local evaluation to get the table below; use these
credentials to verify authentication and role-based access controls.
Passwords satisfy the ≥10-character policy enforced by the backend.

| Role           | Username    | Password           | Notes                                                                                  |
| :------------- | :---------- | :----------------- | :------------------------------------------------------------------------------------- |
| **Admin**      | `admin`     | `AdminTest123!`    | Full access: user + permission management, service regions, reference ranges, audit.   |
| **Front Desk** | `frontdesk` | `FrontDeskTest1!`  | Register customers, create and transition orders, manage the address book.             |
| **Lab Tech**   | `labtech`   | `LabTechTest123!`  | Submit samples, issue and correct versioned reports, archive reports.                  |
| **Dispatch**   | `dispatch`  | `DispatchTest1!`   | Validate delivery pins against service regions, quote fees via the route table.        |
| **Analyst**    | `analyst`   | `AnalystTest123!`  | Read-only operational analytics (status counts, time series, abnormal rate).           |

Keep `SEED_DEMO_USERS=0` for real deployments. When you do enable
demo seeding for evaluation, rotate each password before the portal
leaves the isolated network.
