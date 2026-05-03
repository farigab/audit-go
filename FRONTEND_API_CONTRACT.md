# Frontend API Contract

Contrato do frontend para a plataforma `audit-go`, alinhado ao backend Go atual.

Este documento separa:

- **Disponivel agora**: endpoints ja implementados.
- **Planejado**: areas que o frontend pode prever, mas ainda precisam de mocks/adapters.

## Base URL

Local:

```text
http://localhost:8080
```

Todas as chamadas autenticadas do frontend web devem usar cookies:

```ts
fetch("http://localhost:8080/auth/me", {
  credentials: "include",
});
```

## Resumo de Disponibilidade

| Area | Status backend | Observacao frontend |
| --- | --- | --- |
| Health | Disponivel | Endpoint simples para status/dev |
| Auth BFF Entra | Disponivel | Login via redirect, sessao por cookies |
| CSRF | Disponivel | Obrigatorio em requests mutaveis com cookies de auth |
| Documents metadata | Disponivel | Criar, buscar, listar por JV, deletar |
| Upload Blob | Disponivel | URL assinada para upload direto ao Azure Blob |
| Processing status | Disponivel | `document.status` no payload e endpoint dedicado por documento |
| Processed chunks | Disponivel | Consulta paginada de chunks extraidos por documento |
| Storage metadata | Interno | Persistido no backend; frontend nao chama direto |
| Outbox/jobs | Interno | Persistido e publicado internamente; frontend nao chama direto |
| Regions CRUD | Disponivel | CRUD autenticado com permissao por escopo |
| Joint Ventures CRUD | Disponivel | CRUD autenticado com heranca system/region/JV |
| Memberships | Disponivel | Administracao de acessos por system/region/JV |
| Audit runs/findings | Planejado | Mockar telas ate existir backend |
| Prompt management | Planejado | Mockar telas ate existir backend |

## Quick Start do Frontend

Se o objetivo for subir o frontend rapido contra o backend atual, use este fluxo.

### 1. Configuração base

```ts
const API_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080";
```

Todas as requests autenticadas do frontend web devem usar:

```ts
credentials: "include"
```

### 2. Login e bootstrap de sessão

Redirecione o browser para login:

```ts
window.location.href =
  `${API_URL}/auth/login?return_url=${encodeURIComponent(window.location.origin)}`;
```

Depois do redirect de volta ao frontend, carregue o usuário atual:

```ts
const me = await fetch(`${API_URL}/auth/me`, {
  credentials: "include",
});
```

### 3. Helper mínimo para API

```ts
function getCookie(name: string) {
  return document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${name}=`))
    ?.split("=")[1];
}

export async function apiFetch(path: string, init: RequestInit = {}) {
  const method = init.method?.toUpperCase() ?? "GET";
  const mutating = ["POST", "PUT", "PATCH", "DELETE"].includes(method);

  const headers = new Headers(init.headers);
  if (mutating) {
    headers.set("X-CSRF-Token", getCookie("audit_csrf") ?? "");
  }

  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });

  if (response.status === 401 && path !== "/auth/refresh") {
    const refreshed = await fetch(`${API_URL}/auth/refresh`, {
      method: "POST",
      credentials: "include",
      headers: {
        "X-CSRF-Token": getCookie("audit_csrf") ?? "",
      },
    });

    if (refreshed.ok) {
      return apiFetch(path, init);
    }
  }

  return response;
}
```

### 4. Fluxos já implementados

Auth:

- `GET /auth/login`
- `GET /auth/callback`
- `GET /auth/me`
- `POST /auth/refresh`
- `POST /auth/logout`

Documents:

- `POST /documents`
- `GET /joint-ventures/{jvID}/documents`
- `GET /documents/get?id=`
- `GET /documents/{documentID}/processing-status`
- `GET /documents/{documentID}/chunks`
- `DELETE /documents/delete?id=`

Upload:

- `POST /joint-ventures/{jvID}/documents/upload-url`
- `POST /documents/{documentID}/upload-complete`

Access:

- `GET /access/memberships`
- `POST /access/memberships`
- `DELETE /access/memberships/{membershipID}`

Regions:

- `GET /regions`
- `POST /regions`
- `GET /regions/{regionID}`
- `PATCH /regions/{regionID}`
- `DELETE /regions/{regionID}`

Joint Ventures:

- `GET /regions/{regionID}/joint-ventures`
- `POST /regions/{regionID}/joint-ventures`
- `GET /joint-ventures/{jvID}`
- `PATCH /joint-ventures/{jvID}`
- `DELETE /joint-ventures/{jvID}`

### 5. Fluxo de upload recomendado

1. Chame `POST /joint-ventures/{jvID}/documents/upload-url`.
2. Use `upload.method`, `upload.url` e `upload.headers` para fazer o `PUT` direto no Blob.
3. Chame `POST /documents/{documentID}/upload-complete`.
4. Atualize a UI com o `document.status` retornado.

### 6. O que ainda deve ser mockado

No backend atual, ainda nao existem handlers HTTP para:

- audit runs/findings
- prompt management

## Convencoes Gerais

### Cookies

Cookies emitidos hoje pelo backend:

| Cookie | HttpOnly | Path | Uso |
| --- | --- | --- | --- |
| `audit_session` | Sim | `/` | Sessao opaca da aplicacao |
| `audit_refresh` | Sim | `/auth` | Refresh token opaco e rotacionado |
| `audit_csrf` | Nao | `/` | Token CSRF para requests mutaveis |

Comportamento atual:

- `Secure` e controlado por configuracao de ambiente e deve ficar ligado em producao.
- `SameSite` default e `Lax`.
- TTL default de `audit_session`: `15m`.
- TTL default de `audit_refresh`: `30d`.
- `audit_csrf` hoje e reemitido junto com login/refresh e expira junto com o refresh atual emitido pelo backend.

### CSRF

Requests `POST`, `PUT`, `PATCH` e `DELETE` feitos com cookies de autenticacao devem enviar:

```http
X-CSRF-Token: <valor do cookie audit_csrf>
```

O middleware CSRF atual so valida quando existe pelo menos um destes cookies:

- `audit_session`
- `audit_refresh`

Isso significa:

- `POST /auth/refresh` exige `audit_refresh` e tambem exige header CSRF valido.
- endpoints mutaveis autenticados por sessao exigem `audit_session` e header CSRF valido.

### Autenticacao do backend

Para requests do frontend web, o fluxo esperado e cookie-based.

O backend atual tambem aceita `Authorization: Bearer <token>` no middleware de auth para clientes nao-browser em endpoints protegidos. Para o frontend web, ignore esse modo e use `credentials: "include"`.

### Formato de erro

Handlers de aplicacao retornam JSON neste formato:

```json
{
  "error": "message"
}
```

Alguns erros emitidos diretamente por middleware ainda podem sair como `text/plain`.

## Tipos TypeScript Base

```ts
export type Role =
  | "admin"
  | "region_admin"
  | "jv_admin"
  | "contributor"
  | "auditor"
  | "visitor";

export type ScopeType = "system" | "region" | "joint_venture";

export type Principal = {
  ID: string;
  Login: string;
  Name: string;
  Roles: Role[];
};

export type DocumentType = "contract" | "financial" | "report" | "other";

export type DocumentStatus =
  | "upload_pending"
  | "uploaded"
  | "registered"
  | "queued"
  | "processing"
  | "parsed"
  | "ocr_completed"
  | "indexed"
  | "failed"
  | "deleted";

export type AuditDocument = {
  id: string;
  jv_id: string;
  name: string;
  type: DocumentType;
  storage_key: string;
  uploaded_by: string;
  uploaded_at: string;
  status: DocumentStatus;
  processed: boolean;
};

export type UploadTarget = {
  method: string;
  url: string;
  headers: Record<string, string>;
  expires_at: string;
};

export type RequestDocumentUploadResponse = {
  document: AuditDocument;
  upload: UploadTarget;
};

export type DocumentProcessingJobStatus = {
  id: string;
  type: string;
  status:
    | "queued"
    | "running"
    | "retry_scheduled"
    | "completed"
    | "failed"
    | "dead_letter";
  attempts: number;
  max_attempts: number;
  available_at: string;
  locked_until?: string;
  last_error?: string;
  last_transition: string;
};

export type DocumentParseResultSummary = {
  document_id: string;
  filename: string;
  pages: number;
  text_bytes: number;
  markdown_bytes: number;
  tables_count: number;
  chunks_count: number;
  raw_sha256?: string;
  text_sha256?: string;
  markdown_sha256?: string;
  tables_sha256?: string;
  last_parsed_at?: string;
};

export type DocumentChunkRecord = {
  id: string;
  document_id: string;
  index: number;
  content: string;
  created_at: string;
};

export type DocumentChunksPage = {
  document_id: string;
  chunks: DocumentChunkRecord[];
  limit: number;
  offset: number;
  count: number;
};

export type DocumentProcessingStatusResponse = {
  document: AuditDocument;
  job?: DocumentProcessingJobStatus;
  parse_result?: DocumentParseResultSummary;
};

export type ApiError = {
  error: string;
};
```

## Cliente Frontend Recomendado

```ts
const API_URL = import.meta.env.VITE_API_URL ?? "http://localhost:8080";

function getCookie(name: string) {
  return document.cookie
    .split("; ")
    .find((row) => row.startsWith(`${name}=`))
    ?.split("=")[1];
}

export async function apiFetch(path: string, init: RequestInit = {}) {
  const method = init.method?.toUpperCase() ?? "GET";
  const mutating = ["POST", "PUT", "PATCH", "DELETE"].includes(method);

  const headers = new Headers(init.headers);
  if (mutating) {
    headers.set("X-CSRF-Token", getCookie("audit_csrf") ?? "");
  }

  const response = await fetch(`${API_URL}${path}`, {
    ...init,
    credentials: "include",
    headers,
  });

  if (response.status === 401 && path !== "/auth/refresh") {
    const refreshed = await fetch(`${API_URL}/auth/refresh`, {
      method: "POST",
      credentials: "include",
      headers: {
        "X-CSRF-Token": getCookie("audit_csrf") ?? "",
      },
    });

    if (refreshed.ok) {
      return apiFetch(path, init);
    }
  }

  return response;
}
```

## Autenticacao

O backend usa Microsoft Entra no modelo BFF:

1. Frontend redireciona o browser para `GET /auth/login`.
2. Backend redireciona para Microsoft Entra.
3. Entra redireciona para `GET /auth/callback`.
4. Backend cria cookies da aplicacao e redireciona para o frontend.
5. Frontend chama APIs usando `credentials: "include"`.

O frontend nao armazena access token da Microsoft.

### GET `/auth/login`

Inicia login com Microsoft Entra.

Query params:

| Nome | Obrigatorio | Descricao |
| --- | --- | --- |
| `return_url` | Nao | URL ou path para redirecionar apos login |

Uso recomendado:

```ts
window.location.href =
  `${API_URL}/auth/login?return_url=${encodeURIComponent(window.location.origin)}`;
```

Resposta:

```http
302 Location: https://login.microsoftonline.com/...
```

### GET `/auth/callback`

Callback da Microsoft Entra. O frontend nao deve chamar diretamente.

Resposta de sucesso:

```http
302 Location: <return_url>
Set-Cookie: audit_session=...
Set-Cookie: audit_refresh=...
Set-Cookie: audit_csrf=...
```

Falha tipica:

| Status | Motivo |
| --- | --- |
| `401` | Login falhou |

### GET `/auth/me`

Retorna o usuario autenticado.

Para frontend web, chame com cookies.

Request:

```ts
const response = await apiFetch("/auth/me");
const user = (await response.json()) as Principal;
```

Resposta `200`:

```json
{
  "ID": "entra-oid-or-login",
  "Login": "user@example.com",
  "Name": "User Name",
  "Roles": ["admin"]
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Autenticado |
| `401` | Sem sessao valida ou sem bearer valido |

### POST `/auth/refresh`

Rotaciona `audit_refresh`, emite novo `audit_session` e reemite `audit_csrf`.

Requer:

- cookie `audit_refresh`
- header `X-CSRF-Token` valido

Request:

```ts
await apiFetch("/auth/refresh", { method: "POST" });
```

Resposta `200`:

```json
{
  "user": {
    "ID": "entra-oid-or-login",
    "Login": "user@example.com",
    "Name": "User Name",
    "Roles": ["admin"]
  }
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Refresh aceito |
| `401` | Refresh ausente, expirado, revogado ou invalido |
| `403` | CSRF ausente ou invalido |

### POST `/auth/logout`

Revoga sessao e refresh token atuais quando presentes e limpa os cookies do navegador.

Observacao: o backend atual le `audit_refresh` no path `/auth`, por isso o cookie de refresh esta nesse path hoje.

Request:

```ts
await apiFetch("/auth/logout", { method: "POST" });
```

Resposta `200`:

```json
{
  "status": "logged_out"
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Logout executado e cookies limpos |
| `403` | CSRF ausente ou invalido |

## Health

### GET `/health`

Endpoint simples de liveness.

Resposta `200`:

```text
ok
```

## Documents

Todos os endpoints abaixo exigem autenticacao.

Permissoes esperadas:

| Operacao | Roles aceitas no escopo da JV/regiao/sistema |
| --- | --- |
| Criar documento | `admin`, `region_admin`, `jv_admin`, `contributor` |
| Ler documento | `admin`, `region_admin`, `jv_admin`, `contributor`, `auditor`, `visitor` |
| Deletar documento | `admin`, `region_admin`, `jv_admin` |

Escopos aceitos:

| Scope | Descricao |
| --- | --- |
| `system` | Acesso global |
| `region` | Acesso a todas as JVs da regiao |
| `joint_venture` | Acesso a uma JV especifica |

### Status de Documento

| Status | Significado UI |
| --- | --- |
| `upload_pending` | Upload ainda nao confirmado |
| `uploaded` | Blob identificado mas ainda nao enfileirado |
| `registered` | Metadados registrados |
| `queued` | Job de processamento criado |
| `processing` | Worker processando |
| `parsed` | Texto e tabelas extraidos |
| `ocr_completed` | OCR finalizado |
| `indexed` | Conteudo indexado |
| `failed` | Falha no processamento |
| `deleted` | Documento removido |

### POST `/documents`

Cria o registro de metadados de um documento.

Este endpoint nao faz upload binario. Ele espera um `storage_key` ja definido pelo fluxo do frontend.

Ao criar o documento, o backend tambem:

- persiste metadados em `storage_objects`;
- registra evento de auditoria;
- cria outbox event `DocumentUploaded`;
- cria job idempotente de parse;
- retorna o documento com `status: "queued"`.

Body:

```json
{
  "jv_id": "00000000-0000-0000-0000-000000000000",
  "name": "contract.pdf",
  "type": "contract",
  "storage_key": "tenants/demo/regions/latam/jvs/jv-001/documents/doc-001/raw/contract.pdf"
}
```

Request:

```ts
const response = await apiFetch("/documents", {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    jv_id: selectedJvId,
    name: file.name,
    type: "contract",
    storage_key:
      "tenants/demo/regions/latam/jvs/jv-001/documents/doc-001/raw/contract.pdf",
  }),
});

const document = (await response.json()) as AuditDocument;
```

Resposta `201`:

```json
{
  "id": "document-uuid",
  "jv_id": "jv-uuid",
  "name": "contract.pdf",
  "type": "contract",
  "storage_key": "tenants/demo/regions/latam/jvs/jv-001/documents/doc-001/raw/contract.pdf",
  "uploaded_by": "user@example.com",
  "uploaded_at": "2026-05-02T12:00:00Z",
  "status": "queued",
  "processed": false
}
```

Status:

| Status | Motivo |
| --- | --- |
| `201` | Documento criado |
| `400` | Body invalido, `jv_id` invalido ou tipo invalido |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV |
| `500` | Erro interno |

### GET `/documents/get?id=<document_id>`

Busca um documento por ID.

Request:

```ts
const response = await apiFetch(`/documents/get?id=${documentId}`);
const document = (await response.json()) as AuditDocument;
```

Resposta `200`:

```json
{
  "id": "document-uuid",
  "jv_id": "jv-uuid",
  "name": "contract.pdf",
  "type": "contract",
  "storage_key": "tenants/demo/regions/latam/jvs/jv-001/documents/doc-001/raw/contract.pdf",
  "uploaded_by": "user@example.com",
  "uploaded_at": "2026-05-02T12:00:00Z",
  "status": "queued",
  "processed": false
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Documento encontrado |
| `400` | `id` ausente |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV |
| `404` | Documento nao encontrado |

### GET `/documents/{documentID}/processing-status`

Busca o estado dedicado de processamento de um documento.

Request:

```ts
const response = await apiFetch(`/documents/${documentId}/processing-status`);
const status = (await response.json()) as DocumentProcessingStatusResponse;
```

Resposta `200`:

```json
{
  "document": {
    "id": "document-uuid",
    "jv_id": "jv-uuid",
    "name": "contract.pdf",
    "type": "contract",
    "storage_key": "jvs/{jv_id}/documents/{document_id}/raw/contract.pdf",
    "uploaded_by": "user@example.com",
    "uploaded_at": "2026-05-02T12:00:00Z",
    "status": "parsed",
    "processed": true
  },
  "job": {
    "id": "job-uuid",
    "type": "parse_document",
    "status": "completed",
    "attempts": 1,
    "max_attempts": 5,
    "available_at": "2026-05-02T12:00:00Z",
    "last_transition": "2026-05-02T12:01:00Z"
  },
  "parse_result": {
    "document_id": "document-uuid",
    "filename": "contract.pdf",
    "pages": 3,
    "text_bytes": 12000,
    "markdown_bytes": 13000,
    "tables_count": 2,
    "chunks_count": 8,
    "raw_sha256": "64-char-hex-sha256",
    "text_sha256": "64-char-hex-sha256",
    "markdown_sha256": "64-char-hex-sha256",
    "tables_sha256": "64-char-hex-sha256",
    "last_parsed_at": "2026-05-02T12:01:00Z"
  }
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Status retornado |
| `400` | `documentID` invalido ou ausente |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV |
| `404` | Documento nao encontrado |

### GET `/documents/{documentID}/chunks`

Lista chunks processados de um documento. O endpoint exige permissao de leitura no escopo da JV dona do documento.

Query params:

| Nome | Obrigatorio | Default | Max | Descricao |
| --- | --- | --- | --- | --- |
| `limit` | Nao | `50` | `200` | Quantidade de chunks retornados |
| `offset` | Nao | `0` | - | Offset para paginacao |

Request:

```ts
const response = await apiFetch(`/documents/${documentId}/chunks?limit=50&offset=0`);
const page = (await response.json()) as DocumentChunksPage;
```

Resposta `200`:

```json
{
  "document_id": "document-uuid",
  "chunks": [
    {
      "id": "chunk-uuid",
      "document_id": "document-uuid",
      "index": 0,
      "content": "texto normalizado do chunk",
      "created_at": "2026-05-02T12:01:00Z"
    }
  ],
  "limit": 50,
  "offset": 0,
  "count": 1
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Chunks retornados, possivelmente lista vazia se o documento ainda nao foi parseado |
| `400` | `documentID`, `limit` ou `offset` invalido |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV |
| `404` | Documento nao encontrado |

### GET `/joint-ventures/{jvID}/documents`

Lista documentos de uma joint venture.

Request:

```ts
const response = await apiFetch(`/joint-ventures/${jvId}/documents`);
const documents = (await response.json()) as AuditDocument[];
```

Resposta `200`:

```json
[
  {
    "id": "document-uuid",
    "jv_id": "jv-uuid",
    "name": "contract.pdf",
    "type": "contract",
    "storage_key": "tenants/demo/regions/latam/jvs/jv-001/documents/doc-001/raw/contract.pdf",
    "uploaded_by": "user@example.com",
    "uploaded_at": "2026-05-02T12:00:00Z",
    "status": "queued",
    "processed": false
  }
]
```

Resposta vazia:

```json
[]
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Lista retornada |
| `400` | `jvID` ausente |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV |

### DELETE `/documents/delete?id=<document_id>`

Remove um documento por ID e registra evento de auditoria.

Request:

```ts
await apiFetch(`/documents/delete?id=${documentId}`, {
  method: "DELETE",
});
```

Resposta `200`:

```json
{
  "status": "deleted",
  "id": "document-uuid"
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Documento removido |
| `400` | `id` ausente |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV ou CSRF invalido |
| `404` | Documento nao encontrado |

## Upload Blob

Fluxo implementado hoje:

1. Usuario escolhe arquivo.
2. Frontend chama `POST /joint-ventures/{jvID}/documents/upload-url`.
3. Backend cria documento com `status: "upload_pending"` e retorna a URL assinada.
4. Frontend faz `PUT` direto no Azure Blob usando a URL assinada.
5. Frontend chama `POST /documents/{documentID}/upload-complete`.
6. Backend valida o blob, persiste metadados tecnicos e marca o documento como `queued`.
7. Worker calcula SHA-256 do arquivo bruto e dos artefatos parseados ao concluir o processamento.

### POST `/joint-ventures/{jvID}/documents/upload-url`

Cria um documento pendente e devolve o alvo para upload direto.

Body aceito:

```json
{
  "filename": "contract.pdf",
  "name": "contract.pdf",
  "type": "contract",
  "content_type": "application/pdf",
  "size_bytes": 123456
}
```

Observacoes:

- `filename` e o campo principal.
- se `filename` vier vazio, o backend usa `name` como fallback.
- `size_bytes` e opcional.

Request:

```ts
const response = await apiFetch(`/joint-ventures/${jvId}/documents/upload-url`, {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    filename: file.name,
    type: "contract",
    content_type: file.type || "application/octet-stream",
    size_bytes: file.size,
  }),
});

const payload = (await response.json()) as RequestDocumentUploadResponse;
```

Resposta `201`:

```json
{
  "document": {
    "id": "document-uuid",
    "jv_id": "jv-uuid",
    "name": "contract.pdf",
    "type": "contract",
    "storage_key": "jvs/{jv_id}/documents/{document_id}/raw/contract.pdf",
    "uploaded_by": "user@example.com",
    "uploaded_at": "2026-05-02T12:00:00Z",
    "status": "upload_pending",
    "processed": false
  },
  "upload": {
    "method": "PUT",
    "url": "https://account.blob.core.windows.net/container/key?sas=...",
    "headers": {
      "x-ms-blob-type": "BlockBlob",
      "Content-Type": "application/pdf",
      "x-ms-blob-content-type": "application/pdf"
    },
    "expires_at": "2026-05-02T12:15:00Z"
  }
}
```

Status:

| Status | Motivo |
| --- | --- |
| `201` | URL de upload criada |
| `400` | `jvID` invalido, filename invalido, tipo invalido ou `size_bytes` invalido |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV ou CSRF invalido |
| `503` | Storage Blob nao configurado |
| `500` | Erro interno |

### PUT direto na URL assinada do Blob

O upload binario nao passa pelo backend Go.

Use exatamente o metodo, a URL e os headers devolvidos em `upload`.

Exemplo:

```ts
await fetch(payload.upload.url, {
  method: payload.upload.method,
  headers: payload.upload.headers,
  body: file,
});
```

### POST `/documents/{documentID}/upload-complete`

Confirma o upload, valida o blob no storage e enfileira processamento.

Body opcional:

```json
{
  "size_bytes": 123456
}
```

Se `size_bytes` for enviado e nao bater com o blob real, o backend retorna erro.

Request:

```ts
const response = await apiFetch(`/documents/${documentId}/upload-complete`, {
  method: "POST",
  headers: {
    "Content-Type": "application/json",
  },
  body: JSON.stringify({
    size_bytes: file.size,
  }),
});

const document = (await response.json()) as AuditDocument;
```

Resposta `200`:

```json
{
  "id": "document-uuid",
  "jv_id": "jv-uuid",
  "name": "contract.pdf",
  "type": "contract",
  "storage_key": "jvs/{jv_id}/documents/{document_id}/raw/contract.pdf",
  "uploaded_by": "user@example.com",
  "uploaded_at": "2026-05-02T12:00:00Z",
  "status": "queued",
  "processed": false
}
```

Status:

| Status | Motivo |
| --- | --- |
| `200` | Upload confirmado e documento enfileirado |
| `400` | `documentID` invalido, body invalido, blob vazio ou tamanho divergente |
| `401` | Sem sessao valida |
| `403` | Sem permissao no escopo da JV ou CSRF invalido |
| `404` | Documento nao encontrado |
| `409` | Blob ainda nao encontrado no storage |
| `503` | Azure Blob Storage nao configurado no backend |
| `500` | Erro interno |

## Access, Regions e Joint Ventures

As secoes abaixo possuem handlers HTTP implementados no backend atual.

### Tipos

Membership:

```ts
export type Membership = {
  id: string;
  user_login: string;
  role: Role;
  scope_type: ScopeType;
  scope_id?: string;
  created_at: string;
};
```

Regioes e Joint Ventures:

```ts
export type Region = {
  id: string;
  code: string;
  name: string;
  created_at: string;
};

export type JointVentureStatus = "draft" | "active" | "suspended" | "closed";

export type JointVenture = {
  id: string;
  region_id: string;
  name: string;
  parties: string[];
  status: JointVentureStatus;
  created_by: string;
  created_at: string;
  updated_at: string;
  metadata?: Record<string, string>;
};
```

### Memberships

Todos os endpoints exigem autenticacao. Gerenciamento de `system` exige `admin` no escopo `system`; gerenciamento de `region` aceita administradores com acesso a essa regiao; gerenciamento de `joint_venture` aceita administradores com acesso herdado ou direto a essa JV.

```http
GET    /access/memberships?user_login=&scope_type=&scope_id=
POST   /access/memberships
DELETE /access/memberships/{membershipID}
```

POST body:

```json
{
  "user_login": "auditor@example.com",
  "role": "auditor",
  "scope_type": "joint_venture",
  "scope_id": "00000000-0000-0000-0000-000000000000"
}
```

Para `scope_type: "system"`, omita `scope_id`.

### Regioes

Todos os endpoints exigem autenticacao. Criar/deletar regiao exige `admin` em `system`; leitura e update usam permissao no escopo da regiao ou superior.

```http
GET    /regions
POST   /regions
GET    /regions/{regionID}
PATCH  /regions/{regionID}
DELETE /regions/{regionID}
```

POST/PATCH body:

```json
{
  "name": "Latin America",
  "code": "LATAM"
}
```

### Joint Ventures

Todos os endpoints exigem autenticacao. Acesso a JV aceita heranca de `system`, de `region` ou membership direto em `joint_venture`. A listagem por regiao retorna somente JVs visiveis para o usuario.

```http
GET    /regions/{regionID}/joint-ventures
POST   /regions/{regionID}/joint-ventures
GET    /joint-ventures/{jvID}
PATCH  /joint-ventures/{jvID}
DELETE /joint-ventures/{jvID}
```

POST body:

```json
{
  "name": "JV Alpha",
  "parties": ["Company A", "Company B"],
  "metadata": {
    "country": "BR"
  }
}
```

PATCH body aceita `name`, `parties`, `status` e `metadata`.

## Areas Planejadas

As secoes abaixo ainda nao possuem handlers HTTP implementados no backend atual.

### Prompt Management

Endpoints planejados:

```http
GET    /prompts
POST   /prompts
GET    /prompts/{promptID}
POST   /prompts/{promptID}/versions
GET    /prompts/{promptID}/versions
POST   /prompt-versions/{versionID}/test
POST   /prompt-versions/{versionID}/approve
POST   /prompt-versions/{versionID}/deprecate
GET    /prompt-runs?audit_run_id=
```

### Audit Runs e Findings

Tipos recomendados:

```ts
export type AuditRunStatus =
  | "requested"
  | "running"
  | "completed"
  | "failed"
  | "cancelled";

export type FindingSeverity = "low" | "medium" | "high" | "critical";

export type AuditRun = {
  id: string;
  jv_id: string;
  status: AuditRunStatus;
  requested_by: string;
  started_at?: string;
  completed_at?: string;
  failed_at?: string;
  error_message?: string;
  created_at: string;
};

export type AuditFinding = {
  id: string;
  audit_run_id: string;
  document_id: string;
  prompt_run_id?: string;
  category: string;
  severity: FindingSeverity;
  title: string;
  description: string;
  evidence: string;
  page_number?: number;
  chunk_id?: string;
  confidence?: number;
  source: "ai" | "rule" | "manual";
  created_at: string;
};
```
