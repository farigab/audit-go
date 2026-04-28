package usecase

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"audit-go/internal/domain"
)

// interfaces mínimas — só o que este usecase precisa
// não existe uma interface global de "tudo que um repo de documento faz"
type documentRepo interface {
	FindByID(ctx context.Context, id string) (*domain.Document, error)
	Delete(ctx context.Context, id string) error
}

type auditRepo interface {
	Save(ctx context.Context, event domain.AuditEvent) error
}

// DeleteDocumentUseCase handles removing documents and recording an audit event.
type DeleteDocumentUseCase struct {
	DocRepo   documentRepo
	AuditRepo auditRepo
}

// DeleteDocumentInput contains identifiers required to delete a document.
type DeleteDocumentInput struct {
	DocumentID string
	ActorID    string
	TenantID   string
	RequestID  string
}

// Execute deletes the document and creates an audit event.
func (u DeleteDocumentUseCase) Execute(ctx context.Context, input DeleteDocumentInput) error {
	if _, err := u.DocRepo.FindByID(ctx, input.DocumentID); err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	if err := u.DocRepo.Delete(ctx, input.DocumentID); err != nil {
		return fmt.Errorf("deleting document: %w", err)
	}

	event := domain.NewAuditEvent(
		uuid.NewString(),
		input.TenantID,
		input.ActorID,
		input.RequestID,
		domain.ActionDocumentDeleted,
		input.DocumentID,
		domain.TargetDocument,
	)

	if err := u.AuditRepo.Save(ctx, event); err != nil {
		return fmt.Errorf("saving audit event: %w", err)
	}

	return nil
}
