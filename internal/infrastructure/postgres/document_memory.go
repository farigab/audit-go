package postgres

import (
	"errors"

	"audit-go/internal/domain"
)

type DocumentMemoryRepository struct {
	data map[string]domain.Document
}

func NewDocumentMemoryRepository() *DocumentMemoryRepository {
	return &DocumentMemoryRepository{
		data: make(map[string]domain.Document),
	}
}

func (r *DocumentMemoryRepository) Save(doc domain.Document) error {
	r.data[doc.ID] = doc
	return nil
}

func (r *DocumentMemoryRepository) FindByID(id string) (*domain.Document, error) {
	doc, ok := r.data[id]
	if !ok {
		return nil, errors.New("document not found")
	}

	return &doc, nil
}
