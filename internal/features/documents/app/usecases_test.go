package app

import (
	"context"
	"errors"
	"testing"

	"audit-go/internal/features/access"
	"audit-go/internal/features/audit"
	"audit-go/internal/features/documents"
	"audit-go/internal/features/processing"
	"audit-go/internal/platform/storage"
)

const validJVID = "00000000-0000-0000-0000-000000000001"

func TestCreateDocumentWritesStorageOutboxAndJob(t *testing.T) {
	repo := &fakeDocumentRepository{}
	auditRepo := &fakeAuditRepository{}
	storageRepo := &fakeStorageRepository{}
	processingRepo := &fakeProcessingRepository{}
	auth := &fakeAuthorizer{}
	uc := CreateDocumentUseCase{
		DocRepo:        repo,
		AuditRepo:      auditRepo,
		StorageRepo:    storageRepo,
		ProcessingRepo: processingRepo,
		Authorizer:     auth,
		Transactor:     fakeTransactor{},
	}

	doc, err := uc.Execute(context.Background(), access.Principal{Login: "contributor@example.com"}, CreateDocumentInput{
		JVID:       validJVID,
		RequestID:  "00000000-0000-0000-0000-000000000010",
		Name:       "contract.pdf",
		Type:       documents.TypeContract,
		StorageKey: "tenants/t1/regions/r1/jvs/j1/documents/d1/raw/contract.pdf",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if doc.Status != documents.StatusQueued {
		t.Fatalf("expected queued status, got %q", doc.Status)
	}
	if repo.savedDoc == nil || repo.savedDoc.ID != doc.ID {
		t.Fatalf("expected document to be saved, got %#v", repo.savedDoc)
	}
	if storageRepo.savedObject == nil || storageRepo.savedObject.OwnerID != doc.ID {
		t.Fatalf("expected storage metadata for document, got %#v", storageRepo.savedObject)
	}
	if storageRepo.savedObject.Kind != storage.KindRaw {
		t.Fatalf("expected raw storage object, got %q", storageRepo.savedObject.Kind)
	}
	if len(processingRepo.events) != 1 || processingRepo.events[0].EventType != processing.EventDocumentUploaded {
		t.Fatalf("expected DocumentUploaded outbox event, got %#v", processingRepo.events)
	}
	if len(processingRepo.jobs) != 1 {
		t.Fatalf("expected one processing job, got %#v", processingRepo.jobs)
	}
	if want := "parse_document:" + doc.ID + ":v1"; processingRepo.jobs[0].IdempotencyKey != want {
		t.Fatalf("expected idempotency key %q, got %q", want, processingRepo.jobs[0].IdempotencyKey)
	}
	if !auditRepo.saved {
		t.Fatal("expected audit event to be saved")
	}
}

func TestListDocumentsByJVAuthorizesReadAndReturnsDocuments(t *testing.T) {
	repo := &fakeDocumentRepository{
		docs: []documents.Document{
			{ID: "00000000-0000-0000-0000-000000000002", JVID: validJVID, Name: "contract.pdf"},
		},
	}
	auth := &fakeAuthorizer{}
	uc := ListDocumentsByJVUseCase{DocRepo: repo, Authorizer: auth}

	docs, err := uc.Execute(context.Background(), access.Principal{Login: "auditor@example.com"}, validJVID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.jvID != validJVID {
		t.Fatalf("expected authorizer jv id %q, got %q", validJVID, auth.jvID)
	}
	if auth.permission != access.PermissionDocumentRead {
		t.Fatalf("expected document read permission, got %q", auth.permission)
	}
	if repo.findByJVID != validJVID {
		t.Fatalf("expected repository jv id %q, got %q", validJVID, repo.findByJVID)
	}
	if len(docs) != 1 || docs[0].Name != "contract.pdf" {
		t.Fatalf("unexpected documents: %#v", docs)
	}
}

func TestListDocumentsByJVRejectsInvalidJVID(t *testing.T) {
	repo := &fakeDocumentRepository{}
	auth := &fakeAuthorizer{}
	uc := ListDocumentsByJVUseCase{DocRepo: repo, Authorizer: auth}

	_, err := uc.Execute(context.Background(), access.Principal{Login: "auditor@example.com"}, "not-a-uuid")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if auth.called {
		t.Fatal("expected invalid input to fail before authorization")
	}
}

func TestListDocumentsByJVReturnsEmptySlice(t *testing.T) {
	repo := &fakeDocumentRepository{}
	auth := &fakeAuthorizer{}
	uc := ListDocumentsByJVUseCase{DocRepo: repo, Authorizer: auth}

	docs, err := uc.Execute(context.Background(), access.Principal{Login: "visitor@example.com"}, validJVID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if docs == nil {
		t.Fatal("expected an empty slice, got nil")
	}
	if len(docs) != 0 {
		t.Fatalf("expected no documents, got %d", len(docs))
	}
}

type fakeDocumentRepository struct {
	docs       []documents.Document
	findByJVID string
	savedDoc   *documents.Document
}

func (f *fakeDocumentRepository) Save(_ context.Context, doc documents.Document) error {
	f.savedDoc = &doc
	return nil
}

func (f *fakeDocumentRepository) FindByID(context.Context, string) (*documents.Document, error) {
	return nil, nil
}

func (f *fakeDocumentRepository) FindByJVID(_ context.Context, jvID string) ([]documents.Document, error) {
	f.findByJVID = jvID
	return f.docs, nil
}

func (f *fakeDocumentRepository) Delete(context.Context, string) error {
	return nil
}

type fakeAuditRepository struct {
	saved bool
}

func (f *fakeAuditRepository) Save(context.Context, audit.Event) error {
	f.saved = true
	return nil
}

func (fakeTransactor) WithinTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

type fakeTransactor struct{}

type fakeStorageRepository struct {
	savedObject *storage.Object
}

func (f *fakeStorageRepository) Save(_ context.Context, object storage.Object) error {
	f.savedObject = &object
	return nil
}

type fakeProcessingRepository struct {
	events []processing.OutboxEvent
	jobs   []processing.Job
}

func (f *fakeProcessingRepository) SaveOutboxEvent(_ context.Context, event processing.OutboxEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeProcessingRepository) SaveJob(_ context.Context, job processing.Job) error {
	f.jobs = append(f.jobs, job)
	return nil
}

type fakeAuthorizer struct {
	called     bool
	jvID       string
	permission access.Permission
	err        error
}

func (f *fakeAuthorizer) CanAccessJV(
	_ context.Context,
	_ access.Principal,
	jvID string,
	permission access.Permission,
) error {
	f.called = true
	f.jvID = jvID
	f.permission = permission
	return f.err
}
