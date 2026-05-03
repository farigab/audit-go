package app

import (
	"context"
	"errors"
	"testing"

	"audit-go/internal/features/access"
	"audit-go/internal/features/jointventures"
)

const (
	validRegionID = "00000000-0000-0000-0000-000000000001"
	validJVID     = "00000000-0000-0000-0000-000000000002"
)

func TestCreateJointVentureRequiresRegionPermission(t *testing.T) {
	repo := &fakeJointVentureRepository{}
	auth := &fakeJointVentureAuthorizer{}
	uc := CreateJointVentureUseCase{Repo: repo, Authorizer: auth}

	jv, err := uc.Execute(context.Background(), access.Principal{Login: "region-admin@example.com"}, CreateJointVentureInput{
		RegionID: validRegionID,
		Name:     "JV Alpha",
		Parties:  []string{"Company A", "Company B"},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if auth.regionID != validRegionID || auth.regionPermission != access.PermissionJVCreate {
		t.Fatalf("expected JV create region authorization, got %q %q", auth.regionID, auth.regionPermission)
	}
	if jv.Status != jointventures.StatusDraft {
		t.Fatalf("expected draft status, got %q", jv.Status)
	}
	if repo.saved == nil || repo.saved.ID != jv.ID {
		t.Fatalf("expected JV saved, got %#v", repo.saved)
	}
}

func TestListJointVenturesByRegionUsesAccessibleRepositoryQuery(t *testing.T) {
	repo := &fakeJointVentureRepository{
		accessible: []jointventures.JointVenture{{ID: validJVID, RegionID: validRegionID, Name: "JV Alpha"}},
	}
	uc := ListJointVenturesByRegionUseCase{Repo: repo}

	jvs, err := uc.Execute(context.Background(), access.Principal{Login: "auditor@example.com"}, validRegionID)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if repo.listRegionID != validRegionID {
		t.Fatalf("expected region id %q, got %q", validRegionID, repo.listRegionID)
	}
	if len(jvs) != 1 || jvs[0].ID != validJVID {
		t.Fatalf("unexpected JVs: %#v", jvs)
	}
}

func TestUpdateJointVentureRejectsInvalidParties(t *testing.T) {
	repo := &fakeJointVentureRepository{
		found: &jointventures.JointVenture{
			ID:       validJVID,
			RegionID: validRegionID,
			Name:     "JV Alpha",
			Parties:  []string{"A", "B"},
			Status:   jointventures.StatusDraft,
		},
	}
	uc := UpdateJointVentureUseCase{Repo: repo, Authorizer: &fakeJointVentureAuthorizer{}}

	_, err := uc.Execute(context.Background(), access.Principal{Login: "jv-admin@example.com"}, UpdateJointVentureInput{
		ID:      validJVID,
		Parties: []string{"Only One"},
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

type fakeJointVentureRepository struct {
	saved        *jointventures.JointVenture
	updated      *jointventures.JointVenture
	found        *jointventures.JointVenture
	accessible   []jointventures.JointVenture
	listRegionID string
	deletedID    string
}

func (f *fakeJointVentureRepository) Save(_ context.Context, jv jointventures.JointVenture) error {
	f.saved = &jv
	return nil
}

func (f *fakeJointVentureRepository) FindByID(context.Context, string) (*jointventures.JointVenture, error) {
	return f.found, nil
}

func (f *fakeJointVentureRepository) ListByRegionAccessible(
	_ context.Context,
	regionID string,
	_ access.Principal,
) ([]jointventures.JointVenture, error) {
	f.listRegionID = regionID
	return f.accessible, nil
}

func (f *fakeJointVentureRepository) Update(_ context.Context, jv jointventures.JointVenture) error {
	f.updated = &jv
	return nil
}

func (f *fakeJointVentureRepository) Delete(_ context.Context, id string) error {
	f.deletedID = id
	return nil
}

type fakeJointVentureAuthorizer struct {
	regionID         string
	regionPermission access.Permission
	jvID             string
	jvPermission     access.Permission
}

func (f *fakeJointVentureAuthorizer) CanAccessRegion(
	_ context.Context,
	_ access.Principal,
	regionID string,
	permission access.Permission,
) error {
	f.regionID = regionID
	f.regionPermission = permission
	return nil
}

func (f *fakeJointVentureAuthorizer) CanAccessJV(
	_ context.Context,
	_ access.Principal,
	jvID string,
	permission access.Permission,
) error {
	f.jvID = jvID
	f.jvPermission = permission
	return nil
}
