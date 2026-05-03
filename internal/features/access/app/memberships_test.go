package app

import (
	"context"
	"errors"
	"testing"

	"audit-go/internal/features/access"
)

const validScopeID = "00000000-0000-0000-0000-000000000001"

func TestCreateMembershipAuthorizesTargetScope(t *testing.T) {
	repo := &fakeMembershipRepository{}
	auth := &fakeMembershipAuthorizer{}
	uc := CreateMembershipUseCase{Repo: repo, Authorizer: auth}

	membership, err := uc.Execute(context.Background(), access.Principal{Login: "admin@example.com"}, CreateMembershipInput{
		UserLogin: "auditor@example.com",
		Role:      access.RoleAuditor,
		ScopeType: access.ScopeRegion,
		ScopeID:   validScopeID,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.scopeType != access.ScopeRegion || auth.scopeID != validScopeID {
		t.Fatalf("expected region authorization, got %q %q", auth.scopeType, auth.scopeID)
	}
	if repo.saved == nil || repo.saved.ID != membership.ID {
		t.Fatalf("expected membership saved, got %#v", repo.saved)
	}
}

func TestCreateMembershipRejectsSystemScopeID(t *testing.T) {
	uc := CreateMembershipUseCase{
		Repo:       &fakeMembershipRepository{},
		Authorizer: &fakeMembershipAuthorizer{},
	}

	_, err := uc.Execute(context.Background(), access.Principal{Login: "admin@example.com"}, CreateMembershipInput{
		UserLogin: "auditor@example.com",
		Role:      access.RoleAuditor,
		ScopeType: access.ScopeSystem,
		ScopeID:   validScopeID,
	})
	if !errors.Is(err, ErrInvalidMembership) {
		t.Fatalf("expected ErrInvalidMembership, got %v", err)
	}
}

func TestListMembershipsWithoutScopeRequiresSystemAccess(t *testing.T) {
	repo := &fakeMembershipRepository{}
	auth := &fakeMembershipAuthorizer{}
	uc := ListMembershipsUseCase{Repo: repo, Authorizer: auth}

	_, err := uc.Execute(context.Background(), access.Principal{Login: "admin@example.com"}, MembershipFilter{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.scopeType != access.ScopeSystem || auth.scopeID != "" {
		t.Fatalf("expected system membership management authorization, got %q %q", auth.scopeType, auth.scopeID)
	}
}

type fakeMembershipRepository struct {
	saved       *access.Membership
	find        *access.Membership
	memberships []access.Membership
	deletedID   string
}

func (f *fakeMembershipRepository) SaveMembership(_ context.Context, membership access.Membership) error {
	f.saved = &membership
	return nil
}

func (f *fakeMembershipRepository) FindMembershipByID(context.Context, string) (*access.Membership, error) {
	return f.find, nil
}

func (f *fakeMembershipRepository) ListMemberships(context.Context, MembershipFilter) ([]access.Membership, error) {
	return f.memberships, nil
}

func (f *fakeMembershipRepository) DeleteMembership(_ context.Context, id string) error {
	f.deletedID = id
	return nil
}

type fakeMembershipAuthorizer struct {
	systemPermission access.Permission
	scopeType        access.ScopeType
	scopeID          string
}

func (f *fakeMembershipAuthorizer) CanAccessSystem(
	_ context.Context,
	_ access.Principal,
	permission access.Permission,
) error {
	f.systemPermission = permission
	return nil
}

func (f *fakeMembershipAuthorizer) CanManageMembership(
	_ context.Context,
	_ access.Principal,
	scopeType access.ScopeType,
	scopeID string,
) error {
	f.scopeType = scopeType
	f.scopeID = scopeID
	return nil
}
