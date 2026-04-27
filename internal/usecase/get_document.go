package usecase

import "audit-go/internal/repository"

type GetDocumentUseCase struct {
	Repo repository.DocumentRepository
}

func (u GetDocumentUseCase) Execute(id string) (any, error) {
	return u.Repo.FindByID(id)
}
