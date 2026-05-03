package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/features/jointventures"
)

var (
	ErrInvalidInput = errors.New("joint ventures: invalid input")
	ErrNotFound     = errors.New("joint ventures: not found")
)

type jointVentureRepository interface {
	Save(ctx context.Context, jv jointventures.JointVenture) error
	FindByID(ctx context.Context, id string) (*jointventures.JointVenture, error)
	ListByRegionAccessible(ctx context.Context, regionID string, actor access.Principal) ([]jointventures.JointVenture, error)
	Update(ctx context.Context, jv jointventures.JointVenture) error
	Delete(ctx context.Context, id string) error
}

type authorizer interface {
	CanAccessRegion(ctx context.Context, principal access.Principal, regionID string, permission access.Permission) error
	CanAccessJV(ctx context.Context, principal access.Principal, jvID string, permission access.Permission) error
}

// CreateJointVentureUseCase creates a JV in a region.
type CreateJointVentureUseCase struct {
	Repo       jointVentureRepository
	Authorizer authorizer
}

// CreateJointVentureInput contains JV creation fields.
type CreateJointVentureInput struct {
	RegionID string
	Name     string
	Parties  []string
	Metadata map[string]string
}

// Execute creates a joint venture after region-scope authorization.
func (u CreateJointVentureUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input CreateJointVentureInput,
) (*jointventures.JointVenture, error) {
	if _, err := uuid.Parse(input.RegionID); err != nil {
		return nil, fmt.Errorf("%w: invalid region id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessRegion(ctx, actor, input.RegionID, access.PermissionJVCreate); err != nil {
		return nil, err
	}

	jv, err := jointventures.New(uuid.NewString(), input.RegionID, input.Name, actor.UserKey(), input.Parties)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if input.Metadata != nil {
		jv.Metadata = input.Metadata
	}

	if err = u.Repo.Save(ctx, jv); err != nil {
		return nil, fmt.Errorf("saving joint venture: %w", err)
	}

	return &jv, nil
}

// ListJointVenturesByRegionUseCase lists JVs visible in a region.
type ListJointVenturesByRegionUseCase struct {
	Repo jointVentureRepository
}

// Execute returns JVs reachable by system, region, or direct JV memberships.
func (u ListJointVenturesByRegionUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	regionID string,
) ([]jointventures.JointVenture, error) {
	if !actor.Authenticated() {
		return nil, access.ErrUnauthenticated
	}
	if _, err := uuid.Parse(regionID); err != nil {
		return nil, fmt.Errorf("%w: invalid region id", ErrInvalidInput)
	}

	items, err := u.Repo.ListByRegionAccessible(ctx, regionID, actor)
	if err != nil {
		return nil, fmt.Errorf("listing joint ventures by region: %w", err)
	}
	if items == nil {
		return []jointventures.JointVenture{}, nil
	}

	return items, nil
}

// GetJointVentureUseCase fetches one JV.
type GetJointVentureUseCase struct {
	Repo       jointVentureRepository
	Authorizer authorizer
}

// Execute retrieves one JV after scope authorization.
func (u GetJointVentureUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	id string,
) (*jointventures.JointVenture, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, fmt.Errorf("%w: invalid joint venture id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, id, access.PermissionJVRead); err != nil {
		return nil, err
	}

	jv, err := u.Repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if jv == nil {
		return nil, ErrNotFound
	}

	return jv, nil
}

// UpdateJointVentureUseCase updates mutable JV fields.
type UpdateJointVentureUseCase struct {
	Repo       jointVentureRepository
	Authorizer authorizer
}

// UpdateJointVentureInput contains optional mutable JV fields.
type UpdateJointVentureInput struct {
	ID       string
	Name     *string
	Parties  []string
	Status   *jointventures.Status
	Metadata map[string]string
}

// Execute updates a JV after scope authorization.
func (u UpdateJointVentureUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input UpdateJointVentureInput,
) (*jointventures.JointVenture, error) {
	if _, err := uuid.Parse(input.ID); err != nil {
		return nil, fmt.Errorf("%w: invalid joint venture id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, input.ID, access.PermissionJVUpdate); err != nil {
		return nil, err
	}

	jv, err := u.Repo.FindByID(ctx, input.ID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if jv == nil {
		return nil, ErrNotFound
	}

	if input.Name != nil {
		name := strings.TrimSpace(*input.Name)
		if name == "" {
			return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
		}
		jv.Name = name
	}
	if input.Parties != nil {
		jv.Parties = jointventures.CleanParties(input.Parties)
		if len(jv.Parties) < 2 {
			return nil, fmt.Errorf("%w: a joint venture requires at least two parties", ErrInvalidInput)
		}
	}
	if input.Status != nil {
		status := jointventures.NormalizeStatus(*input.Status)
		if !jointventures.IsValidStatus(status) {
			return nil, fmt.Errorf("%w: invalid status", ErrInvalidInput)
		}
		jv.Status = status
	}
	if input.Metadata != nil {
		jv.Metadata = input.Metadata
	}
	jv.UpdatedAt = time.Now().UTC()

	if err = u.Repo.Update(ctx, *jv); err != nil {
		return nil, fmt.Errorf("updating joint venture: %w", err)
	}

	return jv, nil
}

// DeleteJointVentureUseCase deletes a JV.
type DeleteJointVentureUseCase struct {
	Repo       jointVentureRepository
	Authorizer authorizer
}

// Execute deletes a JV after scope authorization.
func (u DeleteJointVentureUseCase) Execute(ctx context.Context, actor access.Principal, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: invalid joint venture id", ErrInvalidInput)
	}
	if err := u.Authorizer.CanAccessJV(ctx, actor, id, access.PermissionJVDelete); err != nil {
		return err
	}
	if err := u.Repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("deleting joint venture: %w", err)
	}

	return nil
}
