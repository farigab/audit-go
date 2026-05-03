package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
)

var (
	ErrInvalidMembership  = errors.New("access memberships: invalid input")
	ErrMembershipNotFound = errors.New("access memberships: not found")
)

type membershipRepository interface {
	SaveMembership(ctx context.Context, membership access.Membership) error
	FindMembershipByID(ctx context.Context, id string) (*access.Membership, error)
	ListMemberships(ctx context.Context, filter MembershipFilter) ([]access.Membership, error)
	DeleteMembership(ctx context.Context, id string) error
}

type membershipAuthorizer interface {
	CanManageMembership(ctx context.Context, principal access.Principal, scopeType access.ScopeType, scopeID string) error
}

// MembershipFilter filters membership list results.
type MembershipFilter struct {
	UserLogin string
	ScopeType access.ScopeType
	ScopeID   string
}

// CreateMembershipUseCase grants a role to a user in a scope.
type CreateMembershipUseCase struct {
	Repo       membershipRepository
	Authorizer membershipAuthorizer
}

// CreateMembershipInput contains the grant request.
type CreateMembershipInput struct {
	UserLogin string
	Role      access.Role
	ScopeType access.ScopeType
	ScopeID   string
}

// Execute validates, authorizes, and saves a membership.
func (u CreateMembershipUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	input CreateMembershipInput,
) (*access.Membership, error) {
	if err := validateMembership(input.UserLogin, input.Role, input.ScopeType, input.ScopeID); err != nil {
		return nil, err
	}
	if err := u.Authorizer.CanManageMembership(ctx, actor, input.ScopeType, input.ScopeID); err != nil {
		return nil, err
	}

	membership := access.Membership{
		ID:        uuid.NewString(),
		UserLogin: input.UserLogin,
		Role:      input.Role,
		ScopeType: input.ScopeType,
		ScopeID:   input.ScopeID,
		CreatedAt: time.Now().UTC(),
	}

	if err := u.Repo.SaveMembership(ctx, membership); err != nil {
		return nil, fmt.Errorf("saving membership: %w", err)
	}

	return &membership, nil
}

// ListMembershipsUseCase lists memberships visible to the actor.
type ListMembershipsUseCase struct {
	Repo       membershipRepository
	Authorizer membershipAuthorizer
}

// Execute returns memberships after checking scope-level management access.
func (u ListMembershipsUseCase) Execute(
	ctx context.Context,
	actor access.Principal,
	filter MembershipFilter,
) ([]access.Membership, error) {
	if err := validateMembershipFilter(filter); err != nil {
		return nil, err
	}

	if filter.ScopeType == "" {
		if err := u.Authorizer.CanManageMembership(ctx, actor, access.ScopeSystem, ""); err != nil {
			return nil, err
		}
	} else if err := u.Authorizer.CanManageMembership(ctx, actor, filter.ScopeType, filter.ScopeID); err != nil {
		return nil, err
	}

	memberships, err := u.Repo.ListMemberships(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("listing memberships: %w", err)
	}
	if memberships == nil {
		return []access.Membership{}, nil
	}

	return memberships, nil
}

// DeleteMembershipUseCase revokes a role grant.
type DeleteMembershipUseCase struct {
	Repo       membershipRepository
	Authorizer membershipAuthorizer
}

// Execute authorizes against the target membership scope before deleting it.
func (u DeleteMembershipUseCase) Execute(ctx context.Context, actor access.Principal, id string) error {
	if _, err := uuid.Parse(id); err != nil {
		return fmt.Errorf("%w: invalid membership id", ErrInvalidMembership)
	}

	membership, err := u.Repo.FindMembershipByID(ctx, id)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMembershipNotFound, err)
	}
	if membership == nil {
		return ErrMembershipNotFound
	}

	if err := u.Authorizer.CanManageMembership(ctx, actor, membership.ScopeType, membership.ScopeID); err != nil {
		return err
	}
	if err := u.Repo.DeleteMembership(ctx, id); err != nil {
		return fmt.Errorf("deleting membership: %w", err)
	}

	return nil
}

func validateMembership(userLogin string, role access.Role, scopeType access.ScopeType, scopeID string) error {
	if userLogin == "" || !access.IsValidRole(role) || !access.IsValidScopeType(scopeType) {
		return ErrInvalidMembership
	}
	return validateScope(scopeType, scopeID)
}

func validateMembershipFilter(filter MembershipFilter) error {
	if filter.ScopeType == "" {
		if filter.ScopeID != "" {
			return fmt.Errorf("%w: scope_type is required with scope_id", ErrInvalidMembership)
		}
		return nil
	}
	if !access.IsValidScopeType(filter.ScopeType) {
		return ErrInvalidMembership
	}
	return validateScope(filter.ScopeType, filter.ScopeID)
}

func validateScope(scopeType access.ScopeType, scopeID string) error {
	if scopeType == access.ScopeSystem {
		if scopeID != "" {
			return fmt.Errorf("%w: system scope must not include scope_id", ErrInvalidMembership)
		}
		return nil
	}
	if scopeID == "" {
		return fmt.Errorf("%w: scope_id is required", ErrInvalidMembership)
	}
	if _, err := uuid.Parse(scopeID); err != nil {
		return fmt.Errorf("%w: invalid scope_id", ErrInvalidMembership)
	}
	return nil
}
