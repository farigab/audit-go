package usecase

import (
	"context"
	"fmt"

	"audit-go/internal/domain"
)

type getDocumentRepo interface {
	FindByID(ctx context.Context, id string) (*domain.Document, error)
}

// GetDocumentUseCase fetches a document by id.
type GetDocumentUseCase struct {
	DocRepo getDocumentRepo
}

// Execute retrieves the document from the repository or returns an error.
func (u GetDocumentUseCase) Execute(id string) (*domain.Document, error) {
	doc, err := u.DocRepo.FindByID(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	return doc, nil
}
