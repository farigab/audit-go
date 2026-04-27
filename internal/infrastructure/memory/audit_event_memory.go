package memory

import (
	"sync"

	"audit-go/internal/domain"
)

type AuditEventRepository struct {
	mu   sync.RWMutex // RWMutex, não Mutex
	data []domain.AuditEvent
}

func (r *AuditEventRepository) Save(event domain.AuditEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data = append(r.data, event)
	return nil
}

func (r *AuditEventRepository) FindByTarget(targetID string) ([]domain.AuditEvent, error) {
	r.mu.RLock() // leitura compartilhada
	defer r.mu.RUnlock()
	var result []domain.AuditEvent
	for _, e := range r.data {
		if e.TargetID == targetID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (r *AuditEventRepository) FindByTenant(tenantID string) ([]domain.AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var result []domain.AuditEvent
	for _, e := range r.data {
		if e.TenantID == tenantID {
			result = append(result, e)
		}
	}
	return result, nil
}
