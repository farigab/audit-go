package usecase

import (
	"fmt"

	"audit-go/internal/domain"
)

type getDocumentRepo interface {
	FindByID(id string) (*domain.Document, error)
}

type GetDocumentUseCase struct {
	DocRepo getDocumentRepo
}

func (u GetDocumentUseCase) Execute(id string) (*domain.Document, error) {
	doc, err := u.DocRepo.FindByID(id)
	if err != nil {
		return nil, fmt.Errorf("document not found: %w", err)
	}

	return doc, nil
}
