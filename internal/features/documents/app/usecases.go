// Package app implements document application use cases.
package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/features/audit"
	"audit-go/internal/features/documents"
	"audit-go/internal/features/processing"
	"audit-go/internal/platform/storage"
)

const defaultDocumentContainer = "documents"

var (
	ErrInvalidInput = errors.New("documents: invalid input")
	ErrNotFound     = errors.New("documents: not found")
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

type processingRepository interface {
	SaveOutboxEvent(ctx context.Context, event processing.OutboxEvent) error
	SaveJob(ctx context.Context, job processing.Job) error
}

type authorizer interface {
	CanAccessJV(ctx context.Context, principal access.Principal, jvID string, permission access.Permission) error
}

type transactor interface {
	WithinTx(ctx context.Context, fn func(context.Context) error) error
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
	if err := u.Authorizer.CanAccessJV(ctx, actor, doc.JVID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}

	return doc, nil
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
