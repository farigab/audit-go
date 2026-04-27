// internal/usecase/delete_document.go
package usecase

import (
	"fmt"

	"audit-go/internal/domain"
	"audit-go/internal/repository"
	"github.com/google/uuid"
)

type DeleteDocumentUseCase struct {
	DocRepo   repository.DocumentRepository
	AuditRepo repository.AuditEventRepository
}

type DeleteDocumentInput struct {
	DocumentID string
	ActorID    string
	TenantID   string
	RequestID  string
}

func (u DeleteDocumentUseCase) Execute(input DeleteDocumentInput) error {
	if _, err := u.DocRepo.FindByID(input.DocumentID); err != nil {
		return fmt.Errorf("document not found: %w", err)
	}

	if err := u.DocRepo.Delete(input.DocumentID); err != nil {
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

	if err := u.AuditRepo.Save(event); err != nil {
		return fmt.Errorf("saving audit event: %w", err)
	}

	return nil
}
