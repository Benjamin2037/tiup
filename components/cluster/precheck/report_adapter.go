// Copyright 2025 PingCAP, Inc.
//
// Adapter to convert tiup-cluster RiskReport to tidb-upgrade-precheck/pkg/report.Report
package precheck

import (
	"time"

	report "github.com/pingcap/tidb-upgrade-precheck/pkg/report"
)

// ToUnifiedReport converts a tiup-cluster RiskReport to tidb-upgrade-precheck/pkg/report.Report
func ToUnifiedReport(r *RiskReport, clusterName, upgradePath string) *report.Report {
	summary := map[report.RiskLevel]int{
		report.RiskHigh:   len(r.High),
		report.RiskMedium: len(r.Medium),
		report.RiskInfo:   len(r.Low), // Map Low to Info
	}
	var risks []report.RiskItem
	for _, item := range r.High {
		risks = append(risks, convertRiskItem(item, report.RiskHigh))
	}
	for _, item := range r.Medium {
		risks = append(risks, convertRiskItem(item, report.RiskMedium))
	}
	for _, item := range r.Low {
		risks = append(risks, convertRiskItem(item, report.RiskInfo))
	}
	return &report.Report{
		ClusterName: clusterName,
		UpgradePath: upgradePath,
		Summary:     summary,
		Risks:       risks,
		Audits:      nil, // Not available in old RiskReport
		GeneratedAt: time.Now().Format(time.RFC3339),
	}
}

func convertRiskItem(item RiskItem, level report.RiskLevel) report.RiskItem {
	return report.RiskItem{
		Component:  item.Component,
		Parameter:  item.Parameter,
		Current:    item.Current,
		Target:     item.NewDefault,
		Level:      level,
		Impact:     item.Impact,
		Suggestion: item.Suggestion,
		RDComment:  item.Comments,
	}
}
