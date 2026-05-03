package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/features/regions"
)

var (
	ErrInvalidInput = errors.New("regions: invalid input")
	ErrNotFound     = errors.New("regions: not found")
)

type regionRepository interface {
	Save(ctx context.Context, region regions.Region) error
	FindByID(ctx context.Context, id string) (*regions.Region, error)
	ListAccessible(ctx context.Context, actor access.Principal) ([]regions.Region, error)
	Update(ctx context.Context, region regions.Region) error
	Delete(ctx context.Context, id string) error
}

type authorizer interface {
	CanAccessSystem(ctx context.Context, principal access.Principal, permission access.Permission) error
	CanAccessRegion(ctx context.Context, principal access.Principal, regionID string, permission access.Permission) error
}

// CreateRegionUseCase creates a region.
type CreateRegionUseCase struct {
	Repo       regionRepository
	Authorizer authorizer
}

// CreateRegionInput contains region creation fields.
type CreateRegionInput struct {
	Name string
	Code string
}

// Execute creates a region after system-level authorization.
func (u CreateRegionUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input CreateRegionInput,
) (*regions.Region, error) {
	if err := u.Authorizer.CanAccessSystem(ctx, actor, access.PermissionRegionCreate); err != nil {
		return nil, err
	}

	region, err := regions.New(uuid.NewString(), input.Name, input.Code)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err = u.Repo.Save(ctx, region); err != nil {
		return nil, fmt.Errorf("saving region: %w", err)
	}

	return &region, nil
}

// ListRegionsUseCase lists regions visible to the actor.
type ListRegionsUseCase struct {
	Repo regionRepository
}

// Execute returns regions reachable through system or region memberships.
func (u ListRegionsUseCase) Execute(ctx context.Context, actor access.Principal) ([]regions.Region, error) {
	if !actor.Authenticated() {
		return nil, access.ErrUnauthenticated
	}

	items, err := u.Repo.ListAccessible(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("listing regions: %w", err)
	}
	if items == nil {
		return []regions.Region{}, nil
	}

	return items, nil
}

// GetRegionUseCase fetches one region.
type GetRegionUseCase struct {
	Repo       regionRepository
	Authorizer authorizer
}

// Execute retrieves one region after scope authorization.
func (u GetRegionUseCase) Execute(ctx context.Context, actor access.Principal, id string) (*regions.Region, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: invalid region id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessRegion(ctx, actor, id, access.PermissionRegionRead); err != nil {
		return nil, err
	}

	region, err := u.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if region == nil {
		return nil, ErrNotFound
	}

	return region, nil
}

// UpdateRegionUseCase updates region attributes.
type UpdateRegionUseCase struct {
	Repo       regionRepository
	Authorizer authorizer
}

// UpdateRegionInput contains mutable region fields.
type UpdateRegionInput struct {
	ID   string
	Name string
	Code string
}

// Execute updates a region after scope authorization.
func (u UpdateRegionUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input UpdateRegionInput,
) (*regions.Region, error) {
	if _, err := uuid.Parse(input.ID); err != nil {
		return nil, fmt.Errorf("%w: invalid region id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessRegion(ctx, actor, input.ID, access.PermissionRegionUpdate); err != nil {
		return nil, err
	}

	current, err := u.Repo.FindByID(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if current == nil {
		return nil, ErrNotFound
	}

	updated, err := regions.New(current.ID, input.Name, input.Code)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	updated.CreatedAt = current.CreatedAt

	if err = u.Repo.Update(ctx, updated); err != nil {
		return nil, fmt.Errorf("updating region: %w", err)
	}

	return &updated, nil
}

// DeleteRegionUseCase deletes a region.
type DeleteRegionUseCase struct {
	Repo       regionRepository
	Authorizer authorizer
}

// Execute deletes a region after system-level authorization.
func (u DeleteRegionUseCase) Execute(ctx context.Context, actor access.Principal, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: invalid region id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessSystem(ctx, actor, access.PermissionRegionDelete); err != nil {
		return err
	}
	if err := u.Repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting region: %w", err)
	}

	return nil
}
