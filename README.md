# Full-stack portfolio

Go + Gin, Next.js App Router + React + TypeScript, Tailwind CSS v4, shadcn/ui Button, PostgreSQL, and Redis. This is local source; no cloud resources are created.

## Run the entire stack

1. Install/start Docker Desktop with Linux containers.
2. Copy .env.example to .env and replace the passwords and admin email. Admin password must be 16–72 bytes. Use URL-safe characters in the PostgreSQL password.
3. Run: docker compose up --build
4. Open http://localhost:3000 and sign in at http://localhost:3000/admin to add projects.

PostgreSQL persists in a named volume. No example projects or artificial metrics are seeded. The schema initializes at startup; add versioned migrations before evolving production tables.

## Local development

Node 20.9+ (22 recommended), Go 1.26+, and Docker Desktop. PostgreSQL and Redis CLIs are optional: containers supply the services.

Run docker compose up -d db redis from the root after preparing .env.
In frontend run npm ci and npm run dev.
In another PowerShell terminal, enter backend and set these variables explicitly; Go does not automatically load .env:

    $env:DATABASE_URL = 'postgres://portfolio:YOUR_PASSWORD@localhost:5432/portfolio?sslmode=disable'
    $env:REDIS_URL = 'redis://localhost:6379/0'
    $env:ADMIN_EMAIL = 'you@example.com'
    $env:ADMIN_PASSWORD = 'YOUR_UNIQUE_PASSWORD_AT_LEAST_16_CHARACTERS'
    $env:PUBLIC_ORIGIN = 'http://localhost:3000'
    go mod tidy
    go run .

Frontend API_URL defaults to localhost:8080. Copy frontend/.env.example to frontend/.env.local to override it.

## Architecture

Browser → Next.js same-origin /api proxy → Go/Gin → PostgreSQL
                                                → Redis cache + rate limits

| Endpoint | Behavior |
| --- | --- |
| GET /api/health | DB/Redis health, DB ping latency, process uptime, allocated Go heap, request/cache counters |
| GET /api/projects | Redis-cached PostgreSQL projects, X-Cache header |
| POST /api/contact | Saves validated contact message; five attempts/hour |
| POST /api/auth/login | Admin login; ten attempts/15 minutes |
| POST /api/auth/logout | Revokes session |
| GET /api/auth/me | Checks session |
| POST /api/admin/projects | Adds project |
| PUT /api/admin/projects/:id | Updates project |
| DELETE /api/admin/projects/:id | Deletes project |

Admin credentials come from environment variables; password is bcrypt-hashed on startup. Random session tokens are stored as SHA-256 hashes in PostgreSQL, expire after eight hours, and use HttpOnly, SameSite=Strict cookies. Mutations validate the exact Origin. The admin page is omitted from navigation, but security comes from API authorization.

Rate limits use atomic Redis counters with expiry and fail closed during Redis outages. Next proxies all requests, so the conservative default quota is shared across visitors by proxy IP. For per-visitor limits, configure a trusted ingress that strips and sets client-IP headers and explicitly configure Gin trusted proxies. Do not trust arbitrary forwarded headers.

Project cache TTL is 60 seconds; mutations rotate a generation to prevent stale concurrent fills. During failed invalidation, stale entries expire within 60 seconds. Project reads fall back to PostgreSQL if Redis is unavailable. Uptime and counters reset on API restart and are instance-local. Uptime is process age, not historical availability; memory is allocated Go heap, not total host memory. Health returns 503 if either dependency is unhealthy. Contacts are stored privately; email delivery and an inbox UI are not included.

## Google Cloud SQL

Your personal Google Cloud account can host PostgreSQL with Cloud SQL, a paid managed service. Local PostgreSQL is the default.

1. Create a Google Cloud project, enable Cloud SQL Admin API, and create a PostgreSQL instance, database, and user.
2. Grant appropriate IAM permissions and use Cloud SQL Auth Proxy or the Go connector.
3. Authenticate locally with Google Cloud CLI, then run:
   cloud-sql-proxy --port 5433 PROJECT:REGION:INSTANCE
4. Set DATABASE_URL to postgres://USER:URL_ENCODED_PASSWORD@127.0.0.1:5433/DATABASE?sslmode=disable
   The proxy encrypts the upstream connection; disabling SSL is appropriate only for the local proxy hop.
5. Containers need a reachable proxy sidecar or connector: localhost inside a container refers to that container. Override api DATABASE_URL and remove its local db dependency when moving to Cloud SQL.

Official guide: https://cloud.google.com/sql/docs/postgres/connect-auth-proxy

Production: use HTTPS, set SESSION_SECURE=true and PUBLIC_ORIGIN to the exact public origin, store secrets in a secret manager, keep data services private, configure backups, use versioned migrations. No Google credentials or cloud provisioning are included.

## Validate

In frontend: npm run typecheck; npm run build
In backend: go test ./...; go vet ./...

## Customize

Edit frontend/src/app/page.tsx for your name, biography, and links; layout.tsx for metadata. Manage projects at /admin. components.json configures adding shadcn/ui components. Add external API integrations following the Redis cache pattern, with tokens kept server-side.

Docker images run as non-root users. Development DB, Redis, and API ports bind to loopback. Only the frontend needs public ingress.

The container API development port is localhost:8081 (container port 8080). An ignored .env was generated with random passwords for local checks; view it locally to retrieve the bootstrap admin login, then replace ADMIN_EMAIL. Do not overwrite it after PostgreSQL initializes without also updating the database password.
Run node scripts/smoke.mjs against a fresh development stack to check API behavior. This consumes the shared contact rate limit; wait for expiry or remove development limiter keys before retesting.
If the shell inherits a nonstandard NODE_ENV, set it to production before npm run build, or development before npm run dev.
