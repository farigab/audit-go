# Audit Platform — Advanced Architecture (C4 + OpenAPI + Data Model)

Este documento complementa o `ARCHITECTURE.md` com:

- Visão C4
- Contrato de API inicial
- Modelo de dados pronto para implementação

---

## 1. C4 — Context Diagram

```text
[ User / Frontend ]
          |
          v
[ Go API (Audit Platform) ]
    |         |         |
    v         v         v
[ PostgreSQL ] [ Blob Storage ] [ Python Service ]
                                        |
                                        v
                                [ Azure AI Services ]
```

Serviços externos:

- Microsoft Entra (autenticação)
- Azure Blob Storage (arquivos)
- Azure OpenAI (LLM)
- Azure Document Intelligence (OCR)

---

## 2. C4 — Container Diagram

```text
Client (Browser)
   |
   | HTTPS (cookies + CSRF)
   v
Go API (Monolith)          ← binário principal: HTTP handlers, auth, RBAC
   |
   | SQL
   v
PostgreSQL

Go Worker (processo separado, mesmo repositório)
   |
   | HTTP interno
   v
Python Service

Storage:
Azure Blob Storage
```

> **Nota:** Go API e Go Worker são dois binários distintos compilados do mesmo
> repositório (`cmd/api` e `cmd/worker`). Cada um tem seu próprio health check,
> deploy e escala independente. O Worker nunca expõe porta pública.

Roadmap:

```text
Go API → Outbox → Queue (Azure Service Bus) → Workers
```

---

## 3. OpenAPI (versão inicial)

```yaml
openapi: 3.0.0
info:
  title: Audit Platform API
  version: 1.0.0

paths:
  /auth/me:
    get:
      summary: Current user
      responses:
        "200":
          description: OK

  /documents:
    post:
      summary: Create document (metadata) + gera SAS URL para upload direto
      requestBody:
        required: true
        content:
          application/json:
            schema:
              # TODO: extrair para componente reutilizável
              type: object
              required: [jv_id, name, type]
              properties:
                jv_id:
                  type: string
                  format: uuid
                name:
                  type: string
                type:
                  type: string
                  enum: [contract, invoice, report, other]
      responses:
        "201":
          description: Created
          content:
            application/json:
              schema:
                type: object
                properties:
                  document_id:
                    type: string
                    format: uuid
                  upload_url:
                    type: string
                    description: SAS URL para upload direto ao Blob Storage

  /documents/{id}:
    get:
      summary: Get document
      parameters:
        - in: path
          name: id
          schema:
            type: string
            format: uuid
          required: true
      responses:
        "200":
          description: OK

  /documents/{id}/upload-complete:
    post:
      summary: Confirmar upload concluído — dispara processing_job
      parameters:
        - in: path
          name: id
          schema:
            type: string
            format: uuid
          required: true
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
              required: [checksum]
              properties:
                checksum:
                  type: string
                  description: SHA-256 do arquivo enviado, para validação de integridade
      responses:
        "202":
          description: Accepted — job enfileirado

  /documents/{id}/process:
    post:
      summary: Re-trigger processing (reprocessamento manual)
      parameters:
        - in: path
          name: id
          schema:
            type: string
            format: uuid
          required: true
      responses:
        "202":
          description: Accepted

  /joint-ventures/{jvId}/documents:
    get:
      summary: List documents
      parameters:
        - in: path
          name: jvId
          schema:
            type: string
            format: uuid
          required: true
      responses:
        "200":
          description: OK

  /audits:
    post:
      summary: Start audit run
      requestBody:
        required: true
        content:
          application/json:
            schema:
              # TODO: adicionar schema completo
              type: object
      responses:
        "201":
          description: Created

  /audits/{id}:
    get:
      summary: Get audit run
      parameters:
        - in: path
          name: id
          schema:
            type: string
            format: uuid
          required: true
      responses:
        "200":
          description: OK

  /audits/{id}/findings:
    get:
      summary: List findings
      parameters:
        - in: path
          name: id
          schema:
            type: string
            format: uuid
          required: true
      responses:
        "200":
          description: OK
```

---

## 4. Modelo de Dados

### Tipos enumerados

```sql
-- Centralizar enums evita typos silenciosos em campos status/tipo
create type document_status   as enum ('pending', 'processing', 'done', 'failed');
create type job_status        as enum ('pending', 'processing', 'done', 'failed');
create type audit_status      as enum ('pending', 'running', 'done', 'failed');
create type finding_severity  as enum ('low', 'medium', 'high', 'critical');
create type outbox_status     as enum ('pending', 'sent', 'failed');
create type prompt_status     as enum ('draft', 'active', 'deprecated');
create type membership_scope  as enum ('global', 'region', 'joint_venture');
```

---

### users

```sql
create table users (
  id              uuid primary key default gen_random_uuid(),
  entra_object_id text        not null unique,
  email           text        not null unique,
  name            text,
  created_at      timestamptz not null default now(),
  updated_at      timestamptz not null default now()
);
```

---

### regions

```sql
create table regions (
  id         uuid primary key default gen_random_uuid(),
  code       text unique not null,
  name       text        not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
```

---

### joint_ventures

```sql
create table joint_ventures (
  id         uuid primary key default gen_random_uuid(),
  region_id  uuid        not null references regions(id),
  name       text        not null,
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);
```

---

### memberships

```sql
-- RBAC polimórfico: scope_id referencia a tabela indicada por scope_type.
-- Não há FK nativa para polimorfismo; a integridade é garantida via
-- check constraint + validação na camada de serviço.
-- scope_id é NULL quando scope_type = 'global'.
create table memberships (
  id         uuid            primary key default gen_random_uuid(),
  user_id    uuid            not null references users(id) on delete cascade,
  role       text            not null,
  scope_type membership_scope not null,
  scope_id   uuid,           -- NULL para scope 'global'
  created_at timestamptz     not null default now(),
  updated_at timestamptz     not null default now(),

  constraint memberships_scope_consistency check (
    (scope_type = 'global'        and scope_id is null) or
    (scope_type in ('region', 'joint_venture') and scope_id is not null)
  )
);

create index on memberships(user_id, scope_type, scope_id);
```

---

### documents

```sql
create table documents (
  id          uuid            primary key default gen_random_uuid(),
  jv_id       uuid            not null references joint_ventures(id),
  name        text            not null,
  type        text            not null,
  status      document_status not null default 'pending',
  storage_key text,
  -- SHA-256 hex do arquivo; preenchido em POST /upload-complete
  checksum    text,
  created_at  timestamptz     not null default now(),
  updated_at  timestamptz     not null default now()
);

create index on documents(jv_id, status);
```

---

### document_processing_steps

```sql
-- Estado de negócio por etapa (OCR, parse, embed, etc.)
-- Separado de processing_jobs que é fila interna de execução.
create table document_processing_steps (
  id          uuid primary key default gen_random_uuid(),
  document_id uuid        not null references documents(id) on delete cascade,
  step        text        not null,
  status      job_status  not null default 'pending',
  error       text,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);
```

---

### processing_jobs

```sql
create table processing_jobs (
  id          uuid       primary key default gen_random_uuid(),
  job_type    text       not null,
  document_id uuid       not null references documents(id) on delete cascade,
  status      job_status not null default 'pending',
  payload     jsonb,
  attempts    int        not null default 0,
  last_error  text,
  created_at  timestamptz not null default now(),
  updated_at  timestamptz not null default now()
);

create index on processing_jobs(status, created_at)
  where status in ('pending', 'failed');
```

---

### outbox_events

```sql
create table outbox_events (
  id           uuid         primary key default gen_random_uuid(),
  event_type   text         not null,
  payload      jsonb        not null,
  status       outbox_status not null default 'pending',
  attempts     int          not null default 0,
  error        text,
  created_at   timestamptz  not null default now(),
  processed_at timestamptz
);

create index on outbox_events(status, created_at)
  where status = 'pending';
```

---

### prompts

```sql
create table prompts (
  id         uuid          primary key default gen_random_uuid(),
  name       text          not null,
  content    text          not null,
  version    int           not null,
  status     prompt_status not null default 'draft',
  created_at timestamptz   not null default now(),
  updated_at timestamptz   not null default now(),

  constraint prompts_name_version_unique unique (name, version)
);
```

---

### audit_runs

```sql
create table audit_runs (
  id         uuid         primary key default gen_random_uuid(),
  jv_id      uuid         not null references joint_ventures(id),
  status     audit_status not null default 'pending',
  created_at timestamptz  not null default now(),
  updated_at timestamptz  not null default now()
);

create index on audit_runs(jv_id, status);
```

---

### audit_findings

```sql
create table audit_findings (
  id           uuid             primary key default gen_random_uuid(),
  audit_run_id uuid             not null references audit_runs(id) on delete cascade,
  document_id  uuid             not null references documents(id),
  severity     finding_severity not null,
  description  text             not null,
  evidence     text,
  created_at   timestamptz      not null default now(),
  updated_at   timestamptz      not null default now()
);

create index on audit_findings(audit_run_id, severity);
```

---

### Trigger: updated_at automático

```sql
-- Função genérica; associar a cada tabela que precisar.
create or replace function set_updated_at()
returns trigger language plpgsql as $$
begin
  new.updated_at = now();
  return new;
end;
$$;

-- Exemplo de associação (repetir para cada tabela):
create trigger trg_documents_updated_at
  before update on documents
  for each row execute function set_updated_at();
```

---

## 5. Fluxo principal (end-to-end)

### Upload

```text
1. POST /documents                  → cria registro com status 'pending', retorna SAS URL
2. Cliente faz PUT direto ao Blob   → sem passar pelo Go API
3. POST /documents/{id}/upload-complete  → valida checksum SHA-256
4. Go API cria processing_job       → status 'pending'
5. Go API insere outbox_event       → na mesma transação (atomicidade)
```

### Processamento

```text
Go Worker:
  → polling em processing_jobs onde status = 'pending'
  → marca status = 'processing' (select for update skip locked)
  → chama Python Service (/parse)
  → salva texto extraído
  → atualiza document.status + document_processing_steps
  → em caso de falha: incrementa attempts, salva last_error, agenda retry com backoff
```

### Auditoria

```text
1. POST /audits                     → cria audit_run com status 'pending'
2. Go Worker pega audit_run         → executa prompts ativos da tabela prompts
3. Chama Azure OpenAI com contexto  → uma chamada por prompt/documento
4. Persiste audit_findings          → com severity e evidence
5. Atualiza audit_run.status        → 'done' ou 'failed'
```

---

## 6. Regras críticas

| Regra | Detalhe |
|---|---|
| **DB é fonte de verdade** | Nenhum estado existe apenas em memória ou em variável de ambiente |
| **IA nunca decide permissão** | RBAC é sempre verificado pelo Go API antes de qualquer chamada ao modelo |
| **Idempotência** | Jobs devem ser reexecutáveis — `select for update skip locked` + `attempts` |
| **Checksum obrigatório** | SHA-256 do arquivo validado em `upload-complete` antes de enfileirar job |
| **Auditabilidade** | Todo estado mutável tem `created_at` + `updated_at`; erros são persistidos |

---

## 7. Próximos upgrades

- [ ] Azure Service Bus (substituir polling por push)
- [ ] `prompt_versions` com histórico imutável
- [ ] Embeddings com `pgvector` para busca semântica
- [ ] Relatórios exportáveis (PDF/Excel)
- [ ] Multi-tenant isolado por schema PostgreSQL

---

## 8. Checklist de produção

- [ ] CSRF ativo em todos os endpoints mutáveis
- [ ] Cookies com `Secure`, `HttpOnly`, `SameSite=Strict`
- [ ] Retry com exponential backoff no Worker
- [ ] Logs estruturados (JSON) com `trace_id` propagado
- [ ] Rate limiting por usuário/IP no Go API
- [ ] Validação de permissão em **todos** os endpoints (sem exceção)
- [ ] Health check independente para Go API e Go Worker
- [ ] `select for update skip locked` em todos os job queues
- [ ] Alertas em `processing_jobs.attempts >= 3`

---

## 9. Regra final

> **Se não for auditável, não está pronto.**
