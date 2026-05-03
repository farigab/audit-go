// Package app implements document application use cases.
package app

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/features/audit"
	"audit-go/internal/features/documents"
	"audit-go/internal/features/processing"
	"audit-go/internal/platform/storage"
)

const (
	defaultDocumentContainer  = "documents"
	defaultDocumentChunkLimit = 50
	maxDocumentChunkLimit     = 200
)

var (
	ErrInvalidInput         = errors.New("documents: invalid input")
	ErrNotFound             = errors.New("documents: not found")
	ErrStorageNotConfigured = errors.New("documents: storage not configured")
	ErrBlobNotFound         = errors.New("documents: uploaded blob not found")
)

type documentRepository interface {
	Save(ctx context.Context, doc documents.Document) error
	FindByID(ctx context.Context, id string) (*documents.Document, error)
	FindByJVID(ctx context.Context, jvID string) ([]documents.Document, error)
	Delete(ctx context.Context, id string) error
}

type auditRepository interface {
	Save(ctx context.Context, event audit.Event) error
}

type storageRepository interface {
	Save(ctx context.Context, object storage.Object) error
}

type blobGateway interface {
	ContainerName() string
	CreateUploadURL(ctx context.Context, storageKey string, contentType string, expiresAt time.Time) (storage.UploadURL, error)
	GetProperties(ctx context.Context, storageKey string) (storage.BlobProperties, error)
}

type processingRepository interface {
	SaveOutboxEvent(ctx context.Context, event processing.OutboxEvent) error
	SaveJob(ctx context.Context, job processing.Job) error
}

type processingStatusRepository interface {
	FindLatestJobByAggregate(ctx context.Context, aggregateType string, aggregateID string) (*processing.Job, error)
	FindParseResultSummary(ctx context.Context, documentID string) (*processing.ParseResultSummary, error)
}

type documentChunksRepository interface {
	ListDocumentChunks(ctx context.Context, documentID string, limit int, offset int) ([]processing.DocumentChunkRecord, error)
}

type authorizer interface {
	CanAccessJV(ctx context.Context, principal access.Principal, jvID string, permission access.Permission) error
}

type transactor interface {
	WithinTx(ctx context.Context, fn func(context.Context) error) error
}

// DocumentProcessingStatus describes the current processing state of a document.
type DocumentProcessingStatus struct {
	Document    documents.Document             `json:"document"`
	Job         *DocumentProcessingJobStatus   `json:"job,omitempty"`
	ParseResult *processing.ParseResultSummary `json:"parse_result,omitempty"`
}

// DocumentProcessingJobStatus is the frontend-safe view of a processing job.
type DocumentProcessingJobStatus struct {
	ID             string               `json:"id"`
	Type           string               `json:"type"`
	Status         processing.JobStatus `json:"status"`
	Attempts       int                  `json:"attempts"`
	MaxAttempts    int                  `json:"max_attempts"`
	AvailableAt    time.Time            `json:"available_at"`
	LockedUntil    *time.Time           `json:"locked_until,omitempty"`
	LastError      string               `json:"last_error,omitempty"`
	LastTransition time.Time            `json:"last_transition"`
}

// ListDocumentChunksInput describes a paginated chunk query.
type ListDocumentChunksInput struct {
	DocumentID string
	Limit      int
	Offset     int
}

// DocumentChunksPage is a paginated response for processed chunks.
type DocumentChunksPage struct {
	DocumentID string                           `json:"document_id"`
	Chunks     []processing.DocumentChunkRecord `json:"chunks"`
	Limit      int                              `json:"limit"`
	Offset     int                              `json:"offset"`
	Count      int                              `json:"count"`
}

// CreateDocumentUseCase creates document metadata and records an audit event.
type CreateDocumentUseCase struct {
	DocRepo        documentRepository
	AuditRepo      auditRepository
	StorageRepo    storageRepository
	ProcessingRepo processingRepository
	Authorizer     authorizer
	Transactor     transactor
}

// CreateDocumentInput contains the information required to create a document.
type CreateDocumentInput struct {
	JVID       string
	RequestID  string
	Name       string
	Type       documents.Type
	StorageKey string
}

// Execute creates the document and records an audit event in one transaction.
func (u CreateDocumentUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input CreateDocumentInput,
) (*documents.Document, error) {
	docType, err := validateCreateDocumentInput(input)
	if err != nil {
		return nil, err
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, input.JVID, access.PermissionDocumentCreate); err != nil {
		return nil, err
	}

	artifacts := newCreateDocumentArtifacts(actor, input, docType)

	if err = u.Transactor.WithinTx(ctx, func(txCtx context.Context) error {
		return u.saveCreatedDocument(txCtx, artifacts)
	}); err != nil {
		return nil, err
	}

	return &artifacts.Document, nil
}

type createDocumentArtifacts struct {
	Document      documents.Document
	StorageObject storage.Object
	AuditEvent    audit.Event
	OutboxEvent   processing.OutboxEvent
	Job           processing.Job
}

func validateCreateDocumentInput(input CreateDocumentInput) (documents.Type, error) {
	docType := documents.NormalizeType(input.Type)
	if input.JVID == "" || input.Name == "" || input.StorageKey == "" || !documents.IsValidType(docType) {
		return "", ErrInvalidInput
	}
	if _, err := uuid.Parse(input.JVID); err != nil {
		return "", fmt.Errorf("%w: invalid jv_id", ErrInvalidInput)
	}

	return docType, nil
}

func newCreateDocumentArtifacts(
	actor access.Principal,
	input CreateDocumentInput,
	docType documents.Type,
) createDocumentArtifacts {
	doc := documents.Document{
		ID:         uuid.NewString(),
		JVID:       input.JVID,
		Name:       input.Name,
		Type:       docType,
		StorageKey: input.StorageKey,
		UploadedBy: actor.UserKey(),
		UploadedAt: time.Now().UTC(),
		Status:     documents.StatusQueued,
		Processed:  false,
	}

	event := audit.NewEvent(
		uuid.NewString(),
		actor.UserKey(),
		input.RequestID,
		audit.ActionDocumentUploaded,
		doc.ID,
		audit.TargetDocument,
	).WithMetadata("jv_id", doc.JVID).WithMetadata("name", doc.Name)

	return createDocumentArtifacts{
		Document: doc,
		StorageObject: storage.Object{
			ID:         uuid.NewString(),
			OwnerType:  storage.OwnerDocument,
			OwnerID:    doc.ID,
			Container:  defaultDocumentContainer,
			StorageKey: doc.StorageKey,
			Filename:   doc.Name,
			Kind:       storage.KindRaw,
			CreatedBy:  actor.UserKey(),
			CreatedAt:  doc.UploadedAt,
		},
		AuditEvent:  event,
		OutboxEvent: processing.NewDocumentUploadedOutboxEvent(doc.ID, doc.JVID, doc.StorageKey),
		Job:         processing.NewParseDocumentJob(doc.ID, doc.JVID, doc.StorageKey),
	}
}

func (u CreateDocumentUseCase) saveCreatedDocument(ctx context.Context, artifacts createDocumentArtifacts) error {
	if err := u.DocRepo.Save(ctx, artifacts.Document); err != nil {
		return fmt.Errorf("saving document: %w", err)
	}
	if err := u.saveStorageObject(ctx, artifacts.StorageObject); err != nil {
		return err
	}
	if err := u.AuditRepo.Save(ctx, artifacts.AuditEvent); err != nil {
		return fmt.Errorf("saving audit event: %w", err)
	}
	if err := u.saveProcessingArtifacts(ctx, artifacts.OutboxEvent, artifacts.Job); err != nil {
		return err
	}
	return nil
}

func (u CreateDocumentUseCase) saveStorageObject(ctx context.Context, object storage.Object) error {
	if u.StorageRepo == nil {
		return nil
	}
	if err := u.StorageRepo.Save(ctx, object); err != nil {
		return fmt.Errorf("saving storage object: %w", err)
	}
	return nil
}

func (u CreateDocumentUseCase) saveProcessingArtifacts(
	ctx context.Context,
	event processing.OutboxEvent,
	job processing.Job,
) error {
	if u.ProcessingRepo == nil {
		return nil
	}
	if err := u.ProcessingRepo.SaveOutboxEvent(ctx, event); err != nil {
		return fmt.Errorf("saving outbox event: %w", err)
	}
	if err := u.ProcessingRepo.SaveJob(ctx, job); err != nil {
		return fmt.Errorf("saving processing job: %w", err)
	}
	return nil
}

// RequestDocumentUploadUseCase creates pending metadata and a direct Blob upload URL.
type RequestDocumentUploadUseCase struct {
	DocRepo      documentRepository
	AuditRepo    auditRepository
	StorageRepo  storageRepository
	BlobGateway  blobGateway
	Authorizer   authorizer
	Transactor   transactor
	UploadURLTTL time.Duration
}

// RequestDocumentUploadInput contains the client-provided file metadata.
type RequestDocumentUploadInput struct {
	JVID        string
	RequestID   string
	Filename    string
	Type        documents.Type
	ContentType string
	SizeBytes   *int64
}

// RequestDocumentUploadOutput returns the pending document and direct upload target.
type RequestDocumentUploadOutput struct {
	Document documents.Document `json:"document"`
	Upload   storage.UploadURL  `json:"upload"`
}

// Execute creates a pending document and returns a short-lived direct upload URL.
func (u RequestDocumentUploadUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input RequestDocumentUploadInput,
) (*RequestDocumentUploadOutput, error) {
	docType, filename, err := validateRequestDocumentUploadInput(input)
	if err != nil {
		return nil, err
	}
	if u.BlobGateway == nil {
		return nil, ErrStorageNotConfigured
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, input.JVID, access.PermissionDocumentCreate); err != nil {
		return nil, err
	}

	artifacts := newRequestUploadArtifacts(actor, input, filename, docType)
	artifacts.StorageObject.Container = blobContainerName(u.BlobGateway)

	if err = u.Transactor.WithinTx(ctx, func(txCtx context.Context) error {
		return u.saveRequestedUpload(txCtx, artifacts)
	}); err != nil {
		return nil, err
	}

	expiresAt := time.Now().UTC().Add(uploadURLTTL(u.UploadURLTTL))
	upload, err := u.BlobGateway.CreateUploadURL(ctx, artifacts.Document.StorageKey, input.ContentType, expiresAt)
	if err != nil {
		if errors.Is(err, storage.ErrBlobStorageNotConfigured) {
			return nil, ErrStorageNotConfigured
		}
		return nil, fmt.Errorf("creating upload url: %w", err)
	}

	return &RequestDocumentUploadOutput{
		Document: artifacts.Document,
		Upload:   upload,
	}, nil
}

type requestUploadArtifacts struct {
	Document      documents.Document
	StorageObject storage.Object
	AuditEvent    audit.Event
}

func validateRequestDocumentUploadInput(input RequestDocumentUploadInput) (documents.Type, string, error) {
	docType := documents.NormalizeType(input.Type)
	filename := sanitizeFilename(input.Filename)
	if input.JVID == "" || filename == "" || !documents.IsValidType(docType) {
		return "", "", ErrInvalidInput
	}
	if _, err := uuid.Parse(input.JVID); err != nil {
		return "", "", fmt.Errorf("%w: invalid jv_id", ErrInvalidInput)
	}
	if input.SizeBytes != nil && *input.SizeBytes < 0 {
		return "", "", fmt.Errorf("%w: invalid size_bytes", ErrInvalidInput)
	}

	return docType, filename, nil
}

func newRequestUploadArtifacts(
	actor access.Principal,
	input RequestDocumentUploadInput,
	filename string,
	docType documents.Type,
) requestUploadArtifacts {
	now := time.Now().UTC()
	docID := uuid.NewString()
	storageKey := buildRawDocumentStorageKey(input.JVID, docID, filename)

	doc := documents.Document{
		ID:         docID,
		JVID:       input.JVID,
		Name:       filename,
		Type:       docType,
		StorageKey: storageKey,
		UploadedBy: actor.UserKey(),
		UploadedAt: now,
		Status:     documents.StatusUploadPending,
		Processed:  false,
	}

	event := audit.NewEvent(
		uuid.NewString(),
		actor.UserKey(),
		input.RequestID,
		audit.ActionDocumentUploadRequested,
		doc.ID,
		audit.TargetDocument,
	).WithMetadata("jv_id", doc.JVID).WithMetadata("name", doc.Name)

	return requestUploadArtifacts{
		Document: doc,
		StorageObject: storage.Object{
			ID:          uuid.NewString(),
			OwnerType:   storage.OwnerDocument,
			OwnerID:     doc.ID,
			Container:   defaultDocumentContainer,
			StorageKey:  doc.StorageKey,
			Filename:    doc.Name,
			ContentType: input.ContentType,
			SizeBytes:   input.SizeBytes,
			Kind:        storage.KindRaw,
			CreatedBy:   actor.UserKey(),
			CreatedAt:   now,
		},
		AuditEvent: event,
	}
}

func (u RequestDocumentUploadUseCase) saveRequestedUpload(ctx context.Context, artifacts requestUploadArtifacts) error {
	if err := u.DocRepo.Save(ctx, artifacts.Document); err != nil {
		return fmt.Errorf("saving pending document: %w", err)
	}
	if err := u.StorageRepo.Save(ctx, artifacts.StorageObject); err != nil {
		return fmt.Errorf("saving pending storage object: %w", err)
	}
	if err := u.AuditRepo.Save(ctx, artifacts.AuditEvent); err != nil {
		return fmt.Errorf("saving audit event: %w", err)
	}
	return nil
}

// CompleteDocumentUploadUseCase verifies an uploaded blob and queues processing.
type CompleteDocumentUploadUseCase struct {
	DocRepo        documentRepository
	AuditRepo      auditRepository
	StorageRepo    storageRepository
	ProcessingRepo processingRepository
	BlobGateway    blobGateway
	Authorizer     authorizer
	Transactor     transactor
}

// CompleteDocumentUploadInput contains the upload completion request.
type CompleteDocumentUploadInput struct {
	DocumentID string
	RequestID  string
	SizeBytes  *int64
}

// Execute verifies the raw blob exists, then marks the document ready for processing.
func (u CompleteDocumentUploadUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input CompleteDocumentUploadInput,
) (*documents.Document, error) {
	if _, err := uuid.Parse(input.DocumentID); err != nil {
		return nil, fmt.Errorf("%w: invalid document id", ErrInvalidInput)
	}
	if input.SizeBytes != nil && *input.SizeBytes < 0 {
		return nil, fmt.Errorf("%w: invalid size_bytes", ErrInvalidInput)
	}
	if u.BlobGateway == nil {
		return nil, ErrStorageNotConfigured
	}

	doc, err := u.DocRepo.FindByID(ctx, input.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if doc == nil {
		return nil, ErrNotFound
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, doc.JVID, access.PermissionDocumentCreate); err != nil {
		return nil, err
	}
	if doc.Status != documents.StatusUploadPending && doc.Status != documents.StatusUploaded {
		return doc, nil
	}

	props, err := u.BlobGateway.GetProperties(ctx, doc.StorageKey)
	if errors.Is(err, storage.ErrBlobNotFound) {
		return nil, ErrBlobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("verifying uploaded blob: %w", err)
	}
	if props.SizeBytes <= 0 {
		return nil, fmt.Errorf("%w: empty uploaded blob", ErrInvalidInput)
	}
	if input.SizeBytes != nil && *input.SizeBytes != props.SizeBytes {
		return nil, fmt.Errorf("%w: uploaded size mismatch", ErrInvalidInput)
	}

	artifacts := newCompletedUploadArtifacts(actor, input.RequestID, *doc, props)

	if err = u.Transactor.WithinTx(ctx, func(txCtx context.Context) error {
		return u.saveCompletedUpload(txCtx, artifacts)
	}); err != nil {
		return nil, err
	}

	return &artifacts.Document, nil
}

type completedUploadArtifacts struct {
	Document      documents.Document
	StorageObject storage.Object
	AuditEvent    audit.Event
	OutboxEvent   processing.OutboxEvent
	Job           processing.Job
}

func newCompletedUploadArtifacts(
	actor access.Principal,
	requestID string,
	doc documents.Document,
	props storage.BlobProperties,
) completedUploadArtifacts {
	now := time.Now().UTC()
	sizeBytes := props.SizeBytes

	doc.Status = documents.StatusQueued
	doc.Processed = false

	event := audit.NewEvent(
		uuid.NewString(),
		actor.UserKey(),
		requestID,
		audit.ActionDocumentUploaded,
		doc.ID,
		audit.TargetDocument,
	).WithMetadata("jv_id", doc.JVID).
		WithMetadata("name", doc.Name).
		WithMetadata("etag", props.ETag).
		WithMetadata("version_id", props.VersionID)

	return completedUploadArtifacts{
		Document: doc,
		StorageObject: storage.Object{
			ID:          uuid.NewString(),
			OwnerType:   storage.OwnerDocument,
			OwnerID:     doc.ID,
			Container:   props.Container,
			StorageKey:  doc.StorageKey,
			Filename:    doc.Name,
			ContentType: props.ContentType,
			SizeBytes:   &sizeBytes,
			ETag:        props.ETag,
			VersionID:   props.VersionID,
			VerifiedAt:  &now,
			Kind:        storage.KindRaw,
			CreatedBy:   doc.UploadedBy,
			CreatedAt:   doc.UploadedAt,
		},
		AuditEvent:  event,
		OutboxEvent: processing.NewDocumentUploadedOutboxEvent(doc.ID, doc.JVID, doc.StorageKey),
		Job:         processing.NewParseDocumentJob(doc.ID, doc.JVID, doc.StorageKey),
	}
}

func (u CompleteDocumentUploadUseCase) saveCompletedUpload(ctx context.Context, artifacts completedUploadArtifacts) error {
	if err := u.DocRepo.Save(ctx, artifacts.Document); err != nil {
		return fmt.Errorf("saving completed upload document: %w", err)
	}
	if err := u.StorageRepo.Save(ctx, artifacts.StorageObject); err != nil {
		return fmt.Errorf("saving verified storage object: %w", err)
	}
	if err := u.AuditRepo.Save(ctx, artifacts.AuditEvent); err != nil {
		return fmt.Errorf("saving audit event: %w", err)
	}
	if u.ProcessingRepo == nil {
		return nil
	}
	if err := u.ProcessingRepo.SaveOutboxEvent(ctx, artifacts.OutboxEvent); err != nil {
		return fmt.Errorf("saving outbox event: %w", err)
	}
	if err := u.ProcessingRepo.SaveJob(ctx, artifacts.Job); err != nil {
		return fmt.Errorf("saving processing job: %w", err)
	}
	return nil
}

func uploadURLTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return 15 * time.Minute
	}
	return ttl
}

func blobContainerName(gateway blobGateway) string {
	if gateway == nil || gateway.ContainerName() == "" {
		return defaultDocumentContainer
	}
	return gateway.ContainerName()
}

func buildRawDocumentStorageKey(jvID, docID, filename string) string {
	return "jvs/" + jvID + "/documents/" + docID + "/raw/" + filename
}

func sanitizeFilename(filename string) string {
	clean := strings.TrimSpace(strings.ReplaceAll(filename, "\\", "/"))
	if clean == "" {
		return ""
	}

	base := path.Base(clean)
	if base == "." || base == "/" || strings.ContainsRune(base, 0) {
		return ""
	}
	return base
}

// GetDocumentUseCase fetches a document after authorizing its JV scope.
type GetDocumentUseCase struct {
	DocRepo    documentRepository
	Authorizer authorizer
}

// Execute retrieves a document by id.
func (u GetDocumentUseCase) Execute(ctx context.Context, actor access.Principal, id string) (*documents.Document, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: invalid document id", ErrInvalidInput)
	}

	doc, err := u.DocRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if doc == nil {
		return nil, ErrNotFound
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, doc.JVID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}

	return doc, nil
}

// GetDocumentProcessingStatusUseCase fetches processing state for one document.
type GetDocumentProcessingStatusUseCase struct {
	DocRepo        documentRepository
	ProcessingRepo processingStatusRepository
	Authorizer     authorizer
}

// Execute returns the document status, latest processing job, and parse artifact summary.
func (u GetDocumentProcessingStatusUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	documentID string,
) (*DocumentProcessingStatus, error) {
	if _, err := uuid.Parse(documentID); err != nil {
		return nil, fmt.Errorf("%w: invalid document id", ErrInvalidInput)
	}

	doc, err := u.DocRepo.FindByID(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if doc == nil {
		return nil, ErrNotFound
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, doc.JVID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}

	status := &DocumentProcessingStatus{Document: *doc}
	if u.ProcessingRepo == nil {
		return status, nil
	}

	job, err := u.ProcessingRepo.FindLatestJobByAggregate(ctx, processing.AggregateDocument, documentID)
	if err != nil {
		return nil, fmt.Errorf("finding processing job status: %w", err)
	}
	if job != nil {
		status.Job = &DocumentProcessingJobStatus{
			ID:             job.ID,
			Type:           job.JobType,
			Status:         job.Status,
			Attempts:       job.Attempts,
			MaxAttempts:    job.MaxAttempts,
			AvailableAt:    job.AvailableAt,
			LockedUntil:    job.LockedUntil,
			LastError:      job.LastError,
			LastTransition: job.UpdatedAt,
		}
	}

	parseResult, err := u.ProcessingRepo.FindParseResultSummary(ctx, documentID)
	if err != nil {
		return nil, fmt.Errorf("finding parse result summary: %w", err)
	}
	status.ParseResult = parseResult

	return status, nil
}

// ListDocumentChunksUseCase returns processed chunks after authorizing document read.
type ListDocumentChunksUseCase struct {
	DocRepo        documentRepository
	ProcessingRepo documentChunksRepository
	Authorizer     authorizer
}

// Execute returns a paginated list of processed document chunks.
func (u ListDocumentChunksUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input ListDocumentChunksInput,
) (*DocumentChunksPage, error) {
	limit, offset, err := normalizeChunkPagination(input.Limit, input.Offset)
	if err != nil {
		return nil, err
	}
	if _, err := uuid.Parse(input.DocumentID); err != nil {
		return nil, fmt.Errorf("%w: invalid document id", ErrInvalidInput)
	}

	doc, err := u.DocRepo.FindByID(ctx, input.DocumentID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if doc == nil {
		return nil, ErrNotFound
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, doc.JVID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}

	page := &DocumentChunksPage{
		DocumentID: input.DocumentID,
		Chunks:     []processing.DocumentChunkRecord{},
		Limit:      limit,
		Offset:     offset,
	}
	if u.ProcessingRepo == nil {
		return page, nil
	}

	chunks, err := u.ProcessingRepo.ListDocumentChunks(ctx, input.DocumentID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("listing document chunks: %w", err)
	}
	if chunks != nil {
		page.Chunks = chunks
	}
	page.Count = len(page.Chunks)

	return page, nil
}

func normalizeChunkPagination(limit int, offset int) (int, int, error) {
	if offset < 0 {
		return 0, 0, fmt.Errorf("%w: invalid offset", ErrInvalidInput)
	}
	if limit < 0 {
		return 0, 0, fmt.Errorf("%w: invalid limit", ErrInvalidInput)
	}
	if limit == 0 {
		limit = defaultDocumentChunkLimit
	}
	if limit > maxDocumentChunkLimit {
		limit = maxDocumentChunkLimit
	}

	return limit, offset, nil
}

// ListDocumentsByJVUseCase lists documents after authorizing the JV scope.
type ListDocumentsByJVUseCase struct {
	DocRepo    documentRepository
	Authorizer authorizer
}

// Execute returns all documents owned by a joint venture.
func (u ListDocumentsByJVUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	jvID string,
) ([]documents.Document, error) {
	if _, err := uuid.Parse(jvID); err != nil {
		return nil, fmt.Errorf("%w: invalid jv id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, jvID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}

	docs, err := u.DocRepo.FindByJVID(ctx, jvID)
	if err != nil {
		return nil, fmt.Errorf("listing documents by jv: %w", err)
	}
	if docs == nil {
		return []documents.Document{}, nil
	}

	return docs, nil
}

// DeleteDocumentUseCase deletes document metadata and records an audit event.
type DeleteDocumentUseCase struct {
	DocRepo    documentRepository
	AuditRepo  auditRepository
	Authorizer authorizer
	Transactor transactor
}

// DeleteDocumentInput contains identifiers required to delete a document.
type DeleteDocumentInput struct {
	DocumentID string
	RequestID  string
}

// Execute deletes the document and creates an audit event in one transaction.
func (u DeleteDocumentUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input DeleteDocumentInput,
) error {
	if _, err := uuid.Parse(input.DocumentID); err != nil {
		return fmt.Errorf("%w: invalid document id", ErrInvalidInput)
	}

	doc, err := u.DocRepo.FindByID(ctx, input.DocumentID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, doc.JVID, access.PermissionDocumentDelete); err != nil {
		return err
	}

	event := audit.NewEvent(
		uuid.NewString(),
		actor.UserKey(),
		input.RequestID,
		audit.ActionDocumentDeleted,
		input.DocumentID,
		audit.TargetDocument,
	).WithMetadata("jv_id", doc.JVID).WithMetadata("name", doc.Name)

	if err := u.Transactor.WithinTx(ctx, func(txCtx context.Context) error {
		if err := u.DocRepo.Delete(txCtx, input.DocumentID); err != nil {
			return fmt.Errorf("deleting document: %w", err)
		}
		if err := u.AuditRepo.Save(txCtx, event); err != nil {
			return fmt.Errorf("saving audit event: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}
