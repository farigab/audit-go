// Package app implements sampling application use cases.
package app

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/features/audit"
	"audit-go/internal/features/documents"
	"audit-go/internal/features/sampling"
)

var (
	ErrInvalidInput = errors.New("sampling: invalid input")
	ErrNotFound     = errors.New("sampling: not found")
)

type repository interface {
	SaveRuleSet(ctx context.Context, ruleSet sampling.RuleSet) error
	ListRuleSets(ctx context.Context, jvID string) ([]sampling.RuleSet, error)
	FindRuleSetByID(ctx context.Context, id string) (*sampling.RuleSet, error)
	FindActiveRuleSet(ctx context.Context, jvID string) (*sampling.RuleSet, error)
	SaveRun(ctx context.Context, run sampling.Run) error
	ListRuns(ctx context.Context, jvID string, limit int) ([]sampling.Run, error)
}

type documentRepository interface {
	FindByJVID(ctx context.Context, jvID string) ([]documents.Document, error)
}

type auditRepository interface {
	Save(ctx context.Context, event audit.Event) error
}

type authorizer interface {
	CanAccessJV(ctx context.Context, principal access.Principal, jvID string, permission access.Permission) error
}

type transactor interface {
	WithinTx(ctx context.Context, fn func(context.Context) error) error
}

// CreateRuleSetInput contains a user-configured sampling policy.
type CreateRuleSetInput struct {
	JVID             string
	RequestID        string
	Name             string
	Description      string
	Parameters       sampling.Parameters
	QualitativeRules []string
}

// CreateRuleSetUseCase persists a new active sampling configuration.
type CreateRuleSetUseCase struct {
	Repo       repository
	AuditRepo  auditRepository
	Authorizer authorizer
	Transactor transactor
}

// Execute creates a rule set.
func (uc CreateRuleSetUseCase) Execute(ctx context.Context, actor access.Principal, input CreateRuleSetInput) (*sampling.RuleSet, error) {
	if err := validateJVID(input.JVID); err != nil {
		return nil, err
	}
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidInput)
	}
	rules, err := normalizeQualitativeRules(input.QualitativeRules)
	if err != nil {
		return nil, err
	}
	if err := uc.Authorizer.CanAccessJV(ctx, actor, input.JVID, access.PermissionDocumentCreate); err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	ruleSet := sampling.RuleSet{
		ID:               uuid.NewString(),
		JVID:             input.JVID,
		Name:             name,
		Description:      strings.TrimSpace(input.Description),
		Parameters:       sampling.NormalizeParameters(input.Parameters),
		QualitativeRules: rules,
		Active:           true,
		CreatedBy:        actor.UserKey(),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	event := audit.NewEvent(
		uuid.NewString(),
		actor.UserKey(),
		input.RequestID,
		audit.ActionSamplingRuleSetCreated,
		ruleSet.ID,
		audit.TargetSampling,
	).WithMetadata("jv_id", ruleSet.JVID).WithMetadata("name", ruleSet.Name)

	err = uc.Transactor.WithinTx(ctx, func(txCtx context.Context) error {
		if err := uc.Repo.SaveRuleSet(txCtx, ruleSet); err != nil {
			return fmt.Errorf("saving sampling rule set: %w", err)
		}
		if err := uc.AuditRepo.Save(txCtx, event); err != nil {
			return fmt.Errorf("saving audit event: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &ruleSet, nil
}

// ListRuleSetsUseCase lists sampling configurations for a JV.
type ListRuleSetsUseCase struct {
	Repo       repository
	Authorizer authorizer
}

// Execute lists rule sets.
func (uc ListRuleSetsUseCase) Execute(ctx context.Context, actor access.Principal, jvID string) ([]sampling.RuleSet, error) {
	if err := validateJVID(jvID); err != nil {
		return nil, err
	}
	if err := uc.Authorizer.CanAccessJV(ctx, actor, jvID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}
	items, err := uc.Repo.ListRuleSets(ctx, jvID)
	if err != nil {
		return nil, fmt.Errorf("listing sampling rule sets: %w", err)
	}
	if items == nil {
		return []sampling.RuleSet{}, nil
	}
	return items, nil
}

// PreviewInput describes an ad hoc sampling preview.
type PreviewInput struct {
	JVID             string
	RuleSetID        string
	Parameters       sampling.Parameters
	QualitativeRules []string
}

// PreviewUseCase applies configured rules without persisting a run.
type PreviewUseCase struct {
	Repo       repository
	DocRepo    documentRepository
	Authorizer authorizer
}

// Execute builds a preview.
func (uc PreviewUseCase) Execute(ctx context.Context, actor access.Principal, input PreviewInput) (*sampling.Preview, error) {
	ruleSet, params, rules, err := uc.resolveRules(ctx, input)
	if err != nil {
		return nil, err
	}
	jvID := input.JVID
	if ruleSet != nil {
		jvID = ruleSet.JVID
	}
	if err := uc.Authorizer.CanAccessJV(ctx, actor, jvID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}
	docs, err := uc.DocRepo.FindByJVID(ctx, jvID)
	if err != nil {
		return nil, fmt.Errorf("listing sample candidates: %w", err)
	}
	return BuildPreview(jvID, docs, params, rules), nil
}

func (uc PreviewUseCase) resolveRules(ctx context.Context, input PreviewInput) (*sampling.RuleSet, sampling.Parameters, []string, error) {
	if input.RuleSetID != "" {
		ruleSet, err := uc.Repo.FindRuleSetByID(ctx, input.RuleSetID)
		if err != nil {
			return nil, sampling.Parameters{}, nil, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		if ruleSet == nil {
			return nil, sampling.Parameters{}, nil, ErrNotFound
		}
		return ruleSet, sampling.NormalizeParameters(ruleSet.Parameters), ruleSet.QualitativeRules, nil
	}
	if err := validateJVID(input.JVID); err != nil {
		return nil, sampling.Parameters{}, nil, err
	}
	rules, err := normalizeQualitativeRules(input.QualitativeRules)
	if err != nil {
		return nil, sampling.Parameters{}, nil, err
	}
	return nil, sampling.NormalizeParameters(input.Parameters), rules, nil
}

// CreateRunInput describes a persisted sampling execution request.
type CreateRunInput struct {
	JVID      string
	RuleSetID string
	RequestID string
}

// CreateRunUseCase persists the current sample produced by a rule set.
type CreateRunUseCase struct {
	Repo       repository
	DocRepo    documentRepository
	AuditRepo  auditRepository
	Authorizer authorizer
	Transactor transactor
}

// Execute creates a sampling run.
func (uc CreateRunUseCase) Execute(ctx context.Context, actor access.Principal, input CreateRunInput) (*sampling.Run, error) {
	if err := validateJVID(input.JVID); err != nil {
		return nil, err
	}
	if err := uc.Authorizer.CanAccessJV(ctx, actor, input.JVID, access.PermissionDocumentCreate); err != nil {
		return nil, err
	}
	ruleSet, err := uc.resolveRuleSet(ctx, input)
	if err != nil {
		return nil, err
	}
	docs, err := uc.DocRepo.FindByJVID(ctx, input.JVID)
	if err != nil {
		return nil, fmt.Errorf("listing sample candidates: %w", err)
	}
	preview := BuildPreview(input.JVID, docs, ruleSet.Parameters, ruleSet.QualitativeRules)
	now := time.Now().UTC()
	run := sampling.Run{
		ID:              uuid.NewString(),
		JVID:            input.JVID,
		RuleSetID:       ruleSet.ID,
		Status:          "completed",
		TotalCandidates: preview.TotalCandidates,
		SelectedCount:   preview.SelectedCount,
		CreatedBy:       actor.UserKey(),
		CreatedAt:       now,
		Items:           preview.Items,
	}
	event := audit.NewEvent(
		uuid.NewString(),
		actor.UserKey(),
		input.RequestID,
		audit.ActionSamplingRunCreated,
		run.ID,
		audit.TargetSampling,
	).WithMetadata("jv_id", run.JVID).WithMetadata("rule_set_id", run.RuleSetID)

	err = uc.Transactor.WithinTx(ctx, func(txCtx context.Context) error {
		if err := uc.Repo.SaveRun(txCtx, run); err != nil {
			return fmt.Errorf("saving sampling run: %w", err)
		}
		if err := uc.AuditRepo.Save(txCtx, event); err != nil {
			return fmt.Errorf("saving audit event: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &run, nil
}

func (uc CreateRunUseCase) resolveRuleSet(ctx context.Context, input CreateRunInput) (*sampling.RuleSet, error) {
	if input.RuleSetID != "" {
		ruleSet, err := uc.Repo.FindRuleSetByID(ctx, input.RuleSetID)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
		}
		if ruleSet == nil || ruleSet.JVID != input.JVID {
			return nil, ErrNotFound
		}
		return ruleSet, nil
	}
	ruleSet, err := uc.Repo.FindActiveRuleSet(ctx, input.JVID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNotFound, err)
	}
	if ruleSet == nil {
		return nil, ErrNotFound
	}
	return ruleSet, nil
}

// ListRunsUseCase lists previous sampling executions.
type ListRunsUseCase struct {
	Repo       repository
	Authorizer authorizer
}

// Execute lists runs.
func (uc ListRunsUseCase) Execute(ctx context.Context, actor access.Principal, jvID string, limit int) ([]sampling.Run, error) {
	if err := validateJVID(jvID); err != nil {
		return nil, err
	}
	if err := uc.Authorizer.CanAccessJV(ctx, actor, jvID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	runs, err := uc.Repo.ListRuns(ctx, jvID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing sampling runs: %w", err)
	}
	if runs == nil {
		return []sampling.Run{}, nil
	}
	return runs, nil
}

// BuildPreview applies quantitative and qualitative rules to documents.
func BuildPreview(jvID string, docs []documents.Document, params sampling.Parameters, rules []string) *sampling.Preview {
	params = sampling.NormalizeParameters(params)
	items := make([]sampling.SampleItem, 0, len(docs))
	for _, doc := range docs {
		if doc.Status == documents.StatusDeleted || !matchesFilters(doc, params) {
			continue
		}
		score, reasons := scoreDocument(doc)
		for _, rule := range rules {
			if sampling.MatchesQualitativeRule(rule, doc) {
				score = max(score, 90)
				reasons = append(reasons, sampling.QualitativeRuleLabel(rule))
			}
		}
		if score >= params.MinRiskScore || hasQualitativeReason(reasons) {
			items = append(items, sampling.SampleItem{Document: doc, Score: score, Reasons: uniqueStrings(reasons)})
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score == items[j].Score {
			return items[i].Document.UploadedAt.After(items[j].Document.UploadedAt)
		}
		return items[i].Score > items[j].Score
	})
	limit := quantitativeLimit(len(items), params)
	if limit < len(items) {
		items = items[:limit]
	}
	return &sampling.Preview{
		JVID:            jvID,
		TotalCandidates: len(docs),
		SelectedCount:   len(items),
		Items:           items,
	}
}

func matchesFilters(doc documents.Document, params sampling.Parameters) bool {
	if params.RequireProcessed && !doc.Processed {
		return false
	}
	if len(params.IncludeTypes) > 0 {
		found := false
		for _, t := range params.IncludeTypes {
			if doc.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if len(params.IncludeStatuses) > 0 {
		found := false
		for _, s := range params.IncludeStatuses {
			if doc.Status == s {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func scoreDocument(doc documents.Document) (int, []string) {
	score := 20
	reasons := []string{"Elegivel pelos filtros quantitativos"}
	switch doc.Type {
	case documents.TypeContract:
		score += 45
		reasons = append(reasons, "Contrato tem prioridade de auditoria")
	case documents.TypeFinancial:
		score += 35
		reasons = append(reasons, "Documento financeiro aumenta risco")
	case documents.TypeReport:
		score += 20
	}
	switch doc.Status {
	case documents.StatusIndexed, documents.StatusParsed, documents.StatusOCRCompleted:
		score += 15
		reasons = append(reasons, "Documento pronto para revisao")
	case documents.StatusQueued, documents.StatusProcessing, documents.StatusUploaded:
		score += 10
		reasons = append(reasons, "Documento ainda exige acompanhamento")
	}
	if strings.Contains(strings.ToLower(doc.Name), "principal") {
		score += 20
	}
	if score > 100 {
		score = 100
	}
	return score, reasons
}

func quantitativeLimit(count int, params sampling.Parameters) int {
	limit := int(math.Ceil(float64(count) * float64(params.SamplePercentage) / 100))
	if count > 0 && limit == 0 {
		limit = 1
	}
	if params.MaxItems > 0 && params.MaxItems < limit {
		limit = params.MaxItems
	}
	return limit
}

func validateJVID(jvID string) error {
	if _, err := uuid.Parse(jvID); err != nil {
		return fmt.Errorf("%w: invalid jv id", ErrInvalidInput)
	}
	return nil
}

func normalizeQualitativeRules(rules []string) ([]string, error) {
	out := make([]string, 0, len(rules))
	seen := map[string]bool{}
	for _, rule := range rules {
		rule = strings.TrimSpace(rule)
		if rule == "" || seen[rule] {
			continue
		}
		if !sampling.IsKnownQualitativeRule(rule) {
			return nil, fmt.Errorf("%w: unknown qualitative rule", ErrInvalidInput)
		}
		seen[rule] = true
		out = append(out, rule)
	}
	return out, nil
}

func hasQualitativeReason(reasons []string) bool {
	for _, reason := range reasons {
		if strings.Contains(reason, "regra de dominio") || strings.Contains(reason, "identificado") || strings.Contains(reason, "pendente") {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}
