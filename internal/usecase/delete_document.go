package usecase

import (
	"fmt"

	"github.com/google/uuid"

	"audit-go/internal/domain"
)

// interfaces mínimas — só o que este usecase precisa
// não existe uma interface global de "tudo que um repo de documento faz"
type documentRepo interface {
	FindByID(id string) (*domain.Document, error)
	Delete(id string) error
}

type auditRepo interface {
	Save(event domain.AuditEvent) error
}

type DeleteDocumentUseCase struct {
	DocRepo   documentRepo
	AuditRepo auditRepo
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
