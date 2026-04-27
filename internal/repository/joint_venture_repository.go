// internal/repository/joint_venture_repository.go
package repository

import "audit-go/internal/domain"

type JointVentureRepository interface {
	Save(jv domain.JointVenture) error
	FindByID(id string) (*domain.JointVenture, error)
	FindByTenant(tenantID string) ([]domain.JointVenture, error)
}
