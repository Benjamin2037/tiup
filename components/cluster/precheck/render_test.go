package precheck

import (
	"strings"
	"testing"
)

func sampleReport() *RiskReport {
	return &RiskReport{
		SourceVersion: "v6.5.0",
		TargetVersion: "v7.1.0",
		High: []RiskItem{{
			Level:      RiskHigh,
			Category:   "Config Deprecated",
			Component:  "TiKV",
			Parameter:  "raftstore.notify-capacity",
			Scope:      "TiKV",
			Current:    "40960",
			Impact:     "Removed in target version",
			Suggestion: "Remove from config",
			Comments:   "Adjust capacity planning",
		}},
		Medium: []RiskItem{{
			Level:      RiskMedium,
			Component:  "TiDB",
			Parameter:  "tidb_index_join_batch_size",
			Current:    "256",
			NewDefault: "1024",
			Impact:     "Retains old default",
			Suggestion: "Consider updating",
		}},
		Low: nil,
	}
}

func TestRenderReportMarkdown(t *testing.T) {
	out := sampleReport()
	payload, err := RenderReport(out, OutputMarkdown)
	if err != nil {
		t.Fatalf("RenderReport markdown failed: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "# TiDB Upgrade Precheck Report") {
		t.Fatalf("markdown output missing header: %s", text)
	}
	if !strings.Contains(text, "raftstore.notify-capacity") {
		t.Fatalf("markdown output missing risk item: %s", text)
	}
}

func TestRenderReportHTML(t *testing.T) {
	payload, err := RenderReport(sampleReport(), OutputHTML)
	if err != nil {
		t.Fatalf("RenderReport html failed: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "<!DOCTYPE html>") {
		t.Fatalf("html output missing doctype: %s", text)
	}
	if !strings.Contains(text, "raftstore.notify-capacity") {
		t.Fatalf("html output missing risk item: %s", text)
	}
}

func TestParseOutputFormat(t *testing.T) {
	cases := map[string]OutputFormat{
		"":         OutputText,
		"text":     OutputText,
		"md":       OutputMarkdown,
		"markdown": OutputMarkdown,
		"html":     OutputHTML,
		"htm":      OutputHTML,
	}
	for input, expect := range cases {
		got, err := ParseOutputFormat(input)
		if err != nil {
			t.Fatalf("ParseOutputFormat(%q) returned error: %v", input, err)
		}
		if got != expect {
			t.Fatalf("ParseOutputFormat(%q) = %v, expected %v", input, got, expect)
		}
	}
	if _, err := ParseOutputFormat("json"); err == nil {
		t.Fatalf("expected error for unsupported format")
	}
}
