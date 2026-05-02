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

## Documentation

- [FRONTEND_API_CONTRACT.md](FRONTEND_API_CONTRACT.md): contract consumivel pelo frontend com cookies, CSRF, auth, documents e upload.
- [DEVELOPMENT_GUIDE.md](DEVELOPMENT_GUIDE.md): direcao arquitetural, backlog e evolucao planejada da plataforma.

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
- `ENTRA_CLIENT_SECRET` when using a confidential server-side app registration
- `ENTRA_REDIRECT_URL`, usually `http://localhost:8080/auth/callback` locally
- `OPENAI_API_KEY` for Python AI helpers

Run the stack:

```bash
docker compose up --build
```

Run migrations in order:

```bash
psql "$DB_URL" -f db/migrations/001_create_regions.sql
psql "$DB_URL" -f db/migrations/002_create_users.sql
psql "$DB_URL" -f db/migrations/003_create_joint_ventures.sql
psql "$DB_URL" -f db/migrations/004_create_access_sessions.sql
psql "$DB_URL" -f db/migrations/005_create_access_memberships.sql
psql "$DB_URL" -f db/migrations/006_create_documents.sql
psql "$DB_URL" -f db/migrations/007_create_audit_events.sql
psql "$DB_URL" -f db/migrations/008_enable_pgvector.sql
psql "$DB_URL" -f db/migrations/009_create_storage_and_processing.sql
psql "$DB_URL" -f db/migrations/010_add_upload_confirmation_metadata.sql
```

## Frontend Quick Start

Para o frontend web atual, assuma estes pontos:

- base URL local: `http://localhost:8080`
- autenticacao principal: cookies com `credentials: "include"`
- cookie auth: `audit_session`
- refresh: `audit_refresh`
- CSRF header: `X-CSRF-Token` com o valor de `audit_csrf`

Fluxo minimo:

1. Redirecione o browser para `GET /auth/login?return_url=<frontend-url>`.
2. Depois do callback, chame `GET /auth/me` com `credentials: "include"`.
3. Em `POST`, `PUT`, `PATCH` e `DELETE`, envie `X-CSRF-Token`.
4. Se uma chamada autenticada voltar `401`, tente `POST /auth/refresh` e repita a request original.

Fluxo de upload atual:

1. Chame `POST /joint-ventures/{jvID}/documents/upload-url`.
2. Faça `PUT` direto para a URL do Blob retornada.
3. Confirme com `POST /documents/{documentID}/upload-complete`.

O backend hoje ja implementa:

- `GET /health`
- `GET /auth/me`
- `POST /auth/refresh`
- `POST /auth/logout`
- `POST /documents`
- `GET /joint-ventures/{jvID}/documents`
- `GET /documents/get?id=`
- `DELETE /documents/delete?id=`
- `POST /joint-ventures/{jvID}/documents/upload-url`
- `POST /documents/{documentID}/upload-complete`

Para detalhes de payload e respostas, use [FRONTEND_API_CONTRACT.md](FRONTEND_API_CONTRACT.md).

## API

```http
GET /health
```

Authentication endpoints:

```http
GET  /auth/login?return_url=
GET  /auth/callback
GET  /auth/me
POST /auth/refresh
POST /auth/logout
```

The API uses Microsoft Entra Authorization Code + PKCE and issues application cookies:

- `audit_session`: opaque HttpOnly app session.
- `audit_refresh`: opaque HttpOnly rotating refresh token.
- `audit_csrf`: readable double-submit CSRF token.

For mutating requests sent with cookies, clients must copy `audit_csrf` into the `X-CSRF-Token` header. The API still accepts Microsoft Entra bearer tokens for non-browser clients.

Authenticated document endpoints:

```http
POST   /documents
POST   /joint-ventures/{jvID}/documents/upload-url
POST   /documents/{documentID}/upload-complete
GET    /documents/get?id=
DELETE /documents/delete?id=
GET    /joint-ventures/{jvID}/documents
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

`POST /documents` creates the document with `status: "queued"` and, in the same PostgreSQL transaction, stores raw blob metadata in `storage_objects`, records the audit event, writes a `DocumentUploaded` outbox event, and creates an idempotent `parse_document:{document_id}:v1` processing job.

Preferred direct upload flow for browser clients:

1. Call `POST /joint-ventures/{jvID}/documents/upload-url` with `filename`, `type`, `content_type`, and optional `size_bytes`.
2. Upload the file with `PUT` to the returned Azure Blob URL using the returned headers.
3. Call `POST /documents/{documentID}/upload-complete` with optional `size_bytes`.

The backend creates the document as `upload_pending`, verifies the blob exists on completion, persists Blob metadata (`etag`, `version_id`, content type, size, verification timestamp), records audit events, and only then queues OCR/AI processing.

Azure upload configuration:

```bash
AZURE_STORAGE_ACCOUNT_NAME=<account>
AZURE_STORAGE_BLOB_CONTAINER=documents
AZURE_STORAGE_ENDPOINT=https://<account>.blob.core.windows.net/
DOCUMENT_UPLOAD_URL_TTL=15m
```

The API uses `DefaultAzureCredential`, so local development can use Azure CLI credentials and Azure runtime should use Managed Identity with Blob permissions that can generate user delegation SAS and read/write blobs.

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

- Make the Go worker consume `processing_jobs`, read the verified Blob, call Azure AI Document Intelligence/Python, and persist parsed artifacts.
- Add region and joint venture CRUD endpoints.
