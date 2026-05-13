// Package sampling owns audit sample selection concepts.
package sampling

import (
	"strings"
	"time"

	"audit-go/internal/features/documents"
)

const (
	QualitativeRuleMainContract = "main_contract"
	QualitativeRuleGovernance   = "governance"
	QualitativeRuleFinancial    = "financial"
	QualitativeRuleUnprocessed  = "unprocessed"
)

// Parameters contains quantitative sampling controls configured by users.
type Parameters struct {
	SamplePercentage int                `json:"sample_percentage"`
	MaxItems         int                `json:"max_items"`
	MinRiskScore     int                `json:"min_risk_score"`
	RequireProcessed bool               `json:"require_processed"`
	IncludeTypes     []documents.Type   `json:"include_types"`
	IncludeStatuses  []documents.Status `json:"include_statuses"`
}

// RuleSet is a persisted sampling configuration version.
type RuleSet struct {
	ID               string     `json:"id"`
	JVID             string     `json:"jv_id"`
	Name             string     `json:"name"`
	Description      string     `json:"description"`
	Parameters       Parameters `json:"parameters"`
	QualitativeRules []string   `json:"qualitative_rules"`
	Active           bool       `json:"active"`
	CreatedBy        string     `json:"created_by"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Run records a concrete sample generated from a rule set.
type Run struct {
	ID              string       `json:"id"`
	JVID            string       `json:"jv_id"`
	RuleSetID       string       `json:"rule_set_id,omitempty"`
	Status          string       `json:"status"`
	TotalCandidates int          `json:"total_candidates"`
	SelectedCount   int          `json:"selected_count"`
	CreatedBy       string       `json:"created_by"`
	CreatedAt       time.Time    `json:"created_at"`
	Items           []SampleItem `json:"items,omitempty"`
}

// SampleItem describes why one document entered the sample.
type SampleItem struct {
	Document documents.Document `json:"document"`
	Score    int                `json:"score"`
	Reasons  []string           `json:"reasons"`
}

// Preview is the deterministic result of applying a rule set to current documents.
type Preview struct {
	JVID            string       `json:"jv_id"`
	TotalCandidates int          `json:"total_candidates"`
	SelectedCount   int          `json:"selected_count"`
	Items           []SampleItem `json:"items"`
}

// NormalizeParameters applies conservative defaults.
func NormalizeParameters(p Parameters) Parameters {
	if p.SamplePercentage <= 0 || p.SamplePercentage > 100 {
		p.SamplePercentage = 100
	}
	if p.MaxItems < 0 {
		p.MaxItems = 0
	}
	if p.MinRiskScore < 0 {
		p.MinRiskScore = 0
	}
	if p.MinRiskScore > 100 {
		p.MinRiskScore = 100
	}
	return p
}

// IsKnownQualitativeRule reports whether name maps to a domain rule.
func IsKnownQualitativeRule(name string) bool {
	switch name {
	case QualitativeRuleMainContract, QualitativeRuleGovernance, QualitativeRuleFinancial, QualitativeRuleUnprocessed:
		return true
	default:
		return false
	}
}

// QualitativeRuleLabel returns a human-readable reason for a domain rule.
func QualitativeRuleLabel(name string) string {
	switch name {
	case QualitativeRuleMainContract:
		return "Contrato principal identificado"
	case QualitativeRuleGovernance:
		return "Documento de governanca incluido por regra de dominio"
	case QualitativeRuleFinancial:
		return "Documento financeiro incluido por regra de dominio"
	case QualitativeRuleUnprocessed:
		return "Documento pendente de processamento incluido por risco operacional"
	default:
		return "Regra qualitativa aplicada"
	}
}

// MatchesQualitativeRule evaluates domain-specific selection rules.
func MatchesQualitativeRule(rule string, doc documents.Document) bool {
	name := strings.ToLower(doc.Name)
	switch rule {
	case QualitativeRuleMainContract:
		return doc.Type == documents.TypeContract && strings.Contains(name, "principal")
	case QualitativeRuleGovernance:
		return strings.Contains(name, "governanca") || strings.Contains(name, "governance") || strings.Contains(name, "ata")
	case QualitativeRuleFinancial:
		return doc.Type == documents.TypeFinancial || strings.Contains(name, "finance")
	case QualitativeRuleUnprocessed:
		return !doc.Processed && doc.Status != documents.StatusIndexed && doc.Status != documents.StatusDeleted
	default:
		return false
	}
}
