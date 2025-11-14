package precheck

import (
	"bytes"
	"strings"
	"testing"
)

func TestPrintReportToConsole_Basic(t *testing.T) {
	report := &RiskReport{
		SourceVersion: "v6.5.0",
		TargetVersion: "v7.1.0",
		High: []RiskItem{{
			Level:      RiskHigh,
			Category:   "Config",
			Component:  "TiKV",
			Parameter:  "rocksdb.max-background-jobs",
			Impact:     "Higher compaction concurrency may increase write stall risk",
			Suggestion: "Review compaction settings",
			Comments:   "Ensure capacity planning accounts for higher IO usage.",
		}},
		Medium: []RiskItem{{
			Level:      RiskMedium,
			Category:   "Compatibility",
			Component:  "TiDB",
			Parameter:  "sql_mode",
			Impact:     "Stricter SQL mode may cause errors for legacy apps",
			Suggestion: "Test with new default",
		}},
		Low: nil,
	}

	buf := &bytes.Buffer{}
	restore := OverrideIO(buf, nil)
	defer restore()

	PrintReportToConsole(report)

	out := buf.String()
	for _, expect := range []string{
		"Source Version: v6.5.0",
		"Target Version: v7.1.0",
		"[PRECHECK REPORT - SUMMARY]",
		"[HIGH RISK]",
		"TiKV",
		"rocksdb.max-background-jobs",
		"R&D Comments: Ensure capacity planning accounts for higher IO usage.",
		"[MEDIUM RISK]",
		"TiDB",
		"sql_mode",
	} {
		if !strings.Contains(out, expect) {
			t.Fatalf("expected output to contain %q, got: %s", expect, out)
		}
	}
}
