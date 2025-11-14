package precheck

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// RunPrecheckForUpgrade is the entry point used by tiup before an upgrade.
// It currently only passes source/target versions; collectors for live
// cluster variables and component configs will extend the Input later.
func RunPrecheckForUpgrade(ctx context.Context, sourceVersion, targetVersion string) (*RiskReport, error) {
	in := Input{
		SourceVersion: sourceVersion,
		TargetVersion: targetVersion,
	}
	return Run(ctx, in)
}

// PrintReportToConsole renders a human-readable risk report to stdout.
var (
	stdOut io.Writer = os.Stdout
	stdIn  io.Reader = os.Stdin
)

func PrintReportToConsole(r *RiskReport) {
	renderTextReport(stdOut, r)
}

// AskForUserConfirmation prompts the operator for confirmation in the
// default "execute" mode. Returns true to proceed, false to abort.

func AskForUserConfirmation() (bool, error) {
	reader := bufio.NewReader(stdIn)
	if _, err := fmt.Fprintf(stdOut, "Do you want to continue with the upgrade? [y/N]: "); err != nil {
		return false, err
	}
	text, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	text = strings.TrimSpace(strings.ToLower(text))
	return text == "y" || text == "yes", nil
}

// OverrideIO swaps the output and input streams used by this package.
// Passing nil for either parameter leaves it unchanged. The returned
// function restores the previous streams and should be deferred by callers.
func OverrideIO(out io.Writer, in io.Reader) func() {
	prevOut := stdOut
	prevIn := stdIn
	if out != nil {
		stdOut = out
	}
	if in != nil {
		stdIn = in
	}
	return func() {
		stdOut = prevOut
		stdIn = prevIn
	}
}

func renderTextReport(w io.Writer, r *RiskReport) {
	fmt.Fprintln(w, "Running parameter precheck...")
	fmt.Fprintf(w, "  Source Version: %s\n", nullIfEmpty(r.SourceVersion))
	fmt.Fprintf(w, "  Target Version: %s\n\n", nullIfEmpty(r.TargetVersion))

	summary := r.Summary()
	fmt.Fprintln(w, "[PRECHECK REPORT - SUMMARY]")
	fmt.Fprintf(w, "Found %d potential risks:\n", summary.Total)
	fmt.Fprintf(w, "  - [HIGH RISK]: %d\n", summary.High)
	fmt.Fprintf(w, "  - [MEDIUM RISK]: %d\n\n", summary.Medium)

	if len(r.High) > 0 {
		fmt.Fprintln(w, "-------------------------------------------------------------------")
		for _, it := range r.High {
			printRiskItem(w, it)
		}
	}
	if len(r.Medium) > 0 {
		fmt.Fprintln(w, "-------------------------------------------------------------------")
		for _, it := range r.Medium {
			printRiskItem(w, it)
		}
	}
	if len(r.Low) > 0 {
		fmt.Fprintln(w, "-------------------------------------------------------------------")
		for _, it := range r.Low {
			printRiskItem(w, it)
		}
	}

	if summary.Total == 0 {
		fmt.Fprintln(w, "No parameter risks detected.")
	}
	fmt.Fprintln(w, "-------------------------------------------------------------------")
}

func printRiskItem(w io.Writer, it RiskItem) {
	tag := strings.ToUpper(string(it.Level))
	if it.Category != "" {
		fmt.Fprintf(w, "[%s] (%s)\n", tag, it.Category)
	} else {
		fmt.Fprintf(w, "[%s]\n", tag)
	}
	if it.Component != "" {
		fmt.Fprintf(w, "  - Component: %s\n", it.Component)
	}
	if it.Parameter != "" {
		fmt.Fprintf(w, "  - Parameter: %s\n", it.Parameter)
	}
	if it.Scope != "" {
		fmt.Fprintf(w, "  - Scope: %s\n", it.Scope)
	}
	if it.Current != "" {
		fmt.Fprintf(w, "  - Current: %s\n", it.Current)
	}
	if it.NewDefault != "" {
		fmt.Fprintf(w, "  - New Default: %s\n", it.NewDefault)
	}
	if it.Impact != "" {
		fmt.Fprintf(w, "  - Impact: %s\n", it.Impact)
	}
	if it.Suggestion != "" {
		fmt.Fprintf(w, "  - Suggestion: %s\n", it.Suggestion)
	}
	if it.Comments != "" {
		fmt.Fprintf(w, "  - R&D Comments: %s\n", it.Comments)
	}
	if it.Reason != "" {
		fmt.Fprintf(w, "  - Reason: %s\n", it.Reason)
	}
	fmt.Fprintln(w)
}

func nullIfEmpty(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}
