// Package usecase implements application use cases and business logic.
package usecase

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/domain"
)

// CreateDocumentUseCase handles the creation and persistence of documents.
type CreateDocumentUseCase struct {
	DocRepo   createDocumentRepo
	AuditRepo createAuditRepo
}

type createDocumentRepo interface {
	Save(doc domain.Document) error
}

type createAuditRepo interface {
	Save(event domain.AuditEvent) error
}

// CreateDocumentInput contains the information required to create a document.
type CreateDocumentInput struct {
	JVID       string
	TenantID   string
	ActorID    string
	RequestID  string
	Name       string
	Type       domain.DocType
	StorageKey string
}

// Execute creates the document and records an audit event.
func (u CreateDocumentUseCase) Execute(input CreateDocumentInput) (*domain.Document, error) {
	doc := domain.Document{
		ID:         uuid.NewString(),
		JVID:       input.JVID,
		TenantID:   input.TenantID,
		Name:       input.Name,
		Type:       input.Type,
		StorageKey: input.StorageKey,
		UploadedBy: input.ActorID,
		UploadedAt: time.Now().UTC(),
		Processed:  false,
	}

	if err := u.DocRepo.Save(doc); err != nil {
		return nil, fmt.Errorf("saving document: %w", err)
	}

	event := domain.NewAuditEvent(
		uuid.NewString(),
		input.TenantID,
		input.ActorID,
		input.RequestID,
		domain.ActionDocumentUploaded,
		doc.ID,
		domain.TargetDocument,
	)

	if err := u.AuditRepo.Save(event); err != nil {
		return nil, fmt.Errorf("saving audit event: %w", err)
	}

	return &doc, nil
}
