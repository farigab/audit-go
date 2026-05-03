package app

import (
	"context"
	"errors"
	"testing"
	"time"

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

func TestRequestDocumentUploadCreatesPendingDocumentAndUploadURL(t *testing.T) {
	repo := &fakeDocumentRepository{}
	auditRepo := &fakeAuditRepository{}
	storageRepo := &fakeStorageRepository{}
	auth := &fakeAuthorizer{}
	blobGateway := &fakeBlobGateway{}
	uc := RequestDocumentUploadUseCase{
		DocRepo:      repo,
		AuditRepo:    auditRepo,
		StorageRepo:  storageRepo,
		BlobGateway:  blobGateway,
		Authorizer:   auth,
		Transactor:   fakeTransactor{},
		UploadURLTTL: 15,
	}

	output, err := uc.Execute(context.Background(), access.Principal{Login: "contributor@example.com"}, RequestDocumentUploadInput{
		JVID:        validJVID,
		RequestID:   "00000000-0000-0000-0000-000000000010",
		Filename:    "../contract.pdf",
		Type:        documents.TypeContract,
		ContentType: "application/pdf",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if output.Document.Status != documents.StatusUploadPending {
		t.Fatalf("expected upload_pending status, got %q", output.Document.Status)
	}
	if output.Document.Name != "contract.pdf" {
		t.Fatalf("expected sanitized filename, got %q", output.Document.Name)
	}
	if blobGateway.uploadStorageKey != output.Document.StorageKey {
		t.Fatalf("expected upload URL for storage key %q, got %q", output.Document.StorageKey, blobGateway.uploadStorageKey)
	}
	if output.Upload.Method != "PUT" || output.Upload.URL == "" {
		t.Fatalf("expected upload target, got %#v", output.Upload)
	}
	if storageRepo.savedObject == nil || storageRepo.savedObject.Container != "documents" {
		t.Fatalf("expected pending storage metadata, got %#v", storageRepo.savedObject)
	}
	if !auditRepo.saved {
		t.Fatal("expected audit event to be saved")
	}
	if auth.permission != access.PermissionDocumentCreate {
		t.Fatalf("expected document create permission, got %q", auth.permission)
	}
}

func TestCompleteDocumentUploadVerifiesBlobAndQueuesProcessing(t *testing.T) {
	doc := documents.Document{
		ID:         "00000000-0000-0000-0000-000000000002",
		JVID:       validJVID,
		Name:       "contract.pdf",
		Type:       documents.TypeContract,
		StorageKey: "jvs/" + validJVID + "/documents/00000000-0000-0000-0000-000000000002/raw/contract.pdf",
		UploadedBy: "contributor@example.com",
		Status:     documents.StatusUploadPending,
	}
	repo := &fakeDocumentRepository{findDoc: &doc}
	auditRepo := &fakeAuditRepository{}
	storageRepo := &fakeStorageRepository{}
	processingRepo := &fakeProcessingRepository{}
	auth := &fakeAuthorizer{}
	blobGateway := &fakeBlobGateway{
		props: storage.BlobProperties{
			Container:   "documents",
			ContentType: "application/pdf",
			SizeBytes:   1234,
			ETag:        `"etag"`,
			VersionID:   "version-1",
		},
	}
	uc := CompleteDocumentUploadUseCase{
		DocRepo:        repo,
		AuditRepo:      auditRepo,
		StorageRepo:    storageRepo,
		ProcessingRepo: processingRepo,
		BlobGateway:    blobGateway,
		Authorizer:     auth,
		Transactor:     fakeTransactor{},
	}
	size := int64(1234)

	completed, err := uc.Execute(context.Background(), access.Principal{Login: "contributor@example.com"}, CompleteDocumentUploadInput{
		DocumentID: doc.ID,
		RequestID:  "00000000-0000-0000-0000-000000000010",
		SizeBytes:  &size,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if completed.Status != documents.StatusQueued {
		t.Fatalf("expected queued status, got %q", completed.Status)
	}
	if repo.savedDoc == nil || repo.savedDoc.Status != documents.StatusQueued {
		t.Fatalf("expected queued document to be saved, got %#v", repo.savedDoc)
	}
	if storageRepo.savedObject == nil || storageRepo.savedObject.ETag != `"etag"` {
		t.Fatalf("expected verified storage metadata, got %#v", storageRepo.savedObject)
	}
	if storageRepo.savedObject.SizeBytes == nil || *storageRepo.savedObject.SizeBytes != 1234 {
		t.Fatalf("expected verified size, got %#v", storageRepo.savedObject.SizeBytes)
	}
	if len(processingRepo.jobs) != 1 {
		t.Fatalf("expected processing job, got %#v", processingRepo.jobs)
	}
	if !auditRepo.saved {
		t.Fatal("expected audit event to be saved")
	}
}

func TestCompleteDocumentUploadRejectsSizeMismatch(t *testing.T) {
	doc := documents.Document{
		ID:         "00000000-0000-0000-0000-000000000002",
		JVID:       validJVID,
		Name:       "contract.pdf",
		StorageKey: "jvs/" + validJVID + "/documents/00000000-0000-0000-0000-000000000002/raw/contract.pdf",
		Status:     documents.StatusUploadPending,
	}
	repo := &fakeDocumentRepository{findDoc: &doc}
	blobGateway := &fakeBlobGateway{
		props: storage.BlobProperties{
			Container: "documents",
			SizeBytes: 10,
		},
	}
	uc := CompleteDocumentUploadUseCase{
		DocRepo:     repo,
		BlobGateway: blobGateway,
		Authorizer:  &fakeAuthorizer{},
		Transactor:  fakeTransactor{},
	}
	size := int64(11)

	_, err := uc.Execute(context.Background(), access.Principal{Login: "contributor@example.com"}, CompleteDocumentUploadInput{
		DocumentID: doc.ID,
		RequestID:  "00000000-0000-0000-0000-000000000010",
		SizeBytes:  &size,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
	if repo.savedDoc != nil {
		t.Fatalf("did not expect document save, got %#v", repo.savedDoc)
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

func TestGetDocumentProcessingStatusReturnsJobAndParseSummary(t *testing.T) {
	doc := documents.Document{
		ID:         "00000000-0000-0000-0000-000000000002",
		JVID:       validJVID,
		Name:       "contract.pdf",
		Status:     documents.StatusParsed,
		Processed:  true,
		UploadedBy: "contributor@example.com",
	}
	updatedAt := time.Now().UTC()
	parsedAt := updatedAt.Add(-time.Minute)
	job := processing.Job{
		ID:          "00000000-0000-0000-0000-000000000020",
		JobType:     processing.JobTypeParseDocument,
		Status:      processing.JobCompleted,
		Attempts:    1,
		MaxAttempts: 5,
		AvailableAt: updatedAt,
		UpdatedAt:   updatedAt,
	}
	summary := processing.ParseResultSummary{
		DocumentID:    doc.ID,
		Filename:      doc.Name,
		Pages:         3,
		TextBytes:     100,
		MarkdownBytes: 120,
		TablesCount:   2,
		ChunksCount:   4,
		LastParsedAt:  &parsedAt,
	}
	repo := &fakeDocumentRepository{findDoc: &doc}
	processingRepo := &fakeProcessingRepository{latestJob: &job, parseSummary: &summary}
	auth := &fakeAuthorizer{}
	uc := GetDocumentProcessingStatusUseCase{
		DocRepo:        repo,
		ProcessingRepo: processingRepo,
		Authorizer:     auth,
	}

	status, err := uc.Execute(context.Background(), access.Principal{Login: "auditor@example.com"}, doc.ID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.permission != access.PermissionDocumentRead {
		t.Fatalf("expected document read permission, got %q", auth.permission)
	}
	if status.Document.ID != doc.ID || status.Document.Status != documents.StatusParsed {
		t.Fatalf("unexpected document status: %#v", status.Document)
	}
	if status.Job == nil || status.Job.Status != processing.JobCompleted {
		t.Fatalf("expected completed job status, got %#v", status.Job)
	}
	if status.ParseResult == nil || status.ParseResult.ChunksCount != 4 {
		t.Fatalf("expected parse summary, got %#v", status.ParseResult)
	}
}

type fakeDocumentRepository struct {
	docs       []documents.Document
	findDoc    *documents.Document
	findErr    error
	findByJVID string
	savedDoc   *documents.Document
}

func (f *fakeDocumentRepository) Save(_ context.Context, doc documents.Document) error {
	f.savedDoc = &doc
	return nil
}

func (f *fakeDocumentRepository) FindByID(context.Context, string) (*documents.Document, error) {
	if f.findErr != nil {
		return nil, f.findErr
	}
	return f.findDoc, nil
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

type fakeBlobGateway struct {
	uploadStorageKey string
	props            storage.BlobProperties
	propsErr         error
}

func (f *fakeBlobGateway) ContainerName() string {
	return "documents"
}

func (f *fakeBlobGateway) CreateUploadURL(
	_ context.Context,
	storageKey string,
	_ string,
	expiresAt time.Time,
) (storage.UploadURL, error) {
	f.uploadStorageKey = storageKey
	return storage.UploadURL{
		Method:    "PUT",
		URL:       "https://example.blob.core.windows.net/documents/" + storageKey,
		Headers:   map[string]string{"x-ms-blob-type": "BlockBlob"},
		ExpiresAt: expiresAt,
		Container: "documents",
	}, nil
}

func (f *fakeBlobGateway) GetProperties(_ context.Context, storageKey string) (storage.BlobProperties, error) {
	if f.propsErr != nil {
		return storage.BlobProperties{}, f.propsErr
	}
	props := f.props
	props.StorageKey = storageKey
	return props, nil
}

type fakeProcessingRepository struct {
	events       []processing.OutboxEvent
	jobs         []processing.Job
	latestJob    *processing.Job
	parseSummary *processing.ParseResultSummary
}

func (f *fakeProcessingRepository) SaveOutboxEvent(_ context.Context, event processing.OutboxEvent) error {
	f.events = append(f.events, event)
	return nil
}

func (f *fakeProcessingRepository) SaveJob(_ context.Context, job processing.Job) error {
	f.jobs = append(f.jobs, job)
	return nil
}

func (f *fakeProcessingRepository) FindLatestJobByAggregate(
	context.Context,
	string,
	string,
) (*processing.Job, error) {
	return f.latestJob, nil
}

func (f *fakeProcessingRepository) FindParseResultSummary(
	context.Context,
	string,
) (*processing.ParseResultSummary, error) {
	return f.parseSummary, nil
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
