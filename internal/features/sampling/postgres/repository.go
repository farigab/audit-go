// Package postgres implements sampling persistence.
package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"audit-go/internal/features/sampling"
	platformpostgres "audit-go/internal/platform/postgres"
)

// Repository stores sampling rule sets and runs in PostgreSQL.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a sampling repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// SaveRuleSet persists a sampling rule set and makes previous sets inactive.
func (r *Repository) SaveRuleSet(ctx context.Context, ruleSet sampling.RuleSet) error {
	params, err := json.Marshal(ruleSet.Parameters)
	if err != nil {
		return fmt.Errorf("marshaling sampling parameters: %w", err)
	}
	rules, err := json.Marshal(ruleSet.QualitativeRules)
	if err != nil {
		return fmt.Errorf("marshaling qualitative rules: %w", err)
	}

	if ruleSet.Active {
		if _, err := platformpostgres.Executor(ctx, r.db).ExecContext(
			ctx,
			`UPDATE sampling_rule_sets SET active = FALSE, updated_at = NOW() WHERE jv_id = $1`,
			ruleSet.JVID,
		); err != nil {
			return fmt.Errorf("deactivating previous sampling rule sets: %w", err)
		}
	}

	const query = `
		INSERT INTO sampling_rule_sets (
			id, jv_id, name, description, parameters, qualitative_rules,
			active, created_by, created_at, updated_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	_, err = platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		query,
		ruleSet.ID,
		ruleSet.JVID,
		ruleSet.Name,
		ruleSet.Description,
		params,
		rules,
		ruleSet.Active,
		ruleSet.CreatedBy,
		ruleSet.CreatedAt,
		ruleSet.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving sampling rule set: %w", err)
	}
	return nil
}

// ListRuleSets returns sampling rule sets for a JV.
func (r *Repository) ListRuleSets(ctx context.Context, jvID string) ([]sampling.RuleSet, error) {
	const query = `
		SELECT id, jv_id, name, description, parameters, qualitative_rules,
			   active, created_by, created_at, updated_at
		FROM sampling_rule_sets
		WHERE jv_id = $1
		ORDER BY active DESC, created_at DESC
	`
	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(ctx, query, jvID)
	if err != nil {
		return nil, fmt.Errorf("querying sampling rule sets: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanRuleSets(rows)
}

// FindRuleSetByID returns one rule set.
func (r *Repository) FindRuleSetByID(ctx context.Context, id string) (*sampling.RuleSet, error) {
	const query = `
		SELECT id, jv_id, name, description, parameters, qualitative_rules,
			   active, created_by, created_at, updated_at
		FROM sampling_rule_sets
		WHERE id = $1
	`
	return scanRuleSet(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, id))
}

// FindActiveRuleSet returns the active rule set for a JV.
func (r *Repository) FindActiveRuleSet(ctx context.Context, jvID string) (*sampling.RuleSet, error) {
	const query = `
		SELECT id, jv_id, name, description, parameters, qualitative_rules,
			   active, created_by, created_at, updated_at
		FROM sampling_rule_sets
		WHERE jv_id = $1 AND active = TRUE
		ORDER BY created_at DESC
		LIMIT 1
	`
	return scanRuleSet(platformpostgres.Executor(ctx, r.db).QueryRowContext(ctx, query, jvID))
}

// SaveRun persists a sampling run and its selected items.
func (r *Repository) SaveRun(ctx context.Context, run sampling.Run) error {
	const runQuery = `
		INSERT INTO sampling_runs (
			id, jv_id, rule_set_id, status, total_candidates, selected_count, created_by, created_at
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`
	_, err := platformpostgres.Executor(ctx, r.db).ExecContext(
		ctx,
		runQuery,
		run.ID,
		run.JVID,
		nullString(run.RuleSetID),
		run.Status,
		run.TotalCandidates,
		run.SelectedCount,
		run.CreatedBy,
		run.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("saving sampling run: %w", err)
	}
	for _, item := range run.Items {
		reasons, err := json.Marshal(item.Reasons)
		if err != nil {
			return fmt.Errorf("marshaling sampling item reasons: %w", err)
		}
		if _, err = platformpostgres.Executor(ctx, r.db).ExecContext(
			ctx,
			`INSERT INTO sampling_run_items (run_id, document_id, score, reasons) VALUES ($1,$2,$3,$4)`,
			run.ID,
			item.Document.ID,
			item.Score,
			reasons,
		); err != nil {
			return fmt.Errorf("saving sampling item: %w", err)
		}
	}
	return nil
}

// ListRuns returns recent sampling runs for a JV.
func (r *Repository) ListRuns(ctx context.Context, jvID string, limit int) ([]sampling.Run, error) {
	const query = `
		SELECT id, jv_id, COALESCE(rule_set_id::text, ''), status,
			   total_candidates, selected_count, created_by, created_at
		FROM sampling_runs
		WHERE jv_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`
	rows, err := platformpostgres.Executor(ctx, r.db).QueryContext(ctx, query, jvID, limit)
	if err != nil {
		return nil, fmt.Errorf("querying sampling runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	runs := make([]sampling.Run, 0)
	for rows.Next() {
		var run sampling.Run
		if err := rows.Scan(
			&run.ID,
			&run.JVID,
			&run.RuleSetID,
			&run.Status,
			&run.TotalCandidates,
			&run.SelectedCount,
			&run.CreatedBy,
			&run.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning sampling run row: %w", err)
		}
		run.CreatedAt = run.CreatedAt.UTC()
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sampling runs: %w", err)
	}
	return runs, nil
}

func scanRuleSet(row *sql.Row) (*sampling.RuleSet, error) {
	var ruleSet sampling.RuleSet
	var params []byte
	var rules []byte
	var createdAt time.Time
	var updatedAt time.Time
	err := row.Scan(
		&ruleSet.ID,
		&ruleSet.JVID,
		&ruleSet.Name,
		&ruleSet.Description,
		&params,
		&rules,
		&ruleSet.Active,
		&ruleSet.CreatedBy,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("sampling rule set not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scanning sampling rule set: %w", err)
	}
	if err := json.Unmarshal(params, &ruleSet.Parameters); err != nil {
		return nil, fmt.Errorf("unmarshaling sampling parameters: %w", err)
	}
	if err := json.Unmarshal(rules, &ruleSet.QualitativeRules); err != nil {
		return nil, fmt.Errorf("unmarshaling qualitative rules: %w", err)
	}
	ruleSet.CreatedAt = createdAt.UTC()
	ruleSet.UpdatedAt = updatedAt.UTC()
	return &ruleSet, nil
}

func scanRuleSets(rows *sql.Rows) ([]sampling.RuleSet, error) {
	items := make([]sampling.RuleSet, 0)
	for rows.Next() {
		var ruleSet sampling.RuleSet
		var params []byte
		var rules []byte
		var createdAt time.Time
		var updatedAt time.Time
		if err := rows.Scan(
			&ruleSet.ID,
			&ruleSet.JVID,
			&ruleSet.Name,
			&ruleSet.Description,
			&params,
			&rules,
			&ruleSet.Active,
			&ruleSet.CreatedBy,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning sampling rule set row: %w", err)
		}
		if err := json.Unmarshal(params, &ruleSet.Parameters); err != nil {
			return nil, fmt.Errorf("unmarshaling sampling parameters: %w", err)
		}
		if err := json.Unmarshal(rules, &ruleSet.QualitativeRules); err != nil {
			return nil, fmt.Errorf("unmarshaling qualitative rules: %w", err)
		}
		ruleSet.CreatedAt = createdAt.UTC()
		ruleSet.UpdatedAt = updatedAt.UTC()
		items = append(items, ruleSet)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating sampling rule sets: %w", err)
	}
	return items, nil
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}
