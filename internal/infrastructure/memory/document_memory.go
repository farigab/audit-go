package memory

import (
	"errors"
	"sync"

	"audit-go/internal/domain"
)

type DocumentRepository struct {
	mu   sync.RWMutex
	data map[string]domain.Document
}

func NewDocumentRepository() *DocumentRepository {
	return &DocumentRepository{data: make(map[string]domain.Document)}
}

func (r *DocumentRepository) Save(doc domain.Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[doc.ID] = doc
	return nil
}

func (r *DocumentRepository) FindByID(id string) (*domain.Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	doc, ok := r.data[id]
	if !ok {
		return nil, errors.New("document not found")
	}
	return &doc, nil
}

func (r *DocumentRepository) FindByJVID(jvID string) ([]domain.Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []domain.Document
	for _, doc := range r.data {
		if doc.JVID == jvID {
			result = append(result, doc)
		}
	}
	return result, nil
}

func (r *DocumentRepository) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.data[id]; !ok {
		return errors.New("document not found")
	}
	delete(r.data, id)
	return nil
}
