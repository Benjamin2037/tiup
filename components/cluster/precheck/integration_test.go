package precheck

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRunForcedSysvarWarning(t *testing.T) {
	report, err := Run(context.Background(), Input{
		SourceVersion: "v6.5.0",
		TargetVersion: "v7.1.0",
	})
	require.NoError(t, err)

	var found bool
	for _, item := range report.Medium {
		if strings.EqualFold(item.Parameter, "tidb_server_memory_limit") {
			found = true
			require.Equal(t, RiskMedium, item.Level)
			require.Contains(t, item.Impact, "tidb_server_memory_limit")
			require.Contains(t, strings.ToLower(item.Category), "forced")
			break
		}
	}
	require.True(t, found, "expected forced sysvar warning for tidb_server_memory_limit")
}

func TestRunMissingTargetVersionWarning(t *testing.T) {
	report, err := Run(context.Background(), Input{
		SourceVersion: "v7.5.0",
		TargetVersion: "",
	})
	require.NoError(t, err)

	var warningFound bool
	for _, item := range report.Medium {
		if strings.Contains(item.Impact, "目标版本为空") {
			warningFound = true
			require.Equal(t, RiskMedium, item.Level)
			break
		}
	}
	require.True(t, warningFound, "expected warning for missing target version")
}
