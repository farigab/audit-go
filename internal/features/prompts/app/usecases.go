// Package app implements prompt and chatbot application use cases.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"audit-go/internal/features/access"
	"audit-go/internal/features/audit"
	"audit-go/internal/features/documents"
	"audit-go/internal/features/processing"
	"audit-go/internal/features/prompts"
)

const (
	defaultCategory     = "audit_chat"
	defaultModel        = "gpt-4o-mini"
	defaultTemperature  = 0.2
	defaultContextLimit = 80
	maxContextBytes     = 24000
)

var (
	ErrInvalidInput = errors.New("prompts: invalid input")
	ErrNotFound     = errors.New("prompts: not found")
	ErrForbidden    = errors.New("prompts: forbidden")
)

type repository interface {
	SavePrompt(ctx context.Context, prompt prompts.Prompt) error
	ListPrompts(ctx context.Context) ([]prompts.Prompt, error)
	FindPromptByID(ctx context.Context, id string) (*prompts.Prompt, error)
	SaveVersion(ctx context.Context, version prompts.Version) error
	ListVersions(ctx context.Context, promptID string) ([]prompts.Version, error)
	FindVersionByID(ctx context.Context, id string) (*prompts.Version, error)
	FindActiveVersion(ctx context.Context, promptID string) (*prompts.Version, error)
	NextVersionNumber(ctx context.Context, promptID string) (int, error)
	ApproveVersion(ctx context.Context, versionID string, actor string, approvedAt time.Time) error
	DeprecateVersion(ctx context.Context, versionID string, deprecatedAt time.Time) error
	SaveRun(ctx context.Context, run prompts.Run) error
	ListRuns(ctx context.Context, jvID string, limit int) ([]prompts.Run, error)
}

type documentRepository interface {
	FindByJVID(ctx context.Context, jvID string) ([]documents.Document, error)
}

type chunksRepository interface {
	ListDocumentChunks(ctx context.Context, documentID string, limit int, offset int) ([]processing.DocumentChunkRecord, error)
}

type authorizer interface {
	CanAccessJV(ctx context.Context, principal access.Principal, jvID string, permission access.Permission) error
}

type auditRepository interface {
	Save(ctx context.Context, event audit.Event) error
}

type chatClient interface {
	Chat(ctx context.Context, req ChatClientRequest) (ChatClientResponse, error)
}

type ChatClientRequest struct {
	Context      string  `json:"context"`
	Question     string  `json:"question"`
	SystemPrompt string  `json:"system_prompt"`
	UserTemplate string  `json:"user_template"`
	Model        string  `json:"model"`
	Temperature  float64 `json:"temperature"`
}

type ChatClientResponse struct {
	Answer string `json:"answer"`
}

type CreatePromptUseCase struct {
	Repo repository
}

type CreatePromptInput struct {
	Name         string  `json:"name"`
	Description  string  `json:"description"`
	Category     string  `json:"category"`
	SystemPrompt string  `json:"system_prompt"`
	UserTemplate string  `json:"user_template"`
	Model        string  `json:"model"`
	Temperature  float64 `json:"temperature"`
}

func (uc CreatePromptUseCase) Execute(ctx context.Context, principal access.Principal, in CreatePromptInput) (*prompts.Prompt, error) {
	if !canManagePrompts(principal) {
		return nil, ErrForbidden
	}

	name := strings.TrimSpace(in.Name)
	systemPrompt := strings.TrimSpace(in.SystemPrompt)
	userTemplate := strings.TrimSpace(in.UserTemplate)
	if name == "" || systemPrompt == "" {
		return nil, ErrInvalidInput
	}
	if userTemplate == "" {
		userTemplate = "Contexto:\n{{context}}\n\nPergunta: {{question}}"
	}
	category := strings.TrimSpace(in.Category)
	if category == "" {
		category = defaultCategory
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = defaultModel
	}
	temperature := in.Temperature
	if temperature == 0 {
		temperature = defaultTemperature
	}
	if temperature < 0 || temperature > 2 {
		return nil, ErrInvalidInput
	}

	now := time.Now().UTC()
	prompt := prompts.Prompt{
		ID:          uuid.NewString(),
		Name:        name,
		Description: strings.TrimSpace(in.Description),
		Category:    category,
		CreatedBy:   principal.UserKey(),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := uc.Repo.SavePrompt(ctx, prompt); err != nil {
		return nil, err
	}

	version := prompts.Version{
		ID:           uuid.NewString(),
		PromptID:     prompt.ID,
		Version:      1,
		SystemPrompt: systemPrompt,
		UserTemplate: userTemplate,
		Model:        model,
		Temperature:  temperature,
		Status:       prompts.VersionDraft,
		CreatedBy:    principal.UserKey(),
		CreatedAt:    now,
	}
	if err := uc.Repo.SaveVersion(ctx, version); err != nil {
		return nil, err
	}

	return &prompt, nil
}

type ListPromptsUseCase struct {
	Repo repository
}

func (uc ListPromptsUseCase) Execute(ctx context.Context) ([]prompts.Prompt, error) {
	return uc.Repo.ListPrompts(ctx)
}

type CreateVersionUseCase struct {
	Repo repository
}

type CreateVersionInput struct {
	PromptID     string  `json:"prompt_id"`
	SystemPrompt string  `json:"system_prompt"`
	UserTemplate string  `json:"user_template"`
	Model        string  `json:"model"`
	Temperature  float64 `json:"temperature"`
}

func (uc CreateVersionUseCase) Execute(ctx context.Context, principal access.Principal, in CreateVersionInput) (*prompts.Version, error) {
	if !canManagePrompts(principal) {
		return nil, ErrForbidden
	}
	if _, err := uuid.Parse(in.PromptID); err != nil {
		return nil, ErrInvalidInput
	}
	if strings.TrimSpace(in.SystemPrompt) == "" {
		return nil, ErrInvalidInput
	}
	if _, err := uc.Repo.FindPromptByID(ctx, in.PromptID); err != nil {
		return nil, err
	}
	next, err := uc.Repo.NextVersionNumber(ctx, in.PromptID)
	if err != nil {
		return nil, err
	}
	userTemplate := strings.TrimSpace(in.UserTemplate)
	if userTemplate == "" {
		userTemplate = "Contexto:\n{{context}}\n\nPergunta: {{question}}"
	}
	model := strings.TrimSpace(in.Model)
	if model == "" {
		model = defaultModel
	}
	temperature := in.Temperature
	if temperature == 0 {
		temperature = defaultTemperature
	}
	if temperature < 0 || temperature > 2 {
		return nil, ErrInvalidInput
	}
	version := prompts.Version{
		ID:           uuid.NewString(),
		PromptID:     in.PromptID,
		Version:      next,
		SystemPrompt: strings.TrimSpace(in.SystemPrompt),
		UserTemplate: userTemplate,
		Model:        model,
		Temperature:  temperature,
		Status:       prompts.VersionDraft,
		CreatedBy:    principal.UserKey(),
		CreatedAt:    time.Now().UTC(),
	}
	if err := uc.Repo.SaveVersion(ctx, version); err != nil {
		return nil, err
	}
	return &version, nil
}

type ListVersionsUseCase struct {
	Repo repository
}

func (uc ListVersionsUseCase) Execute(ctx context.Context, promptID string) ([]prompts.Version, error) {
	if _, err := uuid.Parse(promptID); err != nil {
		return nil, ErrInvalidInput
	}
	return uc.Repo.ListVersions(ctx, promptID)
}

type ApproveVersionUseCase struct {
	Repo repository
}

func (uc ApproveVersionUseCase) Execute(ctx context.Context, principal access.Principal, versionID string) error {
	if !canManagePrompts(principal) {
		return ErrForbidden
	}
	if _, err := uuid.Parse(versionID); err != nil {
		return ErrInvalidInput
	}
	return uc.Repo.ApproveVersion(ctx, versionID, principal.UserKey(), time.Now().UTC())
}

type DeprecateVersionUseCase struct {
	Repo repository
}

func (uc DeprecateVersionUseCase) Execute(ctx context.Context, principal access.Principal, versionID string) error {
	if !canManagePrompts(principal) {
		return ErrForbidden
	}
	if _, err := uuid.Parse(versionID); err != nil {
		return ErrInvalidInput
	}
	return uc.Repo.DeprecateVersion(ctx, versionID, time.Now().UTC())
}

type ChatUseCase struct {
	Repo           repository
	DocRepo        documentRepository
	ProcessingRepo chunksRepository
	Authorizer     authorizer
	AuditRepo      auditRepository
	ChatClient     chatClient
}

type ChatInput struct {
	JVID            string `json:"jv_id"`
	Question        string `json:"question"`
	PromptID        string `json:"prompt_id"`
	PromptVersionID string `json:"prompt_version_id"`
}

type ChatResponse struct {
	Run           prompts.Run     `json:"run"`
	PromptVersion prompts.Version `json:"prompt_version"`
}

func (uc ChatUseCase) Execute(ctx context.Context, principal access.Principal, in ChatInput) (*ChatResponse, error) {
	if _, err := uuid.Parse(in.JVID); err != nil {
		return nil, ErrInvalidInput
	}
	question := strings.TrimSpace(in.Question)
	if question == "" {
		return nil, ErrInvalidInput
	}
	if err := uc.Authorizer.CanAccessJV(ctx, principal, in.JVID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}

	version, err := uc.resolveVersion(ctx, in)
	if err != nil {
		return nil, err
	}
	if version.Status == prompts.VersionDeprecated {
		return nil, ErrInvalidInput
	}

	contextText, err := uc.buildContext(ctx, in.JVID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contextText) == "" {
		return nil, ErrNotFound
	}

	chatResp, err := uc.ChatClient.Chat(ctx, ChatClientRequest{
		Context:      contextText,
		Question:     question,
		SystemPrompt: version.SystemPrompt,
		UserTemplate: version.UserTemplate,
		Model:        version.Model,
		Temperature:  version.Temperature,
	})
	if err != nil {
		return nil, fmt.Errorf("calling chat client: %w", err)
	}

	run := prompts.Run{
		ID:              uuid.NewString(),
		PromptVersionID: version.ID,
		JVID:            in.JVID,
		Question:        question,
		Answer:          strings.TrimSpace(chatResp.Answer),
		ContextBytes:    len([]byte(contextText)),
		CreatedBy:       principal.UserKey(),
		CreatedAt:       time.Now().UTC(),
	}
	if err := uc.Repo.SaveRun(ctx, run); err != nil {
		return nil, err
	}
	if uc.AuditRepo != nil {
		event := audit.NewEvent(uuid.NewString(), principal.UserKey(), "", audit.ActionChatQueried, run.ID, audit.TargetChat).
			WithMetadata("jv_id", in.JVID).
			WithMetadata("prompt_version_id", version.ID)
		_ = uc.AuditRepo.Save(ctx, event)
	}

	return &ChatResponse{Run: run, PromptVersion: *version}, nil
}

func (uc ChatUseCase) resolveVersion(ctx context.Context, in ChatInput) (*prompts.Version, error) {
	if in.PromptVersionID != "" {
		if _, err := uuid.Parse(in.PromptVersionID); err != nil {
			return nil, ErrInvalidInput
		}
		return uc.Repo.FindVersionByID(ctx, in.PromptVersionID)
	}
	if in.PromptID != "" {
		if _, err := uuid.Parse(in.PromptID); err != nil {
			return nil, ErrInvalidInput
		}
		return uc.Repo.FindActiveVersion(ctx, in.PromptID)
	}
	items, err := uc.Repo.ListPrompts(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if item.ActiveVersionID != "" {
			return uc.Repo.FindVersionByID(ctx, item.ActiveVersionID)
		}
	}
	return nil, ErrNotFound
}

func (uc ChatUseCase) buildContext(ctx context.Context, jvID string) (string, error) {
	docs, err := uc.DocRepo.FindByJVID(ctx, jvID)
	if err != nil {
		return "", err
	}
	var builder strings.Builder
	for _, doc := range docs {
		if builder.Len() >= maxContextBytes {
			break
		}
		chunks, err := uc.ProcessingRepo.ListDocumentChunks(ctx, doc.ID, defaultContextLimit, 0)
		if err != nil {
			return "", err
		}
		for _, chunk := range chunks {
			if strings.TrimSpace(chunk.Content) == "" {
				continue
			}
			if builder.Len() >= maxContextBytes {
				break
			}
			_, _ = builder.WriteString("\n[document_id=")
			_, _ = builder.WriteString(doc.ID)
			_, _ = builder.WriteString(" chunk=")
			_, _ = builder.WriteString(fmt.Sprint(chunk.Index))
			_, _ = builder.WriteString("]\n")
			_, _ = builder.WriteString(chunk.Content)
			_, _ = builder.WriteString("\n")
		}
	}
	out := builder.String()
	if len([]byte(out)) > maxContextBytes {
		return string([]byte(out)[:maxContextBytes]), nil
	}
	return out, nil
}

type ListRunsUseCase struct {
	Repo       repository
	Authorizer authorizer
}

func (uc ListRunsUseCase) Execute(ctx context.Context, principal access.Principal, jvID string) ([]prompts.Run, error) {
	if _, err := uuid.Parse(jvID); err != nil {
		return nil, ErrInvalidInput
	}
	if err := uc.Authorizer.CanAccessJV(ctx, principal, jvID, access.PermissionDocumentRead); err != nil {
		return nil, err
	}
	return uc.Repo.ListRuns(ctx, jvID, 20)
}

func canManagePrompts(principal access.Principal) bool {
	return principal.HasRole(access.RoleAdmin) || principal.HasRole(access.RoleRegionAdmin) || principal.HasRole(access.RoleJVAdmin)
}
