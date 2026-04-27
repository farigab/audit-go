package usecase

import (
	"audit-go/internal/domain"
	"audit-go/internal/repository"
)

type CreateDocumentUseCase struct {
	Repo repository.DocumentRepository
}

func (u CreateDocumentUseCase) Execute(doc domain.Document) error {
	return u.Repo.Save(doc)
}
