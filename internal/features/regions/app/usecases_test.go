package app

import (
	"context"
	"errors"
	"testing"

	"audit-go/internal/features/access"
	"audit-go/internal/features/regions"
)

const validRegionID = "00000000-0000-0000-0000-000000000001"

func TestCreateRegionRequiresSystemPermission(t *testing.T) {
	repo := &fakeRegionRepository{}
	auth := &fakeRegionAuthorizer{}
	uc := CreateRegionUseCase{Repo: repo, Authorizer: auth}

	region, err := uc.Execute(context.Background(), access.Principal{Login: "admin@example.com"}, CreateRegionInput{
		Name: "Latin America",
		Code: " latam ",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.systemPermission != access.PermissionRegionCreate {
		t.Fatalf("expected region create permission, got %q", auth.systemPermission)
	}
	if region.Code != "LATAM" {
		t.Fatalf("expected normalized code LATAM, got %q", region.Code)
	}
	if repo.saved == nil || repo.saved.ID != region.ID {
		t.Fatalf("expected region saved, got %#v", repo.saved)
	}
}

func TestGetRegionAuthorizesRegionScope(t *testing.T) {
	repo := &fakeRegionRepository{found: &regions.Region{ID: validRegionID, Name: "LATAM", Code: "LATAM"}}
	auth := &fakeRegionAuthorizer{}
	uc := GetRegionUseCase{Repo: repo, Authorizer: auth}

	_, err := uc.Execute(context.Background(), access.Principal{Login: "auditor@example.com"}, validRegionID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.regionID != validRegionID || auth.regionPermission != access.PermissionRegionRead {
		t.Fatalf("expected region read authorization, got %q %q", auth.regionID, auth.regionPermission)
	}
}

func TestUpdateRegionRejectsInvalidID(t *testing.T) {
	uc := UpdateRegionUseCase{Repo: &fakeRegionRepository{}, Authorizer: &fakeRegionAuthorizer{}}

	_, err := uc.Execute(context.Background(), access.Principal{Login: "admin@example.com"}, UpdateRegionInput{
		ID:   "not-a-uuid",
		Name: "LATAM",
		Code: "LATAM",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

type fakeRegionRepository struct {
	saved      *regions.Region
	updated    *regions.Region
	found      *regions.Region
	accessible []regions.Region
	deletedID  string
}

func (f *fakeRegionRepository) Save(_ context.Context, region regions.Region) error {
	f.saved = &region
	return nil
}

func (f *fakeRegionRepository) FindByID(context.Context, string) (*regions.Region, error) {
	return f.found, nil
}

func (f *fakeRegionRepository) ListAccessible(context.Context, access.Principal) ([]regions.Region, error) {
	return f.accessible, nil
}

func (f *fakeRegionRepository) Update(_ context.Context, region regions.Region) error {
	f.updated = &region
	return nil
}

func (f *fakeRegionRepository) Delete(_ context.Context, id string) error {
	f.deletedID = id
	return nil
}

type fakeRegionAuthorizer struct {
	systemPermission access.Permission
	regionID         string
	regionPermission access.Permission
}

func (f *fakeRegionAuthorizer) CanAccessSystem(
	_ context.Context,
	_ access.Principal,
	permission access.Permission,
) error {
	f.systemPermission = permission
	return nil
}

func (f *fakeRegionAuthorizer) CanAccessRegion(
	_ context.Context,
	_ access.Principal,
	regionID string,
	permission access.Permission,
) error {
	f.regionID = regionID
	f.regionPermission = permission
	return nil
}
