# audit-go

Audit platform for joint ventures. The Go application is a modular monolith that owns identity integration, authorization, document metadata, processing orchestration, and immutable audit trails. Python is an internal processing service for OCR, parsing, embeddings, and AI-specific workloads.

## Architecture

```text
Client
  |
  | HTTPS
  v
Go API / Modular Monolith
  |-- access          users, roles, region/JV memberships
  |-- regions         regional authorization boundary
  |-- jointventures   JV lifecycle and region ownership
  |-- documents       document metadata and scoped CRUD
  |-- processing      workers and Python service integration
  |-- audit           immutable audit events
  |
  | SQL
  v
PostgreSQL + pgvector

Go processing worker
  |
  | internal HTTP
  v
Python service
  |-- PDF parsing
  |-- Excel parsing
  |-- OCR
  |-- embeddings / AI helpers
```

Go is the only public backend entry point. The frontend does not call Python directly. Microsoft Entra proves identity; application authorization is stored and enforced by Go with roles scoped to system, region, or joint venture.

## Project Structure

```text
cmd/
  audit/                 HTTP API entrypoint
  worker/                processing worker entrypoint

internal/
  features/
    access/              roles, scopes, memberships, authorization checks
    audit/               immutable audit events and persistence
    documents/           document domain, use cases, HTTP, Postgres
    jointventures/       JV domain
    processing/          Python client and workers
    regions/             region domain

  platform/
    config/              environment loading
    contextx/            request scoped values
    httpx/               HTTP helpers and middleware
    logger/              zerolog setup
    origin/              CORS origin allowlist
    postgres/            DB connection and transactions
    security/            Microsoft Entra token validation

python-service/
  main.py                FastAPI app
  processors/            PDF, Excel, OCR parsers
  ai/                    embeddings, chat, vector store protocol
```

## Authorization Model

Roles are application roles, not frontend-only flags:

- `admin`
- `region_admin`
- `jv_admin`
- `contributor`
- `auditor`
- `visitor`

Memberships are scoped with:

- `system`: global application scope
- `region`: applies to all JVs in a region
- `joint_venture`: applies to one JV

Document use cases authorize against the owning JV before reading, creating, or deleting data. Mutations and audit events are committed in the same database transaction.

## Prerequisites

| Tool | Version |
| ---- | ------- |
| Go | 1.26.2+ |
| Python | 3.12+ |
| Docker + Compose | v2 |
| PostgreSQL | 16 with pgvector |

## Getting Started

```bash
cp .env.example .env
```

Fill in:

- `ENTRA_TENANT_ID`
- `ENTRA_CLIENT_ID`
- `OPENAI_API_KEY` for Python AI helpers

Run the stack:

```bash
docker compose up --build
```

Run migrations in order:

```bash
psql "$DB_URL" -f db/migrations/001_create_joint_ventures.sql
psql "$DB_URL" -f db/migrations/002_create_documents.sql
psql "$DB_URL" -f db/migrations/003_create_audit_events.sql
psql "$DB_URL" -f db/migrations/004_enable_pgvector.sql
psql "$DB_URL" -f db/migrations/005_create_users.sql
psql "$DB_URL" -f db/migrations/006_create_refresh_tokens.sql
psql "$DB_URL" -f db/migrations/007_create_regions_and_access_memberships.sql
```

## API

```http
GET /health
```

Authenticated document endpoints:

```http
POST   /documents
GET    /documents/get?id=
DELETE /documents/delete?id=
```

`POST /documents`:

```json
{
  "jv_id": "uuid",
  "name": "contract.pdf",
  "type": "contract",
  "storage_key": "jv/123/contract.pdf"
}
```

Document types: `contract`, `financial`, `report`, `other`.

## Python Service

Internal only:

```http
GET  /health
POST /parse
```

Supported parser inputs:

- `.pdf`
- `.xlsx`

The parser response normalizes tables as:

```json
{
  "filename": "report.xlsx",
  "pages": 1,
  "text": "...",
  "markdown": "...",
  "tables": [
    {
      "sheet": "Sheet1",
      "rows": [["A", "B"], ["1", "2"]]
    }
  ]
}
```

## Development

```bash
go fmt ./...
go vet ./...
go test ./...
```

Python service:

```bash
cd python-service
pip install -r requirements.txt
uvicorn main:app --reload --port 8000
```

## Next Steps

- Add BFF login/callback/session endpoints for Entra authorization code flow.
- Store opaque HttpOnly application sessions and rotating refresh tokens.
- Add CSRF protection for cookie-authenticated mutating requests.
- Implement processing jobs/outbox so the Go worker calls Python and persists chunks.
- Add region and joint venture CRUD endpoints.
