// Copyright 2020 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package command

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pingcap/tiup/pkg/cluster/spec"
	logprinter "github.com/pingcap/tiup/pkg/logger/printer"
	"github.com/pingcap/tiup/pkg/tui"
	"github.com/pingcap/tiup/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/pingcap/tiup/pkg/cluster/manager"
)

// RiskReport aggregates all risks found during the precheck.
// This is a simplified structure for TiUP compatibility
type RiskReport struct {
	SourceVersion string
	TargetVersion string
	Items         []RiskItem
	ReportPath    string // Path to the generated report file
}

// RiskItem describes a single detected risk.
type RiskItem struct {
	Severity string
	Message  string
	Details  string
}

// tidbSQLCredentials holds credentials for connecting to TiDB
type tidbSQLCredentials struct {
	User     string
	Password string
}

// highRiskItemOperation represents a single high-risk parameter operation
type highRiskItemOperation struct {
	Operation     string        `json:"operation"`      // "add", "modify", or "remove"
	Component     string        `json:"component"`      // "tidb", "pd", "tikv", "tiflash"
	Type          string        `json:"type"`           // "config" or "system_variable"
	Name          string        `json:"name"`           // parameter name
	Severity      string        `json:"severity,omitempty"`
	Description   string        `json:"description,omitempty"`
	CheckModified bool          `json:"check_modified,omitempty"`
	FromVersion   string        `json:"from_version,omitempty"`
	ToVersion     string        `json:"to_version,omitempty"`
	AllowedValues []interface{} `json:"allowed_values,omitempty"`
}

var clusterUpgradeFunc func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error

func newUpgradeCmd() *cobra.Command {
	offlineMode := false
	ignoreVersionCheck := false
	var tidbVer, tikvVer, pdVer, tsoVer, schedulingVer, tiflashVer, kvcdcVer, dashboardVer, cdcVer, alertmanagerVer, nodeExporterVer, blackboxExporterVer, tiproxyVer string
	var restartTimeout time.Duration
	var precheckOnlyFlag bool
	var withoutPrecheckFlag bool
	var precheckOutputFormat string
	var precheckOutputFile string
	var precheckTiDBUser string
	var precheckTiDBPassword string
	var precheckTiDBPasswordFile string
	var precheckTiDBPromptPassword bool
	var precheckHighRiskParamsConfig string
	var highRiskItemsEditFile string
	var highRiskItemsList bool
	logger := logprinter.NewLogger("")
	cm := manager.NewManager("tidb", tidbSpec, logger)

	// clusterUpgradeFunc enables injection in tests to observe whether an upgrade would execute
	// clusterUpgradeFunc default binding
	if clusterUpgradeFunc == nil {
		clusterUpgradeFunc = func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error {
			return cm.Upgrade(clusterName, version, compVers, gOpt, skip, offline, ignoreVersion, restartTimeout)
		}
	}

	cmd := &cobra.Command{
		Use:   "upgrade <cluster-name> <version>",
		Short: "Upgrade a specified TiDB cluster",
		Long:  `Upgrade a specified TiDB cluster to a newer version.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle high-risk-items management operations first
			if highRiskItemsList {
				return handleHighRiskItemsList(cmd.Context(), logger)
			}
			if highRiskItemsEditFile != "" {
				return handleHighRiskItemsEdit(cmd.Context(), logger, highRiskItemsEditFile)
			}

			// Validate output format
			outputFormat := strings.ToLower(precheckOutputFormat)
			if outputFormat != "text" && outputFormat != "markdown" && outputFormat != "html" && outputFormat != "json" {
				return fmt.Errorf("unsupported output format: %s (supported: text, markdown, html, json)", precheckOutputFormat)
			}

			if len(args) != 2 {
				return cmd.Help()
			}

			clusterName := args[0]
			version, err := utils.FmtVer(args[1])
			if err != nil {
				return err
			}

			resolveCreds := func() (tidbSQLCredentials, error) {
				return buildPrecheckTiDBCredentials(precheckTiDBUser, precheckTiDBPassword, precheckTiDBPasswordFile, precheckTiDBPromptPassword)
			}

			creds, err := resolveCreds()
			if err != nil {
				return err
			}

			// Run precheck unless explicitly skipped
			if !withoutPrecheckFlag {
				report, err := runParameterPrecheck(context.Background(), clusterName, version, logger, outputFormat, precheckOutputFile, creds, precheckHighRiskParamsConfig, cm)
				if err != nil {
					// Even if precheck failed, we still ask for confirmation to continue
					logger.Warnf("Precheck failed but continuing anyway: %v", err)
				}

				// If precheck-only mode, exit after precheck
				if precheckOnlyFlag {
					return nil
				}

				// Ask for confirmation before proceeding with upgrade
				if report != nil && len(report.Items) > 0 {
					logger.Warnf("Precheck found %d potential issues. Please review before continuing.", len(report.Items))
				}

				if !skipConfirm {
					err = tui.PromptForConfirmOrAbortError("Did you backup the cluster data and read the precheck report? Confirm to continue...")
					if err != nil {
						return err
					}
				}
			}

			componentVersions := map[string]string{
				spec.ComponentDashboard:        dashboardVer,
				spec.ComponentAlertmanager:     alertmanagerVer,
				spec.ComponentTiDB:             tidbVer,
				spec.ComponentTiKV:             tikvVer,
				spec.ComponentPD:               pdVer,
				spec.ComponentTSO:              tsoVer,
				spec.ComponentScheduling:       schedulingVer,
				spec.ComponentTiFlash:          tiflashVer,
				spec.ComponentTiKVCDC:          kvcdcVer,
				spec.ComponentCDC:              cdcVer,
				spec.ComponentTiProxy:          tiproxyVer,
				spec.ComponentBlackboxExporter: blackboxExporterVer,
				spec.ComponentNodeExporter:     nodeExporterVer,
			}

			return clusterUpgradeFunc(clusterName, version, componentVersions, skipConfirm, offlineMode, ignoreVersionCheck, restartTimeout)
		},
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			switch len(args) {
			case 0:
				return shellCompGetClusterName(cm, toComplete)
			default:
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
		},
	}

	cmd.Flags().BoolVar(&gOpt.Force, "force", false, "Force upgrade without transferring PD leader")
	cmd.Flags().Uint64Var(&gOpt.APITimeout, "transfer-timeout", 600, "Timeout in seconds when transferring PD and TiKV store leaders, also for TiCDC drain one capture")
	cmd.Flags().BoolVarP(&gOpt.IgnoreConfigCheck, "ignore-config-check", "", false, "Ignore the config check result")
	cmd.Flags().BoolVarP(&offlineMode, "offline", "", false, "Upgrade a stopped cluster")
	cmd.Flags().BoolVarP(&ignoreVersionCheck, "ignore-version-check", "", false, "Ignore checking if target version is bigger than current version")
	cmd.Flags().StringVar(&gOpt.SSHCustomScripts.BeforeRestartInstance.Raw, "pre-upgrade-script", "", "Custom script to be executed on each server before the server is upgraded")
	cmd.Flags().StringVar(&gOpt.SSHCustomScripts.AfterRestartInstance.Raw, "post-upgrade-script", "", "Custom script to be executed on each server after the server is upgraded")

	// cmd.Flags().StringVar(&tidbVer, "tidb-version", "", "Fix the version of tidb and no longer follows the cluster version.")
	cmd.Flags().StringVar(&tikvVer, "tikv-version", "", "Fix the version of tikv and no longer follows the cluster version.")
	cmd.Flags().StringVar(&pdVer, "pd-version", "", "Fix the version of pd and no longer follows the cluster version.")
	cmd.Flags().StringVar(&tsoVer, "tso-version", "", "Fix the version of tso and no longer follows the cluster version.")
	cmd.Flags().StringVar(&schedulingVer, "scheduling-version", "", "Fix the version of scheduling and no longer follows the cluster version.")
	cmd.Flags().StringVar(&tiflashVer, "tiflash-version", "", "Fix the version of tiflash and no longer follows the cluster version.")
	cmd.Flags().StringVar(&dashboardVer, "tidb-dashboard-version", "", "Fix the version of tidb-dashboard and no longer follows the cluster version.")
	cmd.Flags().StringVar(&cdcVer, "cdc-version", "", "Fix the version of cdc and no longer follows the cluster version.")
	cmd.Flags().StringVar(&kvcdcVer, "tikv-cdc-version", "", "Fix the version of tikv-cdc and no longer follows the cluster version.")
	cmd.Flags().StringVar(&alertmanagerVer, "alertmanager-version", "", "Fix the version of alertmanager and no longer follows the cluster version.")
	cmd.Flags().StringVar(&nodeExporterVer, "node-exporter-version", "", "Fix the version of node-exporter and no longer follows the cluster version.")
	cmd.Flags().StringVar(&blackboxExporterVer, "blackbox-exporter-version", "", "Fix the version of blackbox-exporter and no longer follows the cluster version.")
	cmd.Flags().StringVar(&tiproxyVer, "tiproxy-version", "", "Fix the version of tiproxy and no longer follows the cluster version.")
	cmd.Flags().DurationVar(&restartTimeout, "restart-timeout", time.Second*0, "Timeout for after upgrade prompt")
	cmd.Flags().BoolVar(&precheckOnlyFlag, "precheck", false, "Run parameter precheck only, do not perform upgrade")
	cmd.Flags().BoolVar(&withoutPrecheckFlag, "without-precheck", false, "Skip parameter precheck and proceed directly to upgrade (not recommended)")
	cmd.Flags().StringVar(&precheckOutputFormat, "precheck-output", "text", "Format for the precheck report (text, markdown, html)")
	cmd.Flags().StringVar(&precheckOutputFile, "precheck-output-file", "", "Write the precheck report to a file instead of stdout")
	cmd.Flags().StringVar(&precheckTiDBUser, "precheck-tidb-user", "root", "TiDB SQL user for snapshot collection during precheck")
	cmd.Flags().StringVar(&precheckTiDBPassword, "precheck-tidb-password", "", "TiDB SQL password for snapshot collection during precheck")
	cmd.Flags().StringVar(&precheckTiDBPasswordFile, "precheck-tidb-password-file", "", "Path to a file containing the TiDB SQL password for precheck snapshot collection")
	cmd.Flags().BoolVar(&precheckTiDBPromptPassword, "precheck-tidb-password-prompt", false, "Prompt for the TiDB SQL password for precheck snapshot collection")
	cmd.Flags().StringVar(&precheckHighRiskParamsConfig, "precheck-high-risk-params-config", "", "Path to high-risk parameters configuration file (JSON format). If not specified, will try to load from ~/.tiup/high_risk_params.json")
	cmd.Flags().StringVar(&highRiskItemsEditFile, "edit", "", "Edit high-risk parameters from a JSON file (use with --high-risk-items). File should contain operations: add, modify, or remove")
	cmd.Flags().BoolVar(&highRiskItemsList, "list", false, "List current high-risk parameters (use with --high-risk-items)")
	cmd.Flags().Bool("high-risk-items", false, "Manage high-risk parameters configuration (use with --edit or --list)")
	return cmd
}

// runParameterPrecheck runs the parameter precheck using tidb-upgrade-precheck binary.
func runParameterPrecheck(ctx context.Context, clusterName, targetVersion string, logger *logprinter.Logger, format string, outputPath string, creds tidbSQLCredentials, highRiskParamsConfig string, cm *manager.Manager) (*RiskReport, error) {
	logger.Infof("Running parameter precheck...")

	// Step 0: Locate tidb-upgrade-precheck binary
	// Priority: environment variable > same directory as binary > PATH
	var binPath string
	if precheckPath := os.Getenv("TIDB_UPGRADE_PRECHECK_BIN"); precheckPath != "" {
		binPath = precheckPath
		logger.Debugf("Using tidb-upgrade-precheck from environment: %s", precheckPath)
	} else {
		exePath, err := os.Executable()
		if err == nil {
			binPath = filepath.Join(filepath.Dir(exePath), "tidb-upgrade-precheck")
			if _, err := os.Stat(binPath); os.IsNotExist(err) {
				binPath = "tidb-upgrade-precheck"
				logger.Warnf("tidb-upgrade-precheck not found in cluster directory, trying PATH")
			}
		} else {
			binPath = "tidb-upgrade-precheck"
		}
	}

	// Step 0.1: Locate knowledge base directory
	// tidb-upgrade-precheck looks for "knowledge" directory in current working directory
	// Priority: environment variable > TiUP profile > same directory as binary
	var knowledgeDir string
	if kbPath := os.Getenv("TIDB_UPGRADE_PRECHECK_KB"); kbPath != "" {
		knowledgeDir = kbPath
	} else {
		// Try TiUP profile directory first
		knowledgeDir = spec.ProfilePath("knowledge")
		if _, err := os.Stat(knowledgeDir); os.IsNotExist(err) {
			// Fallback to same directory as binary
			exePath, err := os.Executable()
			if err == nil {
				knowledgeDir = filepath.Join(filepath.Dir(exePath), "knowledge")
			}
		}
	}

	// Step 1: Get cluster metadata and topology
	metadata, err := spec.ClusterMetadata(clusterName)
	if err != nil {
		return nil, fmt.Errorf("failed to read cluster metadata: %v", err)
	}

	sourceVersion := metadata.Version
	topo := metadata.GetTopology()
	clusterTopo, ok := topo.(*spec.Specification)
	if !ok {
		return nil, fmt.Errorf("invalid topology type")
	}

	// Step 2: Create topology file for tidb-upgrade-precheck
	precheckBaseDir := spec.ProfilePath("upgrade_precheck")
	precheckTmpDir := filepath.Join(precheckBaseDir, "tmp")
	precheckReportsDir := filepath.Join(precheckBaseDir, "reports")

	if err := os.MkdirAll(precheckTmpDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create precheck tmp directory: %v", err)
	}
	if err := os.MkdirAll(precheckReportsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create precheck reports directory: %v", err)
	}

	// Convert topology to YAML and add source version
	topologyData, err := yaml.Marshal(clusterTopo)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal topology: %v", err)
	}

	if sourceVersion != "" {
		var originalTopo map[string]interface{}
		if err := yaml.Unmarshal(topologyData, &originalTopo); err == nil {
			topologyWithVersion := map[string]interface{}{
				"global": map[string]interface{}{
					"version": sourceVersion,
				},
			}
			for k, v := range originalTopo {
				topologyWithVersion[k] = v
			}
			topologyData, err = yaml.Marshal(topologyWithVersion)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal topology with version: %v", err)
			}
		}
	}

	topologyFile := filepath.Join(precheckTmpDir, fmt.Sprintf("topology-%s-%d.yaml", clusterName, time.Now().Unix()))
	if err := os.WriteFile(topologyFile, topologyData, 0644); err != nil {
		return nil, fmt.Errorf("failed to write topology file: %v", err)
	}

	// Step 3: Build command to call tidb-upgrade-precheck
	cmd := exec.CommandContext(ctx, binPath,
		"--topology-file", topologyFile,
		"--target-version", targetVersion,
		"--format", format,
		"--output-dir", precheckReportsDir,
	)

	if sourceVersion != "" {
		cmd.Args = append(cmd.Args, "--source-version", sourceVersion)
	}
	if creds.User != "" {
		cmd.Args = append(cmd.Args, "--tidb-user", creds.User)
	}
	if creds.Password != "" {
		cmd.Args = append(cmd.Args, "--tidb-password", creds.Password)
	}

	// Add high-risk parameters config
	if highRiskParamsConfig != "" {
		cmd.Args = append(cmd.Args, "--high-risk-params-config", highRiskParamsConfig)
	} else {
		// Try default location
		if homeDir, err := os.UserHomeDir(); err == nil {
			defaultConfigPath := filepath.Join(homeDir, ".tiup", "high_risk_params.json")
			if _, err := os.Stat(defaultConfigPath); err == nil {
				cmd.Args = append(cmd.Args, "--high-risk-params-config", defaultConfigPath)
			}
		}
	}

	// Set working directory so tidb-upgrade-precheck can find knowledge base
	// tidb-upgrade-precheck looks for "knowledge" directory in current working directory
	if knowledgeDir != "" {
		if _, err := os.Stat(knowledgeDir); err == nil {
			// Set working directory to the parent of knowledge directory
			// so that "knowledge" subdirectory is accessible
			cmd.Dir = filepath.Dir(knowledgeDir)
		}
	}

	// Step 4: Execute command and capture output
	logger.Infof("Executing: %s", strings.Join(cmd.Args, " "))
	cmdOutput, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(cmdOutput))
	}

	// Step 5: Extract report file path from command output
	outputStr := string(cmdOutput)
	reportPath := ""

	// Try to extract report path from output: "Report generated successfully: {path}"
	re := regexp.MustCompile(`Report generated successfully:\s*(.+\.(txt|md|html|json))`)
	if matches := re.FindStringSubmatch(outputStr); len(matches) > 1 {
		reportPath = strings.TrimSpace(matches[1])
		if !filepath.IsAbs(reportPath) {
			reportPath = filepath.Join(precheckReportsDir, reportPath)
		}
	} else {
		// Fallback: find most recent report file
		pattern := filepath.Join(precheckReportsDir, fmt.Sprintf("upgrade_precheck_report_*.%s", getFileExtension(format)))
		if matches, err := filepath.Glob(pattern); err == nil && len(matches) > 0 {
			var latestFile string
			var latestTime time.Time
			for _, match := range matches {
				if info, err := os.Stat(match); err == nil && info.ModTime().After(latestTime) {
					latestTime = info.ModTime()
					latestFile = match
				}
			}
			reportPath = latestFile
		}
	}

	// Step 6: Display report
	if reportPath != "" {
		logger.Infof("Precheck report generated: %s", reportPath)
		if outputPath == "" {
			if content, err := os.ReadFile(reportPath); err == nil {
				fmt.Println(string(content))
			}
		}
	} else if outputPath == "" {
		fmt.Println(string(cmdOutput))
	}

	return &RiskReport{
		SourceVersion: sourceVersion,
		TargetVersion: targetVersion,
		ReportPath:    reportPath,
	}, nil
}

// getFileExtension returns the file extension for a given format
func getFileExtension(format string) string {
	switch format {
	case "text":
		return "txt"
	case "markdown":
		return "md"
	case "html":
		return "html"
	case "json":
		return "json"
	default:
		return "txt"
	}
}

// buildPrecheckTiDBCredentials resolves TiDB credentials for precheck
func buildPrecheckTiDBCredentials(username, password, passwordFile string, prompt bool) (tidbSQLCredentials, error) {
	resolved := ""
	switch {
	case password != "":
		resolved = password
	case passwordFile != "":
		content, err := os.ReadFile(passwordFile)
		if err != nil {
			return tidbSQLCredentials{}, err
		}
		resolved = strings.TrimRight(string(content), "\n\r ")
	case prompt:
		// Prompt for password using TiUP's tui package
		resolved = tui.PromptForPassword("Enter password for TiDB user [%s]: ", username)
	}

	return tidbSQLCredentials{User: username, Password: resolved}, nil
}

// handleHighRiskItemsList lists current high-risk parameters
func handleHighRiskItemsList(ctx context.Context, logger *logprinter.Logger) error {
	// Locate tidb-upgrade-precheck binary
	binPath, err := locatePrecheckBinary(logger)
	if err != nil {
		return err
	}

	// Determine runtime storage directory for upgrade-precheck
	runtimeDir := spec.ProfilePath("upgrade-precheck")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("failed to create upgrade-precheck runtime directory: %v", err)
	}

	configFile := filepath.Join(runtimeDir, "high_risk_params.json")

	// Call tidb-upgrade-precheck high-risk-params list
	cmd := exec.CommandContext(ctx, binPath, "high-risk-params", "list",
		"--config", configFile,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(output))
	}

	fmt.Print(string(output))
	return nil
}

// handleHighRiskItemsEdit processes edit operations from a JSON file
func handleHighRiskItemsEdit(ctx context.Context, logger *logprinter.Logger, editFile string) error {
	// Locate tidb-upgrade-precheck binary
	binPath, err := locatePrecheckBinary(logger)
	if err != nil {
		return err
	}

	// Determine runtime storage directory for upgrade-precheck
	runtimeDir := spec.ProfilePath("upgrade-precheck")
	if err := os.MkdirAll(runtimeDir, 0755); err != nil {
		return fmt.Errorf("failed to create upgrade-precheck runtime directory: %v", err)
	}

	configFile := filepath.Join(runtimeDir, "high_risk_params.json")
	logger.Infof("Using high-risk parameters config file: %s", configFile)

	// Read and parse the edit file
	data, err := os.ReadFile(editFile)
	if err != nil {
		return fmt.Errorf("failed to read edit file %s: %v", editFile, err)
	}

	var editData struct {
		Operations []highRiskItemOperation `json:"operations"`
	}

	// Try JSON first, then YAML
	if err := json.Unmarshal(data, &editData); err != nil {
		if err2 := yaml.Unmarshal(data, &editData); err2 != nil {
			return fmt.Errorf("failed to parse edit file %s (tried JSON and YAML): %v, %v", editFile, err, err2)
		}
	}

	// Process each operation
	for _, op := range editData.Operations {
		if err := processHighRiskItemOperation(ctx, binPath, configFile, op, logger); err != nil {
			logger.Warnf("Failed to process operation %s for %s/%s/%s: %v", op.Operation, op.Component, op.Type, op.Name, err)
			// Continue processing other operations
		} else {
			logger.Infof("Successfully processed %s operation for %s/%s/%s", op.Operation, op.Component, op.Type, op.Name)
		}
	}

	logger.Infof("Completed processing edit file: %s", editFile)
	return nil
}

// locatePrecheckBinary locates the tidb-upgrade-precheck binary
func locatePrecheckBinary(logger *logprinter.Logger) (string, error) {
	var binPath string
	if precheckPath := os.Getenv("TIDB_UPGRADE_PRECHECK_BIN"); precheckPath != "" {
		binPath = precheckPath
		logger.Debugf("Using tidb-upgrade-precheck from environment: %s", precheckPath)
	} else {
		exePath, err := os.Executable()
		if err == nil {
			binPath = filepath.Join(filepath.Dir(exePath), "tidb-upgrade-precheck")
			if _, err := os.Stat(binPath); os.IsNotExist(err) {
				binPath = "tidb-upgrade-precheck"
				logger.Warnf("tidb-upgrade-precheck not found in cluster directory, trying PATH")
			}
		} else {
			binPath = "tidb-upgrade-precheck"
		}
	}
	return binPath, nil
}

// processHighRiskItemOperation processes a single high-risk item operation
func processHighRiskItemOperation(ctx context.Context, binPath, configFile string, op highRiskItemOperation, logger *logprinter.Logger) error {
	var cmd *exec.Cmd

	switch op.Operation {
	case "remove":
		// For remove, we only need component, type, and name
		cmd = exec.CommandContext(ctx, binPath, "high-risk-params", "remove",
			"--config", configFile,
			"--component", op.Component,
			"--type", op.Type,
			"--name", op.Name,
		)
	case "add", "modify":
		// For add/modify, we need all parameter details
		cmd = exec.CommandContext(ctx, binPath, "high-risk-params", "add",
			"--config", configFile,
			"--component", op.Component,
			"--type", op.Type,
			"--name", op.Name,
		)

		// Add parameter fields
		if op.Severity != "" {
			cmd.Args = append(cmd.Args, "--severity", op.Severity)
		}
		if op.Description != "" {
			cmd.Args = append(cmd.Args, "--description", op.Description)
		}
		if op.CheckModified {
			cmd.Args = append(cmd.Args, "--check-modified")
		}
		if op.FromVersion != "" {
			cmd.Args = append(cmd.Args, "--from-version", op.FromVersion)
		}
		if op.ToVersion != "" {
			cmd.Args = append(cmd.Args, "--to-version", op.ToVersion)
		}
		if len(op.AllowedValues) > 0 {
			var allowedStr []string
			for _, v := range op.AllowedValues {
				allowedStr = append(allowedStr, fmt.Sprintf("%v", v))
			}
			cmd.Args = append(cmd.Args, "--allowed-values", strings.Join(allowedStr, ","))
		}
	default:
		return fmt.Errorf("unsupported operation: %s (must be 'add', 'modify', or 'remove')", op.Operation)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(output))
	}

	logger.Debugf("Operation %s for %s/%s/%s completed: %s", op.Operation, op.Component, op.Type, op.Name, string(output))
	return nil
}

