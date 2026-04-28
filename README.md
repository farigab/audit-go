# audit-go

Audit platform for Joint Ventures. Manages document ingestion (PDF/XLSX), AI-powered chat over documents, and immutable audit trails — in a multi-tenant architecture.

---

## Architecture

```text
┌─────────────────────────────────────────┐
│              Client (Web/App)           │
└──────────────────┬──────────────────────┘
                   │ HTTP
┌──────────────────▼──────────────────────┐
│             Go API  (audit-go)          │
│  · Authentication / Multi-tenant        │
│  · Audit event recording                │
│  · Use case orchestration               │
│  · File upload → S3/MinIO               │
└────────┬─────────────────┬──────────────┘
         │ HTTP             │ SQL
┌────────▼────────┐  ┌──────▼─────────────┐
│ Python Service  │  │    PostgreSQL       │
│  (FastAPI)      │  │    + pgvector       │
│  · Parse PDF    │  │    · documents      │
│  · Parse Excel  │  │    · audit_events   │
│  · OCR          │  │    · embeddings     │
│  · Embeddings   │  │    · joint_ventures │
│  · Chat / RAG   │  └────────────────────┘
└─────────────────┘
```

**Responsibility boundary:** Go handles infrastructure, security, and audit; Python handles document processing and AI. Clients never call Python directly — Go is the single entry point.

---

## Project Structure

```text
audit-go/
├── cmd/
│   ├── audit/          # HTTP server entrypoint
│   └── worker/         # Background worker (PDF/XLSX processing)
│
├── internal/
│   ├── domain/
│   │   ├── document.go
│   │   ├── audit_event.go
│   │   └── joint_venture.go
│   │
│   ├── delivery/http/
│   │   ├── handler.go
│   │   ├── middleware.go
│   │   └── response.go
│   │
│   ├── usecase/
│   │   ├── create_document.go
│   │   ├── delete_document.go
│   │   └── get_document.go
│   │
│   ├── infrastructure/
│   │   ├── postgres/   # Repositories (documents, audit events)
│   │   └── python/     # HTTP client → Python service
│   │
│   ├── worker/
│   │   └── file_worker.go
│   │
│   └── platform/
│       ├── contextx/   # Request-scoped values (tenant, user, request ID)
│       └── logger/     # zerolog wrappers (JSON prod, pretty dev)
│
├── db/
│   └── migrations/
│       ├── 001_create_joint_ventures.sql
│       ├── 002_create_documents.sql
│       ├── 003_create_audit_events.sql
│       └── 004_enable_pgvector.sql
│
└── python-service/
    ├── main.py
    ├── processors/
    │   ├── pdf.py      # pdfplumber
    │   ├── excel.py    # openpyxl
    │   └── ocr.py      # pytesseract
    └── ai/
        ├── embeddings.py   # text-embedding-3-small
        ├── chat.py         # gpt-4o-mini
        └── vector_store.py # pgvector / ChromaDB (protocol stub)
```

---

## Prerequisites

| Tool | Version |
| ---- | ------- |
| Go | 1.24+ |
| Python | 3.12+ |
| Docker + Compose | v2 |
| PostgreSQL | 16 (via pgvector image) |
| golangci-lint | latest (for local linting) |

---

## Getting Started

### 1. Environment

```bash
cp .env.example .env
# Edit .env and fill in OPENAI_API_KEY and any other values
```

### 2. Run with Docker Compose (recommended)

```bash
docker compose up --build
```

This starts:

- `api` — Go server on `:8080`
- `python-service` — FastAPI on `:8000`
- `postgres` — PostgreSQL 16 with pgvector on `:5432`

### 3. Run database migrations

```bash
psql "$POSTGRES_DSN" -f db/migrations/001_create_joint_ventures.sql
psql "$POSTGRES_DSN" -f db/migrations/002_create_documents.sql
psql "$POSTGRES_DSN" -f db/migrations/003_create_audit_events.sql
psql "$POSTGRES_DSN" -f db/migrations/004_enable_pgvector.sql
```

### 4. Run locally (Go only)

```bash
# Windows
run.bat

# Linux / macOS
go run ./cmd/audit
```

---

## Configuration

All configuration is via environment variables:

| Variable | Default | Description |
| -------- | ------- | ----------- |
| `ADDR` | `:8080` | HTTP listen address |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `POSTGRES_DSN` | `postgres://audit:audit@localhost:5432/auditdb?sslmode=disable` | PostgreSQL connection string |
| `PYTHON_SERVICE_URL` | `http://localhost:8000` | Internal Python service base URL |
| `OPENAI_API_KEY` | — | Required by the Python service |

---

## API

All endpoints expect `X-User-ID` and `X-Tenant-ID` headers for multi-tenancy.

### Health

```http
GET /health
→ 200 "ok"
```

### Documents

```http
POST   /documents         — create document record
GET    /documents/get?id= — get document by ID
DELETE /documents/delete?id= — delete document
```

**POST /documents** body:

```json
{
  "jv_id": "uuid",
  "name": "contract.pdf",
  "type": "contract",
  "storage_key": "jv/123/contract.pdf"
}
```

Document types: `contract`, `financial`, `report`, `other`.

---

## Python Service

Internal only — not exposed to clients.

```http
POST /parse   — parses PDF or XLSX, returns text + tables + markdown
GET  /health  — liveness check
```

Supported file types: `.pdf`, `.xlsx`, `.xls`.

---

## Key Design Decisions

**Immutable audit log** — `audit_events` is insert-only. Every mutation (create, delete, AI query) produces a new event. No updates, no deletes. This is enforced at the use case layer.

**Minimal interfaces per use case** — each use case defines only the repository methods it actually needs (e.g., `createDocumentRepo` only has `Save`). This keeps tests simple and avoids bloated interfaces.

**Go orchestrates, Python processes** — Go owns the request lifecycle, auth context, and audit trail. Python owns CPU/IO-heavy work (parsing, embeddings, RAG). Decoupled via HTTP; could be replaced with gRPC with no domain changes.

**pgvector for embeddings** — document chunks are stored alongside business data in the same Postgres instance. Avoids an extra vector DB for the current scale. The `VectorStore` protocol in Python makes it easy to swap to Chroma or Qdrant later.

**Multi-tenancy via headers** — `X-Tenant-ID` and `X-User-ID` are injected into context by the `RequestContext` middleware and propagated through every use case and audit event. No tenant leakage is possible as long as queries always include `tenant_id`.

---

## Development

### Lint & test (Windows)

```bat
check.bat   # fmt + vet + golangci-lint + tests
lint.bat    # lint only
```

### Lint & test (Linux/macOS)

```bash
go fmt ./...
go vet ./...
golangci-lint run
go test ./...
```

### Python service

```bash
cd python-service
pip install -r requirements.txt
uvicorn main:app --reload --port 8000
```

---

## Roadmap

- [ ] `ProcessDocument` use case — Go worker calls Python `/parse`, stores chunks + embeddings
- [ ] `Chat` use case — retrieval + LLM call, logs `chat.queried` audit event
- [ ] S3/MinIO integration — actual file storage (currently only `storage_key` is tracked)
- [ ] JointVenture CRUD endpoints
- [ ] Authentication middleware (JWT / API key)
- [ ] Pagination on `FindByTenant` and `FindByJVID`
- [ ] Graceful shutdown for HTTP server
- [ ] `pgvector` implementation of `VectorStore` protocol
- [ ] Migration runner (replace manual `psql` calls)
