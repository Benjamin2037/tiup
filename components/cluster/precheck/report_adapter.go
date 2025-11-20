// Copyright 2025 PingCAP, Inc.
//
// Adapter to convert tiup-cluster RiskReport to tidb-upgrade-precheck/pkg/report.Report
package precheck

import (
	"fmt"
	"time"

	"github.com/pingcap/tidb-upgrade-precheck/knowledge"
	report "github.com/pingcap/tidb-upgrade-precheck/pkg/report"
)

// ToUnifiedReport converts a tiup-cluster RiskReport to tidb-upgrade-precheck/pkg/report.Report
// ToUnifiedReport converts a tiup-cluster RiskReport to tidb-upgrade-precheck/pkg/report.Report
// upgradePath: e.g. "v7.5.1 -> v8.5.3"
func ToUnifiedReport(r *RiskReport, clusterName, upgradePath string, sourceVersion, targetVersion string) *report.Report {
	// Only count true UserSet (Low/INFO) as audits
	var audits []report.AuditItem
	for _, item := range r.Low {
		// Only count parameters explicitly set by user (UserSet == true)
		if item.Parameter == "" || !item.UserSet {
			continue
		}
		audits = append(audits, report.AuditItem{
			Component: item.Component,
			Parameter: item.Parameter,
			Current:   item.Current,
			Target:    item.NewDefault,
			Status:    "User Custom",
		})
	}
	summary := map[report.RiskLevel]int{
		report.RiskHigh:   len(r.High),
		report.RiskMedium: len(r.Medium),
		report.RiskInfo:   len(audits), // Only count true UserSet
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
	// Try to get bootstrap version for both source and target
	var path string
	srcBV, srcOK, _ := knowledge.BootstrapVersion(sourceVersion)
	tgtBV, tgtOK, _ := knowledge.BootstrapVersion(targetVersion)
	if srcOK && tgtOK {
		path = fmt.Sprintf("%s (Bootstrap: %d) -> %s (Bootstrap: %d)", sourceVersion, srcBV, targetVersion, tgtBV)
	} else {
		path = upgradePath
	}
	return &report.Report{
		ClusterName: clusterName,
		UpgradePath: path,
		Summary:     summary,
		Risks:       risks,
		Audits:      audits,
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
