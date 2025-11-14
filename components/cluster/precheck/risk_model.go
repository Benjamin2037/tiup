package precheck

import (
	"context"
	"fmt"
	"strings"

	prechecklib "github.com/pingcap/tidb-upgrade-precheck/pkg/precheck"
	"github.com/pingcap/tidb-upgrade-precheck/pkg/rules"
)

// RiskLevel represents the severity of a risk item.
type RiskLevel string

const (
	RiskHigh   RiskLevel = "HIGH RISK"
	RiskMedium RiskLevel = "MEDIUM RISK"
	RiskLow    RiskLevel = "LOW RISK"
)

// RiskItem describes a single detected risk.
type RiskItem struct {
	Level      RiskLevel
	Category   string // e.g. Config Deprecated / Forced Upgrade Logic / Feature Lag / Silent Config Change
	Component  string
	Parameter  string
	Scope      string
	Current    string
	NewDefault string
	Impact     string
	Suggestion string
	Reason     string
	Comments   string // optional R&D comments for additional context
}

// RiskReport aggregates all risks found during the precheck.
type RiskReport struct {
	SourceVersion string
	TargetVersion string
	High          []RiskItem
	Medium        []RiskItem
	Low           []RiskItem
}

// SummaryInfo holds aggregated counters by severity.
type SummaryInfo struct {
	High, Medium, Low int
	Total             int
}

// Summary returns the aggregated counters for the report.
func (r *RiskReport) Summary() SummaryInfo {
	s := SummaryInfo{High: len(r.High), Medium: len(r.Medium), Low: len(r.Low)}
	s.Total = s.High + s.Medium + s.Low
	return s
}

// Input is the engine input. It will be extended with live cluster
// topology, system variables, component configs, TLS info, etc.
type Input struct {
	SourceVersion string
	TargetVersion string
	// TODO: Add system variables, component configs, topology, TLS, etc.
}

// Run is the core entry of the parameter guard engine (placeholder impl).
// Real flow should be: collect source cluster -> load knowledge base ->
// evaluate diffs -> generate report.
func Run(ctx context.Context, in Input) (*RiskReport, error) {
	snapshot := prechecklib.Snapshot{
		SourceVersion: strings.TrimSpace(in.SourceVersion),
		TargetVersion: strings.TrimSpace(in.TargetVersion),
	}

	ruleset := []prechecklib.Rule{
		rules.NewTargetVersionOrderRule(),
	}

	catalog, err := loadEmbeddedCatalog()
	if err != nil {
		return nil, fmt.Errorf("load upgrade metadata: %w", err)
	}
	if rule := rules.NewForcedGlobalSysvarsRule(catalog); rule != nil {
		ruleset = append(ruleset, rule)
	}

	engine := prechecklib.NewEngine(ruleset...)
	report := engine.Run(ctx, snapshot)

	return convertReport(snapshot, report), nil
}

func convertReport(snapshot prechecklib.Snapshot, report prechecklib.Report) *RiskReport {
	result := &RiskReport{
		SourceVersion: snapshot.SourceVersion,
		TargetVersion: snapshot.TargetVersion,
	}

	for _, item := range report.Items {
		risk := convertReportItem(item)
		switch risk.Level {
		case RiskHigh:
			result.High = append(result.High, risk)
		case RiskMedium:
			result.Medium = append(result.Medium, risk)
		default:
			result.Low = append(result.Low, risk)
		}
	}

	return result
}

func convertReportItem(item prechecklib.ReportItem) RiskItem {
	severity := mapSeverity(item.Severity)
	risk := RiskItem{
		Level:    severity,
		Category: mapRuleCategory(item.Rule),
		Impact:   strings.TrimSpace(item.Message),
	}

	if len(item.Suggestions) > 0 {
		risk.Suggestion = strings.Join(item.Suggestions, "; ")
	}
	if len(item.Details) > 0 {
		risk.Comments = strings.Join(item.Details, "; ")
	}

	if meta, ok := item.Metadata.(map[string]any); ok {
		risk.Parameter = trimString(stringFromMeta(meta, "target"))
		risk.NewDefault = trimString(stringFromMeta(meta, "default_value"))
		risk.Scope = trimString(stringFromMeta(meta, "scope"))
		if reason := trimString(stringFromMeta(meta, "reason")); reason != "" {
			risk.Reason = reason
		}
		if risk.Reason == "" {
			risk.Reason = trimString(stringFromMeta(meta, "summary"))
		}
		if risk.Comments == "" {
			risk.Comments = trimString(stringFromMeta(meta, "details"))
		}
		if component := trimString(stringFromMeta(meta, "component")); component != "" {
			risk.Component = component
		}
	}

	if risk.Component == "" {
		risk.Component = inferComponent(item.Rule)
	}
	if risk.Scope == "" && item.Rule == "core.forced-global-sysvars" {
		risk.Scope = "Global"
	}

	return risk
}

func mapSeverity(severity prechecklib.Severity) RiskLevel {
	switch severity {
	case prechecklib.SeverityBlocker, prechecklib.SeverityError:
		return RiskHigh
	case prechecklib.SeverityWarning:
		return RiskMedium
	default:
		return RiskLow
	}
}

func mapRuleCategory(rule string) string {
	switch rule {
	case "core.forced-global-sysvars":
		return "Forced Upgrade Logic"
	case "core.target-version-order":
		return "Upgrade Path Validation"
	default:
		return strings.TrimSpace(rule)
	}
}

func inferComponent(rule string) string {
	switch rule {
	case "core.forced-global-sysvars":
		return "TiDB"
	default:
		return ""
	}
}

func stringFromMeta(meta map[string]any, key string) string {
	value, ok := meta[key]
	if !ok || value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case fmt.Stringer:
		return v.String()
	case []byte:
		return string(v)
	default:
		return fmt.Sprint(v)
	}
}

func trimString(s string) string {
	return strings.TrimSpace(s)
}
