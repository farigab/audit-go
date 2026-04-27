package usecase

import (
	"fmt"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/domain"
)

type createDocumentRepo interface {
	Save(doc domain.Document) error
}

type createAuditRepo interface {
	Save(event domain.AuditEvent) error
}

type CreateDocumentUseCase struct {
	DocRepo   createDocumentRepo
	AuditRepo createAuditRepo
}

type CreateDocumentInput struct {
	JVID       string
	TenantID   string
	ActorID    string
	RequestID  string
	Name       string
	Type       domain.DocType
	StorageKey string
}

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
		// não falha o upload por causa do audit — loga e segue
		// quando tiver logger no usecase, troca o fmt por log.Warn
		return &doc, fmt.Errorf("saving audit event: %w", err)
	}

	return &doc, nil
}
