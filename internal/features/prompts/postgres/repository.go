// Package postgres implements prompt persistence.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"audit-go/internal/features/prompts"
	platformpostgres "audit-go/internal/platform/postgres"
)

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) SavePrompt(ctx context.Context, prompt prompts.Prompt) error {
	const query = `
		INSERT INTO prompts (
			id, name, description, category, active_version_id, created_by, created_at, updated_at
		) VALUES ($1,$2,$3,$4,NULLIF($5, '')::uuid,$6,$7,$8)
		ON CONFLICT (id) DO UPDATE SET
			name = EXCLUDED.name,
			description = EXCLUDED.description,
			category = EXCLUDED.category,
			updated_at = NOW()
	`
	_, err := platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		prompt.ID,
		prompt.Name,
		prompt.Description,
		prompt.Category,
		prompt.ActiveVersionID,
		prompt.CreatedBy,
		prompt.CreatedAt,
		prompt.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving prompt: %w", err)
	}
	return nil
}

func (r *Repository) ListPrompts(ctx context.Context) ([]prompts.Prompt, error) {
	const query = `
		SELECT id, name, description, category, COALESCE(active_version_id::text, ''), created_by, created_at, updated_at
		FROM prompts
		ORDER BY updated_at DESC, name ASC
	`
	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("listing prompts: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanPrompts(rows)
}

func (r *Repository) FindPromptByID(ctx context.Context, id string) (*prompts.Prompt, error) {
	const query = `
		SELECT id, name, description, category, COALESCE(active_version_id::text, ''), created_by, created_at, updated_at
		FROM prompts
		WHERE id = $1
	`
	return scanPrompt(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, id))
}

func (r *Repository) SaveVersion(ctx context.Context, version prompts.Version) error {
	const query = `
		INSERT INTO prompt_versions (
			id, prompt_id, version, system_prompt, user_template, model, temperature, status,
			created_by, created_at, approved_by, approved_at, deprecated_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11, ''),$12,$13)
	`
	_, err := platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		version.ID,
		version.PromptID,
		version.Version,
		version.SystemPrompt,
		version.UserTemplate,
		version.Model,
		version.Temperature,
		string(version.Status),
		version.CreatedBy,
		version.CreatedAt,
		version.ApprovedBy,
		version.ApprovedAt,
		version.DeprecatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving prompt version: %w", err)
	}
	return nil
}

func (r *Repository) ListVersions(ctx context.Context, promptID string) ([]prompts.Version, error) {
	const query = `
		SELECT
			id, prompt_id, version, system_prompt, user_template, model, temperature, status,
			created_by, created_at, COALESCE(approved_by, ''), approved_at, deprecated_at
		FROM prompt_versions
		WHERE prompt_id = $1
		ORDER BY version DESC
	`
	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(ctx, query, promptID)
	if err != nil {
		return nil, fmt.Errorf("listing prompt versions: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanVersions(rows)
}

func (r *Repository) FindVersionByID(ctx context.Context, id string) (*prompts.Version, error) {
	const query = `
		SELECT
			id, prompt_id, version, system_prompt, user_template, model, temperature, status,
			created_by, created_at, COALESCE(approved_by, ''), approved_at, deprecated_at
		FROM prompt_versions
		WHERE id = $1
	`
	return scanVersion(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, id))
}

func (r *Repository) FindActiveVersion(ctx context.Context, promptID string) (*prompts.Version, error) {
	const query = `
		SELECT
			v.id, v.prompt_id, v.version, v.system_prompt, v.user_template, v.model, v.temperature, v.status,
			v.created_by, v.created_at, COALESCE(v.approved_by, ''), v.approved_at, v.deprecated_at
		FROM prompts p
		JOIN prompt_versions v ON v.id = p.active_version_id
		WHERE p.id = $1
	`
	return scanVersion(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, promptID))
}

func (r *Repository) NextVersionNumber(ctx context.Context, promptID string) (int, error) {
	var next int
	err := platformpostgres.Executor(ctx, r.db).QueryRowContext(
		ctx,
		`SELECT COALESCE(MAX(version), 0) + 1 FROM prompt_versions WHERE prompt_id = $1`,
		promptID,
	).Scan(&next)
	if err != nil {
		return 0, fmt.Errorf("finding next prompt version: %w", err)
	}
	return next, nil
}

func (r *Repository) ApproveVersion(ctx context.Context, versionID string, actor string, approvedAt time.Time) error {
	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelReadCommitted})
	if err != nil {
		return fmt.Errorf("beginning prompt approval transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var promptID string
	if err = tx.QueryRowContext(ctx, `SELECT prompt_id FROM prompt_versions WHERE id = $1`, versionID).Scan(&promptID); errors.Is(err, sql.ErrNoRows) {
		return errors.New("prompt version not found")
	} else if err != nil {
		return fmt.Errorf("finding prompt version for approval: %w", err)
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE prompt_versions
		 SET status = 'approved', approved_by = $2, approved_at = $3, deprecated_at = NULL
		 WHERE id = $1`,
		versionID,
		actor,
		approvedAt,
	); err != nil {
		return fmt.Errorf("approving prompt version: %w", err)
	}

	if _, err = tx.ExecContext(
		ctx,
		`UPDATE prompts SET active_version_id = $1, updated_at = NOW() WHERE id = $2`,
		versionID,
		promptID,
	); err != nil {
		return fmt.Errorf("setting active prompt version: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("committing prompt approval: %w", err)
	}
	return nil
}

func (r *Repository) DeprecateVersion(ctx context.Context, versionID string, deprecatedAt time.Time) error {
	const query = `
		WITH version_row AS (
			UPDATE prompt_versions
			SET status = 'deprecated', deprecated_at = $2
			WHERE id = $1
			RETURNING id, prompt_id
		)
		UPDATE prompts
		SET active_version_id = NULL, updated_at = NOW()
		FROM version_row
		WHERE prompts.id = version_row.prompt_id
		  AND prompts.active_version_id = version_row.id
	`
	res, err := platformpostgres.Executor(ctx, r.db).ExecContext(ctx, query, versionID, deprecatedAt)
	if err != nil {
		return fmt.Errorf("deprecating prompt version: %w", err)
	}
	if _, err = res.RowsAffected(); err != nil {
		return fmt.Errorf("checking deprecated prompt version: %w", err)
	}
	return nil
}

func (r *Repository) SaveRun(ctx context.Context, run prompts.Run) error {
	const query = `
		INSERT INTO prompt_runs (
			id, prompt_version_id, jv_id, question, answer, context_bytes, created_by, created_at
		) VALUES ($1,NULLIF($2, '')::uuid,$3,$4,$5,$6,$7,$8)
	`
	_, err := platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		run.ID,
		run.PromptVersionID,
		run.JVID,
		run.Question,
		run.Answer,
		run.ContextBytes,
		run.CreatedBy,
		run.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving prompt run: %w", err)
	}
	return nil
}

func (r *Repository) ListRuns(ctx context.Context, jvID string, limit int) ([]prompts.Run, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	const query = `
		SELECT id, COALESCE(prompt_version_id::text, ''), jv_id, question, answer, context_bytes, created_by, created_at
		FROM prompt_runs
		WHERE jv_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(ctx, query, jvID, limit)
	if err != nil {
		return nil, fmt.Errorf("listing prompt runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	runs := make([]prompts.Run, 0)
	for rows.Next() {
		var run prompts.Run
		if err = rows.Scan(&run.ID, &run.PromptVersionID, &run.JVID, &run.Question, &run.Answer, &run.ContextBytes, &run.CreatedBy, &run.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning prompt run: %w", err)
		}
		run.CreatedAt = run.CreatedAt.UTC()
		runs = append(runs, run)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating prompt runs: %w", err)
	}
	return runs, nil
}

func scanPrompt(row *sql.Row) (*prompts.Prompt, error) {
	var prompt prompts.Prompt
	if err := row.Scan(&prompt.ID, &prompt.Name, &prompt.Description, &prompt.Category, &prompt.ActiveVersionID, &prompt.CreatedBy, &prompt.CreatedAt, &prompt.UpdatedAt); errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("prompt not found")
	} else if err != nil {
		return nil, fmt.Errorf("scanning prompt: %w", err)
	}
	prompt.CreatedAt = prompt.CreatedAt.UTC()
	prompt.UpdatedAt = prompt.UpdatedAt.UTC()
	return &prompt, nil
}

func scanPrompts(rows *sql.Rows) ([]prompts.Prompt, error) {
	items := make([]prompts.Prompt, 0)
	for rows.Next() {
		var prompt prompts.Prompt
		if err := rows.Scan(&prompt.ID, &prompt.Name, &prompt.Description, &prompt.Category, &prompt.ActiveVersionID, &prompt.CreatedBy, &prompt.CreatedAt, &prompt.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scanning prompt row: %w", err)
		}
		prompt.CreatedAt = prompt.CreatedAt.UTC()
		prompt.UpdatedAt = prompt.UpdatedAt.UTC()
		items = append(items, prompt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating prompts: %w", err)
	}
	return items, nil
}

func scanVersion(row *sql.Row) (*prompts.Version, error) {
	var version prompts.Version
	var status string
	if err := row.Scan(
		&version.ID,
		&version.PromptID,
		&version.Version,
		&version.SystemPrompt,
		&version.UserTemplate,
		&version.Model,
		&version.Temperature,
		&status,
		&version.CreatedBy,
		&version.CreatedAt,
		&version.ApprovedBy,
		&version.ApprovedAt,
		&version.DeprecatedAt,
	); errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("prompt version not found")
	} else if err != nil {
		return nil, fmt.Errorf("scanning prompt version: %w", err)
	}
	version.Status = prompts.VersionStatus(status)
	version.CreatedAt = version.CreatedAt.UTC()
	if version.ApprovedAt != nil {
		t := version.ApprovedAt.UTC()
		version.ApprovedAt = &t
	}
	if version.DeprecatedAt != nil {
		t := version.DeprecatedAt.UTC()
		version.DeprecatedAt = &t
	}
	return &version, nil
}

func scanVersions(rows *sql.Rows) ([]prompts.Version, error) {
	items := make([]prompts.Version, 0)
	for rows.Next() {
		var version prompts.Version
		var status string
		if err := rows.Scan(
			&version.ID,
			&version.PromptID,
			&version.Version,
			&version.SystemPrompt,
			&version.UserTemplate,
			&version.Model,
			&version.Temperature,
			&status,
			&version.CreatedBy,
			&version.CreatedAt,
			&version.ApprovedBy,
			&version.ApprovedAt,
			&version.DeprecatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning prompt version row: %w", err)
		}
		version.Status = prompts.VersionStatus(status)
		version.CreatedAt = version.CreatedAt.UTC()
		if version.ApprovedAt != nil {
			t := version.ApprovedAt.UTC()
			version.ApprovedAt = &t
		}
		if version.DeprecatedAt != nil {
			t := version.DeprecatedAt.UTC()
			version.DeprecatedAt = &t
		}
		items = append(items, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating prompt versions: %w", err)
	}
	return items, nil
}
