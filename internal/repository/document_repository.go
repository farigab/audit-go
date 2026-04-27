package repository

import "audit-go/internal/domain"

type DocumentRepository interface {
	Save(doc domain.Document) error
	FindByID(id string) (*domain.Document, error)
	FindByJVID(jvID string) ([]domain.Document, error)
	Delete(id string) error
}
