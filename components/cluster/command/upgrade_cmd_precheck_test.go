package command

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/pingcap/tiup/components/cluster/precheck"
	"github.com/pingcap/tiup/pkg/cluster/spec"
)

func stubClusterMetadata(meta *spec.ClusterMeta, err error) func() {
	original := clusterMetadataFunc
	clusterMetadataFunc = func(string) (*spec.ClusterMeta, error) {
		return meta, err
	}
	return func() { clusterMetadataFunc = original }
}

// Test planning mode (--precheck) exits before attempting an upgrade (clusterUpgradeFunc not invoked).
func TestUpgradeCmdPlanningMode(t *testing.T) {
	called := false
	clusterUpgradeFunc = func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error {
		called = true
		return nil
	}
	restoreMeta := stubClusterMetadata(&spec.ClusterMeta{Version: "v6.5.0"}, nil)
	defer restoreMeta()
	ioRestore := precheck.OverrideIO(bytes.NewBuffer(nil), nil)
	defer ioRestore()
	cmd := newUpgradeCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dummy-cluster", "v7.1.0", "--precheck"})
	if err := cmd.Execute(); err != nil {
		to := buf.String()
		t.Fatalf("unexpected error: %v output=%s", err, to)
	}
	if called {
		to := buf.String()
		t.Fatalf("upgrade invoked in planning mode. Output=%s", to)
	}
}

// Test execute mode with interactive "n" aborts before upgrade.
func TestUpgradeCmdExecuteAbort(t *testing.T) {
	called := false
	clusterUpgradeFunc = func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error {
		called = true
		return nil
	}
	restoreMeta := stubClusterMetadata(&spec.ClusterMeta{Version: "v6.5.0"}, nil)
	defer restoreMeta()

	f, err := os.CreateTemp(t.TempDir(), "stdin-abort")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString("n\n")
	_, _ = f.Seek(0, 0)
	restoreIO := precheck.OverrideIO(bytes.NewBuffer(nil), f)
	defer restoreIO()
	defer f.Close()

	cmd := newUpgradeCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dummy-cluster", "v7.1.0"}) // default execute mode
	if err := cmd.Execute(); err != nil {
		to := buf.String()
		t.Fatalf("unexpected error executing command: %v output=%s", err, to)
	}
	if called {
		to := buf.String()
		t.Fatalf("upgrade invoked despite user abort. Output=%s", to)
	}
	// No specific output assertion: logger writes to global printer, not this buffer.
}

// Test skip precheck (--without-precheck) path invokes upgrade.
func TestUpgradeCmdSkipPrecheck(t *testing.T) {
	called := false
	clusterUpgradeFunc = func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error {
		called = true
		return nil
	}
	restoreMeta := stubClusterMetadata(&spec.ClusterMeta{Version: "v6.5.0"}, nil)
	defer restoreMeta()
	origSkip := skipConfirm
	skipConfirm = true
	defer func() { skipConfirm = origSkip }()

	cmd := newUpgradeCmd()
	buf := bytes.NewBuffer(nil)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"dummy-cluster", "v7.1.0", "--without-precheck"})
	if err := cmd.Execute(); err != nil {
		to := buf.String()
		t.Fatalf("unexpected error executing skip precheck path: %v output=%s", err, to)
	}
	if !called {
		to := buf.String()
		t.Fatalf("expected upgrade invocation. Output=%s", to)
	}
}

// Test standalone `tiup cluster upgrade precheck` subcommand runs precheck without invoking upgrade.
func TestUpgradePrecheckSubcommand(t *testing.T) {
	called := false
	clusterUpgradeFunc = func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error {
		called = true
		return nil
	}
	defer func() { clusterUpgradeFunc = nil }()
	// Stub metadata lookup
	restoreMeta := stubClusterMetadata(&spec.ClusterMeta{Version: "v6.5.0"}, nil)
	defer restoreMeta()

	buf := bytes.NewBuffer(nil)
	restoreIO := precheck.OverrideIO(buf, nil)
	defer restoreIO()

	cmd := newUpgradeCmd()
	cmd.SetOut(bytes.NewBuffer(nil))
	cmd.SetArgs([]string{"precheck", "dummy-cluster", "v7.1.0"})
	if err := cmd.Execute(); err != nil {
		to := buf.String()
		t.Fatalf("precheck subcommand returned error: %v output=%s", err, to)
	}
	if called {
		to := buf.String()
		t.Fatalf("upgrade should not be invoked from precheck subcommand. Output=%s", to)
	}
	if !strings.Contains(buf.String(), "[PRECHECK REPORT - SUMMARY]") {
		t.Fatalf("expected precheck report in output, got: %s", buf.String())
	}
}
