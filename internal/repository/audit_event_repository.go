// internal/repository/audit_event_repository.go
package repository

import "audit-go/internal/domain"

// AuditEventRepository é append-only por design.
// Não há Update nem Delete — eventos são imutáveis.
type AuditEventRepository interface {
	Save(event domain.AuditEvent) error
	FindByTarget(targetID string) ([]domain.AuditEvent, error)
	FindByTenant(tenantID string) ([]domain.AuditEvent, error)
}
