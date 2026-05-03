# Development Guide — Audit Platform for Joint Ventures

Este documento consolida a direção técnica do projeto `audit-go` para desenvolvimento incremental da plataforma de auditoria de joint ventures.

A base atual já aponta para um **modular monolith em Go**, com autenticação via **Microsoft Entra**, autorização interna por escopo, metadados documentais, trilha de auditoria imutável e integração com um serviço Python interno para parsing, OCR, embeddings e workloads específicos de IA.

---

## 1. Decisão arquitetural

Antes de entrar nos detalhes, use esta convenção ao ler este documento:

- **Atual**: comportamento, estrutura ou componente já refletido no código atual.
- **Alvo**: direção arquitetural recomendada para as próximas iterações.

### Escolha principal

```text
Go modular monolith + Python internal service
```

Go deve ser o backend público e dono das regras principais:

- autenticação e sessão;
- autorização por região e joint venture;
- CRUD de regiões, JVs e documentos;
- orquestração de processamento;
- trilha de auditoria;
- gerenciamento de prompts;
- controle de status;
- persistência transacional.

Python deve ser usado como serviço interno para tarefas especializadas:

- OCR;
- parsing de PDF;
- parsing de Excel;
- extração textual;
- experimentos de auditoria;
- embeddings;
- testes de prompts;
- pipelines auxiliares de IA.

Regra importante:

```text
Frontend nunca chama Python diretamente.
```

Fluxo correto:

```text
Frontend -> Go API -> Go Worker -> Python Service
```

---

## 2. Visão geral da arquitetura

### Estado atual da arquitetura

Hoje, o desenho implementado pode ser resumido assim:

```text
Frontend / Client
  |
  | HTTPS + cookies
  v
Go API
  |-- auth/sessão/CSRF dentro da feature access
  |-- documents
  |-- audit
  |-- processing
  |
  | SQL
  v
PostgreSQL

Go Worker
  |
  | internal HTTP
  v
Python Service

Go API / Worker
  |
  | blobs e metadados
  v
Azure Blob Storage
```

### Arquitetura alvo

O diagrama abaixo representa a direção arquitetural desejada, não a fotografia completa do código atual:

```text
Client / Frontend
  |
  | HTTPS + cookies
  v
Go API / Modular Monolith
  |-- identity          Microsoft Entra, sessão, refresh, CSRF
  |-- access            roles, permissions, memberships, scopes
  |-- regions           boundaries regionais
  |-- jointventures     ciclo de vida das JVs
  |-- documents         metadados e autorização por JV
  |-- ingestion         upload, Azure Blob, enqueue processing
  |-- processing        jobs, outbox, filas, workers, Python integration
  |-- ocr               extração e OCR
  |-- indexing          chunks, embeddings, busca vetorial/híbrida
  |-- prompt            prompt management e versionamento
  |-- ai                client centralizado para LLM/embeddings
  |-- audit             audit runs, findings, audit events
  |-- report            relatórios finais
  |
  | SQL
  v
PostgreSQL + pgvector

Go API / Go Worker
  |
  | arquivos brutos e derivados
  v
Azure Blob Storage

Go API / Outbox Publisher
  |
  | mensagens assíncronas
  v
Azure Service Bus ou RabbitMQ
  |
  v
Go Worker
  |
  | internal HTTP
  v
Python Service
  |-- PDF parsing
  |-- Excel parsing
  |-- OCR
  |-- AI helpers
```

---

## 3. Estrutura recomendada do projeto

A estrutura atual já usa `internal/features`. Mantenha essa direção.

### Estrutura atual relevante

Hoje, a base existente está mais próxima deste recorte:

```text
internal/
  features/
    access/
    audit/
    documents/
    jointventures/
    processing/
    regions/

  platform/
    config/
    contextx/
    httpx/
    logger/
    origin/
    postgres/
    security/
    storage/

python-service/
  processors/
  ai/
```

### Estrutura alvo recomendada

O desenho abaixo representa como o projeto pode ser fatiado conforme as capacidades crescerem:

```text
cmd/
  audit/                 HTTP API entrypoint
  worker/                processing worker entrypoint

internal/
  features/
    identity/            Microsoft Entra, sessions, refresh, CSRF
    access/              roles, permissions, scopes, memberships
    audit/               audit runs, findings, immutable events
    documents/           document domain, use cases, HTTP, Postgres
    ingestion/           upload, storage, enqueue processing
    jointventures/       JV domain, lifecycle, region ownership
    processing/          jobs, outbox, Python client, workers
    regions/             region domain
    prompt/              prompt templates, versions, runs, approvals
    ai/                  LLM/embedding clients and response validation
    indexing/            chunks, embeddings, vector index
    reports/             report generation and exports

  platform/
    config/
    contextx/
    httpx/
    logger/
    origin/
    postgres/
    security/
    storage/
    queue/

python-service/
  main.py
  processors/
    pdf.py
    excel.py
    ocr.py
  ai/
    embeddings.py
    chat.py
  audit_engine/
    rules/
    evaluations/
    prompts_client/
```

Se preferir manter menos features no início, comece com:

```text
identity
access
regions
jointventures
documents
processing
audit
prompt
```

Depois extraia `ingestion`, `indexing`, `ai` e `reports` conforme crescer.

---

## 4. Autenticação

O modelo recomendado é BFF com Microsoft Entra.

Fluxo:

```text
1. Frontend redireciona para GET /auth/login
2. Go API redireciona para Microsoft Entra
3. Entra chama GET /auth/callback
4. Go API cria cookies da aplicação
5. Frontend chama APIs com credentials: include
```

Cookies da aplicação:

| Cookie | HttpOnly | Path | Uso |
| --- | --- | --- | --- |
| `audit_session` | sim | `/` | sessão opaca da aplicação |
| `audit_refresh` | sim | `/auth` | refresh token opaco e rotacionado |
| `audit_csrf` | não | `/` | double-submit CSRF token |

Defaults atuais do backend:

```text
SameSite=Lax
Secure controlado por configuração e obrigatório em produção
session TTL default = 15 minutos
refresh TTL default = 30 dias
```

Endpoints atuais/esperados:

```http
GET  /auth/login?return_url=
GET  /auth/callback
GET  /auth/me
POST /auth/refresh
POST /auth/logout
```

Para requests mutáveis com cookies, o frontend deve enviar:

```http
X-CSRF-Token: <valor do cookie audit_csrf>
```

Observações do estado atual:

```text
- POST /auth/refresh exige audit_refresh + CSRF válido
- audit_csrf é reemitido junto com login/refresh
- middleware de auth também aceita Authorization: Bearer para clientes não-browser
```

---

## 5. Autorização

Microsoft Entra autentica.

A aplicação autoriza.

```text
Entra = quem é o usuário
Go/PostgreSQL = o que o usuário pode fazer
```

### Roles atuais

```text
admin
region_admin
jv_admin
contributor
auditor
visitor
```

### Escopos

```text
system          acesso global
region          acesso a todas as JVs da região
joint_venture   acesso a uma JV específica
```

### Modelo recomendado

```sql
users
- id
- entra_object_id
- tenant_id
- email
- name
- status
- created_at
- updated_at

regions
- id
- code
- name

joint_ventures
- id
- region_id
- code
- name
- status
- created_at
- updated_at

roles
- id
- name
- description

permissions
- id
- code
- description

role_permissions
- role_id
- permission_id

memberships
- id
- user_id
- role_id
- scope_type
- scope_id
- created_at
```

### Permissões sugeridas

```text
region:create
region:read
region:update
region:delete

venture:create
venture:read
venture:update
venture:delete

document:create
document:read
document:delete
document:process

audit:start
audit:read
audit:approve
audit:export

prompt:create
prompt:update
prompt:test
prompt:approve
prompt:deprecate
prompt:read

report:generate
report:download

user:read
user:assign_role
user:remove_role
```

Não use apenas role hardcoded no handler. Use `permission + scope`.

Exemplo:

```text
Usuário com auditor em region:LATAM
  pode ler documentos de qualquer JV da LATAM
  não pode ler documentos da EMEA
```

---

## 6. Documentos

Endpoints atuais:

```http
POST   /documents
GET    /joint-ventures/{jvID}/documents
GET    /documents/get?id=
DELETE /documents/delete?id=
POST   /joint-ventures/{jvID}/documents/upload-url
POST   /documents/{documentID}/upload-complete
```

Contrato atual de criação:

```json
{
  "jv_id": "uuid",
  "name": "contract.pdf",
  "type": "contract",
  "storage_key": "jv/123/contract.pdf"
}
```

Tipos válidos:

```text
contract
financial
report
other
```

### Endpoints futuros recomendados

Os endpoints abaixo ainda são alvo de evolução, não parte do conjunto HTTP já implementado hoje:

```http
POST   /joint-ventures/{jvID}/documents
POST   /documents/{documentID}/process
GET    /documents/{documentID}/status
GET    /documents/{documentID}/pages
GET    /documents/{documentID}/chunks
```

### Status recomendado

```text
upload_pending
uploaded
registered
queued
processing
parsed
ocr_completed
indexed
failed
deleted
```

---

## 7. Storage e fila

Arquivo pesado não deve ir para o PostgreSQL. O banco guarda metadados, estado e rastreabilidade. O conteúdo binário fica em storage externo.

### Estado atual de storage e processamento

Hoje, o backend já opera com:

```text
PostgreSQL para metadados e trilha transacional
Azure Blob Storage para upload direto e verificação do blob
Go Worker + Python Service por HTTP interno
```

Ainda não faz parte do estado atual um barramento externo de mensagens já integrado em produção/local.

### Storage recomendado

```text
Azure Blob Storage
```

Use o Blob para:

```text
raw files          arquivos originais enviados pelo usuário
normalized text    texto extraído ou markdown normalizado
tables             tabelas extraídas em JSON/CSV/Parquet
reports            relatórios gerados
temp               artefatos temporários de processamento
```

Estrutura sugerida de paths:

```text
tenants/{tenant_id}/regions/{region_id}/jvs/{jv_id}/documents/{document_id}/raw/{filename}
tenants/{tenant_id}/regions/{region_id}/jvs/{jv_id}/documents/{document_id}/parsed/text.md
tenants/{tenant_id}/regions/{region_id}/jvs/{jv_id}/documents/{document_id}/parsed/tables.json
tenants/{tenant_id}/regions/{region_id}/jvs/{jv_id}/audits/{audit_run_id}/reports/report.pdf
```

Para MVP, o `storage_key` atual pode continuar existindo. Só padronize que ele aponta para um objeto no Blob.

### Tabela de objetos armazenados

```sql
storage_objects
- id
- owner_type          -- document, audit_run, report
- owner_id
- bucket              -- ou container
- storage_key
- filename
- content_type
- size_bytes
- checksum_sha256
- kind                -- raw, parsed_text, parsed_table, report, temp
- created_by
- created_at
```

### Upload recomendado

No frontend, prefira upload direto para Blob com URL assinada/SAS gerada pelo backend.

Fluxo:

```text
POST /joint-ventures/{jvID}/documents/upload-url
  -> Go valida permissão document:create no escopo da JV
  -> Go cria document com status upload_pending
  -> Go gera SAS/upload URL
  -> frontend faz PUT direto no Azure Blob
  -> frontend confirma upload
  -> Go valida o blob, persiste metadados técnicos e marca queued
  -> se processing estiver ativo, Go grava outbox event e job de parse
```

Endpoints atuais/sugeridos:

```http
POST /joint-ventures/{jvID}/documents/upload-url
POST /documents/{documentID}/upload-complete
POST /documents/{documentID}/process
```

### Fila recomendada

As opções abaixo descrevem a arquitetura alvo para mensageria, não um componente já consolidado no código atual.

Para produção em Azure:

```text
Azure Service Bus
```

Para ambiente local/MVP simples:

```text
PostgreSQL outbox + processing_jobs
```

Alternativa comum fora de Azure:

```text
RabbitMQ
```

Use fila para:

```text
DocumentUploaded
DocumentProcessingRequested
DocumentParseRequested
DocumentParsed
DocumentIndexingRequested
DocumentIndexed
AuditRequested
AuditCompleted
AuditFailed
ReportRequested
ReportGenerated
```

### Por que não processar direto no request

Porque OCR, parsing, embeddings e auditoria podem levar segundos ou minutos. Request HTTP deve apenas registrar intenção, persistir estado e enfileirar trabalho.

### Outbox pattern

Para evitar inconsistência entre banco e fila, use outbox.

Observação:

```text
Outbox e processing_jobs aparecem aqui como direção de implementação.
O código atual já registra artefatos de processamento no fluxo documental, mas este desenho ainda deve ser tratado como arquitetura alvo até a camada de publicação/consumo estar fechada ponta a ponta.
```

Fluxo:

```text
transação PostgreSQL:
  - cria/atualiza document
  - grava audit_event
  - grava outbox_event

outbox publisher:
  - lê eventos pending
  - publica na fila
  - marca como published

worker:
  - consome mensagem
  - executa etapa
  - atualiza estado
  - grava próximo outbox_event
```

Tabelas:

```sql
outbox_events
- id
- event_type
- aggregate_type
- aggregate_id
- payload
- status              -- pending, published, failed
- attempts
- last_error
- created_at
- published_at

processing_jobs
- id
- job_type
- aggregate_type
- aggregate_id
- status              -- queued, running, completed, failed, dead_letter
- payload
- idempotency_key
- attempts
- max_attempts
- available_at
- locked_by
- locked_until
- last_error
- created_at
- updated_at
```

### Idempotência

Todo job precisa ser reexecutável sem duplicar dados.

Exemplo:

```text
parse_document:{document_id}:{document_version}
index_document:{document_id}:{parsed_version}
audit_run:{audit_run_id}:{ruleset_version}
```

Antes de processar:

```text
se document.status == indexed, não indexar de novo
se chunk já existe para parsed_version, não duplicar
se audit_run.status == completed, não rodar novamente
```

### Retries e dead-letter

Estados mínimos:

```text
queued
running
retry_scheduled
completed
failed
dead_letter
```

Regras:

```text
falha transitória: retry com backoff
falha definitiva: failed/dead_letter
OCR timeout: retry
arquivo inválido: failed sem retry infinito
permissão/storage ausente: failed e audit_event
```

---

## 8. Processamento assíncrono

Não processe arquivo pesado dentro do request HTTP.

### Estado atual do processamento assíncrono

Hoje, o sistema já separa o request HTTP do upload binário e já possui base para worker e integração com Python, mas o pipeline completo de filas, publicação e consumo ainda está em evolução.

### Fluxo alvo

Fluxo correto:

```text
POST /documents
  -> valida permissão
  -> cria document metadata
  -> registra audit_event
  -> cria processing_job ou outbox_event
  -> retorna 201

worker
  -> consome job
  -> chama Python service
  -> persiste texto/tabelas/chunks
  -> atualiza status
  -> registra audit_event
```

### Tabelas sugeridas

```sql
processing_jobs
- id
- type
- status
- payload
- attempts
- last_error
- available_at
- created_at
- updated_at

outbox_events
- id
- event_type
- aggregate_type
- aggregate_id
- payload
- status
- created_at
- published_at
```

Para MVP, pode começar com `processing_jobs` em PostgreSQL.

Depois, se precisar, troque para:

```text
RabbitMQ
Azure Service Bus
Kafka
```

---

## 9. Python Service

Serviço interno.

Endpoints existentes/esperados:

```http
GET  /health
POST /parse
```

Entradas suportadas inicialmente:

```text
.pdf
.xlsx
```

Resposta normalizada:

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

### Regra de segurança

Python não decide permissão.

Go valida:

```text
usuário
sessão
CSRF
role
scope
JV
região
```

Python recebe somente jobs internos já autorizados.

---

## 10. Prompt Management

Prompt deve ser entidade versionada, não string hardcoded.

Feature:

```text
internal/features/prompt
```

### Tabelas recomendadas

```sql
prompts
- id
- key
- name
- description
- category
- status
- created_by
- created_at
- updated_at

prompt_versions
- id
- prompt_id
- version
- system_prompt
- user_prompt_template
- model
- temperature
- top_p
- max_tokens
- response_schema
- status
- created_by
- approved_by
- created_at
- approved_at

prompt_variables
- id
- prompt_version_id
- name
- required
- type
- description

prompt_runs
- id
- prompt_version_id
- audit_run_id
- document_id
- chunk_id
- model
- input_hash
- output_hash
- input_tokens
- output_tokens
- latency_ms
- cost_estimate
- created_at

prompt_run_outputs
- id
- prompt_run_id
- raw_output
- parsed_output
- error_message
```

### Status de prompt

```text
draft
testing
approved
deprecated
archived
```

Produção deve usar apenas:

```text
approved
```

### Endpoints sugeridos

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

### Regra de auditoria

Todo finding gerado por IA deve apontar para:

```text
prompt_run_id
prompt_version_id
model
input hash
output hash
evidence
```

---

## 11. Audit Runs e Findings

A auditoria deve ser rastreável e baseada em evidência.

### Tabelas sugeridas de auditoria

```sql
audit_runs
- id
- jv_id
- status
- requested_by
- started_at
- completed_at
- failed_at
- error_message
- created_at

 audit_findings
- id
- audit_run_id
- document_id
- prompt_run_id
- category
- severity
- title
- description
- evidence
- page_number
- chunk_id
- confidence
- source
- created_at
```

### Status de audit_run

```text
requested
running
completed
failed
cancelled
```

### Severidades

```text
low
medium
high
critical
```

### Endpoints recomendados

```http
POST /joint-ventures/{jvID}/audits
GET  /joint-ventures/{jvID}/audits
GET  /audits/{auditRunID}
GET  /audits/{auditRunID}/findings
POST /audits/{auditRunID}/approve
POST /audits/{auditRunID}/cancel
```

---

## 12. Audit Events

Eventos de auditoria devem ser imutáveis.

Exemplos:

```text
user.logged_in
document.created
document.deleted
document.processing_started
document.processing_failed
document.processing_completed
prompt.created
prompt.version_approved
audit.started
audit.finding_created
audit.completed
membership.assigned
membership.removed
```

Regra:

```text
mutação de dado + audit_event devem acontecer na mesma transação
```

---

## 13. Ordem de desenvolvimento recomendada

### Fase 1 — Base sólida

- [ ] Validar migrations atuais.
- [x] Garantir `/health`.
- [x] Finalizar BFF com Entra.
- [x] Implementar cookies `audit_session`, `audit_refresh`, `audit_csrf`.
- [x] Implementar CSRF middleware.
- [x] Implementar `/auth/me`, `/auth/refresh`, `/auth/logout`.

### Fase 2 — Autorização real

- [x] Criar roles e permissions.
- [x] Criar memberships por `system`, `region`, `joint_venture`.
- [x] Criar middleware `RequirePermission`.
- [x] Proteger documents por escopo da JV.
- [x] Criar endpoints de memberships.

### Fase 3 — Região e JV

- [x] CRUD de regiões.
- [x] CRUD de joint ventures.
- [x] Listagem de JVs por região.
- [x] Validação de acesso herdado por região.

### Fase 4 — Documentos, Blob e ingestão

- [x] Listar documentos por JV.
- [x] Criar upload URL/SAS para Azure Blob.
- [x] Criar confirmação de upload.
- [x] Persistir `storage_objects`.
- [x] Persistir checksum SHA-256.
- [x] Expor status dedicado de processamento além do campo `document.status`.
- [x] Criar endpoint de consulta de chunks processados.
- [ ] Estruturar extração/consulta de páginas processadas.
- [x] Registrar audit events transacionais.

### Fase 5 — Processing, fila e outbox

- [x] Criar `outbox_events`.
- [x] Criar `processing_jobs`.
- [x] Criar outbox publisher.
- [ ] Integrar Azure Service Bus ou RabbitMQ.
- [x] Criar worker Go idempotente.
- [x] Chamar Python `/parse`.
- [x] Persistir texto extraído no Blob e/ou Postgres.
- [x] Persistir tabelas normalizadas.
- [x] Marcar documento como processado ou failed.
- [x] Implementar retries, backoff e dead-letter.

### Fase 6 — Prompt Management

- [ ] Criar `prompts`.
- [ ] Criar `prompt_versions`.
- [ ] Criar aprovação de versão.
- [ ] Criar teste de prompt.
- [ ] Criar `prompt_runs`.
- [ ] Bloquear execução de prompt não aprovado em produção.

### Fase 7 — Auditoria com IA

- [ ] Criar `audit_runs`.
- [ ] Criar `audit_findings`.
- [ ] Rodar auditoria por JV.
- [ ] Integrar prompts aprovados.
- [ ] Salvar evidências por documento/página/chunk.
- [ ] Expor findings no frontend.

### Fase 8 — Relatórios

- [ ] Gerar relatório por audit_run.
- [ ] Exportar PDF/Markdown/Excel.
- [ ] Incluir findings, evidências e trilha de prompt.

---

## 14. Convenções de código

### Feature-based package

Cada feature deve seguir este formato:

```text
internal/features/<feature>/
  domain/
  application/
  infra/
  http/
```

Exemplo:

```text
internal/features/documents/
  domain/
    document.go
    status.go
  application/
    create_document.go
    get_document.go
    delete_document.go
  infra/
    postgres_document_repository.go
  http/
    handler.go
    routes.go
```

### Não fazer

Evite:

```text
controllers/
services/
repositories/
models/
```

Isso tende a virar separação por camada global, não por capacidade de negócio.

### Regra entre features

Uma feature não deve acessar diretamente a infra da outra.

Errado:

```text
audit -> documents/infra/PostgresDocumentRepository
```

Certo:

```text
audit -> interface DocumentReader
composition root injeta implementação concreta
```

---

## 15. Contratos importantes

### Error format

Formato JSON padrão:

```json
{
  "error": "message"
}
```

Alguns middlewares, como CSRF, podem retornar `text/plain` temporariamente. Idealmente, padronizar tudo em JSON.

### Request context mínimo

O contexto de request deve carregar:

```text
request_id
user_id
user_email
tenant_id
roles/memberships resolvidas ou lazy-loaded
```

### Logs

Todo log relevante deve conter:

```text
request_id
user_id
jv_id quando houver
document_id quando houver
audit_run_id quando houver
```

---

## 16. Decisões fixadas

- Go é o backend público.
- Python é interno.
- Frontend usa cookies e `credentials: include`.
- Access token Microsoft não deve ficar armazenado no frontend.
- Microsoft Entra autentica.
- Go/PostgreSQL autoriza.
- Permissão deve ser por role + scope.
- Scope pode ser `system`, `region` ou `joint_venture`.
- Prompt deve ser versionado e auditável.
- Finding de IA precisa ter evidência.
- Processamento pesado deve ser assíncrono.
- Arquivo binário deve ficar no Azure Blob Storage.
- PostgreSQL guarda metadados, estado, trilha e referências ao Blob.
- Fila deve ser Azure Service Bus, RabbitMQ ou Postgres outbox no MVP.
- Use outbox para garantir consistência entre transação e publicação de evento.
- Jobs precisam ser idempotentes e tolerantes a retry.
- Mutação importante precisa gerar audit event.

---

## 17. Backlog técnico imediato

```text
[auth]
- Padronizar erros de middleware em JSON.
- Endurecer escopo do refresh cookie se o logout deixar de depender de `/auth`.

[access]
- Criar tabela de permissions.
- Mapear role_permissions.
- Criar membership scoped.
- Implementar RequirePermission.

[regions/jv]
- Criar endpoints CRUD.
- Validar hierarquia region -> JV.

[documents/storage]
- Expor status dedicado de processamento.
- Estruturar extração/consulta de páginas processadas.

[queue/processing]
- Criar outbox_events.
- Criar processing_jobs.
- Integrar Azure Service Bus ou RabbitMQ.
- Criar worker.
- Implementar retry/dead-letter.
- Integrar Python /parse.

[prompt]
- Criar prompts.
- Criar prompt_versions.
- Criar approval flow.
- Criar prompt_runs.

[audit]
- Criar audit_runs.
- Criar findings.
- Vincular finding com prompt_run.
```

---

## 18. Meta do MVP

O MVP deve permitir:

```text
1. Usuário logar com Microsoft Entra.
2. Backend criar sessão segura via cookies.
3. Admin cadastrar região.
4. Admin cadastrar joint venture.
5. Admin atribuir usuário a uma JV ou região.
6. Usuário autorizado solicitar upload de documento.
7. Backend gerar upload URL/SAS para Azure Blob.
8. Frontend subir arquivo no Blob e confirmar upload.
9. Backend criar outbox_event/job de processamento.
10. Worker consumir fila/job e processar documento via Python.
11. Sistema registrar audit events.
12. Admin criar prompt versionado.
13. Admin aprovar prompt.
14. Auditor iniciar audit_run.
15. Sistema gerar findings com evidência.
16. Frontend listar resultados por JV.
```

Esse MVP já valida a arquitetura inteira sem precisar escalar prematuramente.
