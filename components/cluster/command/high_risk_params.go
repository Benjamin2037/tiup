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
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pingcap/tiup/pkg/cluster/spec"
	logprinter "github.com/pingcap/tiup/pkg/logger/printer"
	"github.com/pingcap/tiup/pkg/tui"
	"github.com/spf13/cobra"
)

func newHighRiskParamsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "high-risk-params",
		Short: "Manage high-risk parameters configuration for upgrade precheck",
		Long: `Manage high-risk parameters configuration for upgrade precheck.

This command allows you to add, view, list, edit, and remove high-risk parameters
for each component (TiDB, PD, TiKV, TiFlash). The configuration is saved to a
JSON file that can be used by the upgrade precheck tool.`,
	}

	logger := logprinter.NewLogger("")

	// Add subcommands
	cmd.AddCommand(newHighRiskParamsAddCmd(logger))
	cmd.AddCommand(newHighRiskParamsListCmd(logger))
	cmd.AddCommand(newHighRiskParamsViewCmd(logger))
	cmd.AddCommand(newHighRiskParamsRemoveCmd(logger))
	cmd.AddCommand(newHighRiskParamsEditCmd(logger))

	return cmd
}

// getHighRiskParamsConfigPath returns the path to high-risk params config file
func getHighRiskParamsConfigPath() string {
	// Priority: environment variable > TiUP profile > default locations
	if path := os.Getenv("TIDB_UPGRADE_PRECHECK_HIGH_RISK_PARAMS_CONFIG"); path != "" {
		return path
	}

	// TiUP profile directory (preferred)
	runtimeDir := spec.ProfilePath("upgrade-precheck")
	tiupPath := filepath.Join(runtimeDir, "high_risk_params.json")
	return tiupPath
}

// locatePrecheckBinaryForHRP locates the tidb-upgrade-precheck binary for high-risk-params commands
func locatePrecheckBinaryForHRP(logger *logprinter.Logger) (string, error) {
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
				logger.Debugf("tidb-upgrade-precheck not found in cluster directory, trying PATH")
			}
		} else {
			binPath = "tidb-upgrade-precheck"
		}
	}
	return binPath, nil
}

// newHighRiskParamsAddCmd creates the add subcommand
func newHighRiskParamsAddCmd(logger *logprinter.Logger) *cobra.Command {
	var (
		component     string
		paramType     string
		paramName     string
		severity      string
		description   string
		checkModified bool
		fromVersion   string
		toVersion     string
		allowedValues []string
		interactive   bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a high-risk parameter",
		Long: `Add a high-risk parameter to the configuration.

You can either use interactive mode (default) or provide all parameters via command line flags.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			binPath, err := locatePrecheckBinaryForHRP(logger)
			if err != nil {
				return err
			}

			configPath := getHighRiskParamsConfigPath()

			// Ensure directory exists
			if err := os.MkdirAll(filepath.Dir(configPath), 0755); err != nil {
				return fmt.Errorf("failed to create config directory: %v", err)
			}

			var cmdArgs []string

			if interactive || (component == "" && paramType == "" && paramName == "") {
				// Interactive mode
				logger.Infof("Interactive mode: Adding high-risk parameter")
				component, paramType, paramName, severity, description, checkModified, fromVersion, toVersion, allowedValues, err = promptAddParameter()
				if err != nil {
					return err
				}
			}

			// Build command
			cmdArgs = []string{
				"high-risk-params", "add",
				"--config", configPath,
				"--component", component,
				"--type", paramType,
				"--name", paramName,
			}

			if severity != "" {
				cmdArgs = append(cmdArgs, "--severity", severity)
			}
			if description != "" {
				cmdArgs = append(cmdArgs, "--description", description)
			}
			if checkModified {
				cmdArgs = append(cmdArgs, "--check-modified")
			}
			if fromVersion != "" {
				cmdArgs = append(cmdArgs, "--from-version", fromVersion)
			}
			if toVersion != "" {
				cmdArgs = append(cmdArgs, "--to-version", toVersion)
			}
			if len(allowedValues) > 0 {
				cmdArgs = append(cmdArgs, "--allowed-values", strings.Join(allowedValues, ","))
			}

			// Execute command
			execCmd := exec.CommandContext(cmd.Context(), binPath, cmdArgs...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(output))
			}

			fmt.Print(string(output))
			logger.Infof("Configuration saved to: %s", configPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "Component name (tidb, pd, tikv, tiflash)")
	cmd.Flags().StringVar(&paramType, "type", "", "Parameter type (config or system_variable)")
	cmd.Flags().StringVar(&paramName, "name", "", "Parameter name")
	cmd.Flags().StringVar(&severity, "severity", "", "Severity level (error, warning, info)")
	cmd.Flags().StringVar(&description, "description", "", "Description of why this parameter is high-risk")
	cmd.Flags().BoolVar(&checkModified, "check-modified", false, "Only check if parameter is modified from default")
	cmd.Flags().StringVar(&fromVersion, "from-version", "", "From version (e.g., v7.0.0)")
	cmd.Flags().StringVar(&toVersion, "to-version", "", "To version (e.g., v7.5.0)")
	cmd.Flags().StringSliceVar(&allowedValues, "allowed-values", []string{}, "Allowed values (comma-separated)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Use interactive mode (default if no flags provided)")

	return cmd
}

// promptAddParameter prompts user to add a parameter interactively
func promptAddParameter() (component, paramType, paramName, severity, description string, checkModified bool, fromVersion, toVersion string, allowedValues []string, err error) {
	// Component
	component = promptSelect("Select component:", []string{"tidb", "pd", "tikv", "tiflash"}, "tidb")

	// Parameter type
	var paramTypeOptions []string
	if component == "tidb" {
		paramTypeOptions = []string{"config", "system_variable"}
	} else {
		paramTypeOptions = []string{"config"}
	}
	paramType = promptSelect("Select parameter type:", paramTypeOptions, "config")

	// Parameter name
	paramName = tui.Prompt("Enter parameter name: ")
	if paramName == "" {
		return "", "", "", "", "", false, "", "", nil, fmt.Errorf("parameter name is required")
	}

	// Severity
	severity = promptSelect("Select severity:", []string{"error", "warning", "info"}, "warning")

	// Description
	description = tui.Prompt("Enter description (optional): ")

	// Check modified
	checkModified = promptYesNo("Only check if parameter is modified from default?", false)

	// From version
	fromVersion = tui.Prompt("From version (e.g., v7.0.0, optional): ")

	// To version
	toVersion = tui.Prompt("To version (e.g., v7.5.0, optional): ")

	// Allowed values
	if promptYesNo("Specify allowed values?", false) {
		for {
			value := tui.Prompt("Enter allowed value (empty to finish): ")
			if value == "" {
				break
			}
			allowedValues = append(allowedValues, value)
		}
	}

	return component, paramType, paramName, severity, description, checkModified, fromVersion, toVersion, allowedValues, nil
}

// newHighRiskParamsListCmd creates the list subcommand
func newHighRiskParamsListCmd(logger *logprinter.Logger) *cobra.Command {
	var (
		component string
		format    string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all high-risk parameters",
		Long: `List all high-risk parameters in the configuration.

You can filter by component and choose output format (table or json).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			binPath, err := locatePrecheckBinaryForHRP(logger)
			if err != nil {
				return err
			}

			configPath := getHighRiskParamsConfigPath()

			// Build command
			cmdArgs := []string{
				"high-risk-params", "list",
				"--config", configPath,
			}

			if component != "" {
				cmdArgs = append(cmdArgs, "--component", component)
			}
			if format != "" {
				cmdArgs = append(cmdArgs, "--format", format)
			}

			// Execute command
			execCmd := exec.CommandContext(cmd.Context(), binPath, cmdArgs...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(output))
			}

			fmt.Print(string(output))
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "Filter by component (tidb, pd, tikv, tiflash)")
	cmd.Flags().StringVar(&format, "format", "table", "Output format (table or json)")

	return cmd
}

// newHighRiskParamsViewCmd creates the view subcommand
func newHighRiskParamsViewCmd(logger *logprinter.Logger) *cobra.Command {
	var (
		component string
		paramType string
		paramName string
	)

	cmd := &cobra.Command{
		Use:   "view",
		Short: "View a high-risk parameter",
		Long:  `View details of a specific high-risk parameter.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			binPath, err := locatePrecheckBinaryForHRP(logger)
			if err != nil {
				return err
			}

			configPath := getHighRiskParamsConfigPath()

			// Use tidb-upgrade-precheck view command to display parameter
			execCmd := exec.CommandContext(cmd.Context(), binPath,
				"high-risk-params", "view",
				"--config", configPath,
				"--component", component,
				"--type", paramType,
				"--name", paramName,
			)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(output))
			}

			fmt.Print(string(output))
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "Component name (tidb, pd, tikv, tiflash)")
	cmd.Flags().StringVar(&paramType, "type", "", "Parameter type (config or system_variable)")
	cmd.Flags().StringVar(&paramName, "name", "", "Parameter name")
	cmd.MarkFlagRequired("component")
	cmd.MarkFlagRequired("type")
	cmd.MarkFlagRequired("name")

	return cmd
}

// newHighRiskParamsRemoveCmd creates the remove subcommand
func newHighRiskParamsRemoveCmd(logger *logprinter.Logger) *cobra.Command {
	var (
		component   string
		paramType   string
		paramName   string
		interactive bool
	)

	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a high-risk parameter",
		Long:  `Remove a high-risk parameter from the configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			binPath, err := locatePrecheckBinaryForHRP(logger)
			if err != nil {
				return err
			}

			configPath := getHighRiskParamsConfigPath()

			if interactive || (component == "" || paramType == "" || paramName == "") {
				// Interactive mode
				component, paramType, paramName, err = promptRemoveParameter(configPath)
				if err != nil {
					return err
				}
			}

			// Build command
			cmdArgs := []string{
				"high-risk-params", "remove",
				"--config", configPath,
				"--component", component,
				"--type", paramType,
				"--name", paramName,
			}

			// Execute command
			execCmd := exec.CommandContext(cmd.Context(), binPath, cmdArgs...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(output))
			}

			fmt.Print(string(output))
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "Component name (tidb, pd, tikv, tiflash)")
	cmd.Flags().StringVar(&paramType, "type", "", "Parameter type (config or system_variable)")
	cmd.Flags().StringVar(&paramName, "name", "", "Parameter name")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Use interactive mode")

	return cmd
}

// promptRemoveParameter prompts user to remove a parameter interactively
func promptRemoveParameter(configPath string) (component, paramType, paramName string, err error) {
	// Load config to show available parameters
	config, err := loadHighRiskParamsConfig(configPath)
	if err != nil {
		return "", "", "", err
	}

	// Collect all parameters
	var params []struct {
		comp    string
		pType   string
		name    string
		display string
	}

	collectParams(config, &params)

	if len(params) == 0 {
		return "", "", "", fmt.Errorf("no high-risk parameters configured")
	}

	// Show parameters
	fmt.Println("Available high-risk parameters:")
	for i, p := range params {
		fmt.Printf("  [%d] %s\n", i+1, p.display)
	}

	// Select parameter
	input := tui.Prompt(fmt.Sprintf("Select parameter to remove (1-%d): ", len(params)))
	idx, err := strconv.Atoi(input)
	if err != nil || idx < 1 || idx > len(params) {
		return "", "", "", fmt.Errorf("invalid selection")
	}

	selected := params[idx-1]
	return selected.comp, selected.pType, selected.name, nil
}

// newHighRiskParamsEditCmd creates the edit subcommand
func newHighRiskParamsEditCmd(logger *logprinter.Logger) *cobra.Command {
	var (
		component     string
		paramType     string
		paramName     string
		severity      string
		description   string
		checkModified bool
		fromVersion   string
		toVersion     string
		allowedValues []string
		interactive   bool
	)

	cmd := &cobra.Command{
		Use:   "edit",
		Short: "Edit a high-risk parameter",
		Long:  `Edit an existing high-risk parameter in the configuration.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			binPath, err := locatePrecheckBinaryForHRP(logger)
			if err != nil {
				return err
			}

			configPath := getHighRiskParamsConfigPath()

			// Load current config
			config, err := loadHighRiskParamsConfig(configPath)
			if err != nil {
				return err
			}

			// Find parameter
			param, found := findParameter(config, component, paramType, paramName)
			if !found {
				return fmt.Errorf("parameter %s/%s/%s not found. Use 'add' command to create a new parameter", component, paramType, paramName)
			}

			if interactive || (severity == "" && description == "" && !checkModified && fromVersion == "" && toVersion == "" && len(allowedValues) == 0) {
				// Interactive mode
				severity, description, checkModified, fromVersion, toVersion, allowedValues, err = promptEditParameter(param)
				if err != nil {
					return err
				}
			}

			// Remove old parameter first
			removeCmd := exec.CommandContext(cmd.Context(), binPath,
				"high-risk-params", "remove",
				"--config", configPath,
				"--component", component,
				"--type", paramType,
				"--name", paramName,
			)
			if err := removeCmd.Run(); err != nil {
				return fmt.Errorf("failed to remove old parameter: %v", err)
			}

			// Add updated parameter
			cmdArgs := []string{
				"high-risk-params", "add",
				"--config", configPath,
				"--component", component,
				"--type", paramType,
				"--name", paramName,
			}

			// Use provided values or keep existing
			if severity == "" {
				severity = param.Severity
			}
			if description == "" {
				description = param.Description
			}
			if !checkModified {
				checkModified = param.CheckModified
			}
			if fromVersion == "" {
				fromVersion = param.FromVersion
			}
			if toVersion == "" {
				toVersion = param.ToVersion
			}
			// Handle allowed values
			var finalAllowedValues []string
			if len(allowedValues) > 0 {
				finalAllowedValues = allowedValues
			} else if len(param.AllowedValues) > 0 {
				// Convert []interface{} to []string
				for _, v := range param.AllowedValues {
					finalAllowedValues = append(finalAllowedValues, fmt.Sprintf("%v", v))
				}
			}

			cmdArgs = append(cmdArgs, "--severity", severity)
			if description != "" {
				cmdArgs = append(cmdArgs, "--description", description)
			}
			if checkModified {
				cmdArgs = append(cmdArgs, "--check-modified")
			}
			if fromVersion != "" {
				cmdArgs = append(cmdArgs, "--from-version", fromVersion)
			}
			if toVersion != "" {
				cmdArgs = append(cmdArgs, "--to-version", toVersion)
			}
			if len(finalAllowedValues) > 0 {
				cmdArgs = append(cmdArgs, "--allowed-values", strings.Join(finalAllowedValues, ","))
			}

			// Execute command
			execCmd := exec.CommandContext(cmd.Context(), binPath, cmdArgs...)
			output, err := execCmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("tidb-upgrade-precheck failed: %v\nOutput: %s", err, string(output))
			}

			fmt.Print(string(output))
			logger.Infof("Configuration updated: %s", configPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&component, "component", "", "Component name (tidb, pd, tikv, tiflash)")
	cmd.Flags().StringVar(&paramType, "type", "", "Parameter type (config or system_variable)")
	cmd.Flags().StringVar(&paramName, "name", "", "Parameter name")
	cmd.Flags().StringVar(&severity, "severity", "", "Severity level (error, warning, info)")
	cmd.Flags().StringVar(&description, "description", "", "Description of why this parameter is high-risk")
	cmd.Flags().BoolVar(&checkModified, "check-modified", false, "Only check if parameter is modified from default")
	cmd.Flags().StringVar(&fromVersion, "from-version", "", "From version (e.g., v7.0.0)")
	cmd.Flags().StringVar(&toVersion, "to-version", "", "To version (e.g., v7.5.0)")
	cmd.Flags().StringSliceVar(&allowedValues, "allowed-values", []string{}, "Allowed values (comma-separated)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Use interactive mode")
	cmd.MarkFlagRequired("component")
	cmd.MarkFlagRequired("type")
	cmd.MarkFlagRequired("name")

	return cmd
}

// promptEditParameter prompts user to edit a parameter interactively
func promptEditParameter(currentParam highRiskParamConfig) (severity, description string, checkModified bool, fromVersion, toVersion string, allowedValues []string, err error) {
	// Show current values
	fmt.Println("Current configuration:")
	fmt.Printf("  Severity: %s\n", currentParam.Severity)
	fmt.Printf("  Description: %s\n", currentParam.Description)
	fmt.Printf("  Check Modified: %v\n", currentParam.CheckModified)
	fmt.Printf("  From Version: %s\n", currentParam.FromVersion)
	fmt.Printf("  To Version: %s\n", currentParam.ToVersion)
	fmt.Printf("  Allowed Values: %v\n", currentParam.AllowedValues)
	fmt.Println()

	// Severity
	severityInput := tui.Prompt(fmt.Sprintf("Enter severity [%s] (press Enter to keep): ", currentParam.Severity))
	if severityInput == "" {
		severity = currentParam.Severity
	} else {
		severity = promptSelect("Select severity:", []string{"error", "warning", "info"}, severityInput)
	}

	// Description
	descriptionInput := tui.Prompt(fmt.Sprintf("Enter description [%s] (press Enter to keep): ", currentParam.Description))
	if descriptionInput == "" {
		description = currentParam.Description
	} else {
		description = descriptionInput
	}

	// Check modified
	checkModifiedInput := tui.Prompt(fmt.Sprintf("Check modified [%v] (y/n, press Enter to keep): ", currentParam.CheckModified))
	if checkModifiedInput == "" {
		checkModified = currentParam.CheckModified
	} else {
		checkModified = strings.ToLower(checkModifiedInput) == "y" || strings.ToLower(checkModifiedInput) == "yes"
	}

	// From version
	fromVersionInput := tui.Prompt(fmt.Sprintf("From version [%s] (press Enter to keep): ", currentParam.FromVersion))
	if fromVersionInput == "" {
		fromVersion = currentParam.FromVersion
	} else {
		fromVersion = fromVersionInput
	}

	// To version
	toVersionInput := tui.Prompt(fmt.Sprintf("To version [%s] (press Enter to keep): ", currentParam.ToVersion))
	if toVersionInput == "" {
		toVersion = currentParam.ToVersion
	} else {
		toVersion = toVersionInput
	}

	// Allowed values
	if promptYesNo("Update allowed values?", false) {
		for {
			value := tui.Prompt("Enter allowed value (empty to finish): ")
			if value == "" {
				break
			}
			allowedValues = append(allowedValues, value)
		}
	} else {
		// Convert []interface{} to []string
		for _, v := range currentParam.AllowedValues {
			allowedValues = append(allowedValues, fmt.Sprintf("%v", v))
		}
	}

	return severity, description, checkModified, fromVersion, toVersion, allowedValues, nil
}

// Helper types and functions

type highRiskParamConfig struct {
	Severity      string        `json:"severity,omitempty"`
	Description   string        `json:"description,omitempty"`
	CheckModified bool          `json:"check_modified,omitempty"`
	FromVersion   string        `json:"from_version,omitempty"`
	ToVersion     string        `json:"to_version,omitempty"`
	AllowedValues []interface{} `json:"allowed_values,omitempty"`
}

type highRiskParamsConfig struct {
	TiDB struct {
		Config          map[string]highRiskParamConfig `json:"config,omitempty"`
		SystemVariables map[string]highRiskParamConfig `json:"system_variables,omitempty"`
	} `json:"tidb,omitempty"`
	PD struct {
		Config map[string]highRiskParamConfig `json:"config,omitempty"`
	} `json:"pd,omitempty"`
	TiKV struct {
		Config map[string]highRiskParamConfig `json:"config,omitempty"`
	} `json:"tikv,omitempty"`
	TiFlash struct {
		Config map[string]highRiskParamConfig `json:"config,omitempty"`
	} `json:"tiflash,omitempty"`
}

// loadHighRiskParamsConfig loads the high-risk parameters configuration
func loadHighRiskParamsConfig(configPath string) (*highRiskParamsConfig, error) {
	config := &highRiskParamsConfig{}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return config, nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	if len(data) == 0 {
		return config, nil
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return config, nil
}

// findParameter finds a parameter in the config
func findParameter(config *highRiskParamsConfig, component, paramType, paramName string) (highRiskParamConfig, bool) {
	component = strings.ToLower(component)
	paramType = strings.ToLower(paramType)

	switch component {
	case "tidb":
		if paramType == "config" {
			if param, ok := config.TiDB.Config[paramName]; ok {
				return param, true
			}
		} else if paramType == "system_variable" || paramType == "system-variable" || paramType == "sysvar" {
			if param, ok := config.TiDB.SystemVariables[paramName]; ok {
				return param, true
			}
		}
	case "pd":
		if paramType == "config" {
			if param, ok := config.PD.Config[paramName]; ok {
				return param, true
			}
		}
	case "tikv":
		if paramType == "config" {
			if param, ok := config.TiKV.Config[paramName]; ok {
				return param, true
			}
		}
	case "tiflash":
		if paramType == "config" {
			if param, ok := config.TiFlash.Config[paramName]; ok {
				return param, true
			}
		}
	}

	return highRiskParamConfig{}, false
}

// collectParams collects all parameters from config
func collectParams(config *highRiskParamsConfig, params *[]struct {
	comp    string
	pType   string
	name    string
	display string
}) {
	// TiDB config
	for name := range config.TiDB.Config {
		*params = append(*params, struct {
			comp    string
			pType   string
			name    string
			display string
		}{"tidb", "config", name, fmt.Sprintf("tidb/config/%s", name)})
	}
	// TiDB system variables
	for name := range config.TiDB.SystemVariables {
		*params = append(*params, struct {
			comp    string
			pType   string
			name    string
			display string
		}{"tidb", "system_variable", name, fmt.Sprintf("tidb/system_variable/%s", name)})
	}
	// PD
	for name := range config.PD.Config {
		*params = append(*params, struct {
			comp    string
			pType   string
			name    string
			display string
		}{"pd", "config", name, fmt.Sprintf("pd/config/%s", name)})
	}
	// TiKV
	for name := range config.TiKV.Config {
		*params = append(*params, struct {
			comp    string
			pType   string
			name    string
			display string
		}{"tikv", "config", name, fmt.Sprintf("tikv/config/%s", name)})
	}
	// TiFlash
	for name := range config.TiFlash.Config {
		*params = append(*params, struct {
			comp    string
			pType   string
			name    string
			display string
		}{"tiflash", "config", name, fmt.Sprintf("tiflash/config/%s", name)})
	}
}

// promptSelect prompts user to select from options
func promptSelect(prompt string, options []string, defaultValue string) string {
	fmt.Println(prompt)
	for i, opt := range options {
		marker := " "
		if opt == defaultValue {
			marker = "*"
		}
		fmt.Printf("  %s [%d] %s\n", marker, i+1, opt)
	}

	input := tui.Prompt(fmt.Sprintf("Select option [%s]: ", defaultValue))
	if idx, err := strconv.Atoi(input); err == nil && idx > 0 && idx <= len(options) {
		return options[idx-1]
	}

	// Try to match by name
	for _, opt := range options {
		if strings.EqualFold(opt, input) {
			return opt
		}
	}

	return defaultValue
}

// promptYesNo prompts user for yes/no input
func promptYesNo(prompt string, defaultValue bool) bool {
	defaultStr := "n"
	if defaultValue {
		defaultStr = "y"
	}

	input := tui.Prompt(fmt.Sprintf("%s (y/n) [%s]: ", prompt, defaultStr))
	if input == "" {
		return defaultValue
	}
	return strings.ToLower(input) == "y" || strings.ToLower(input) == "yes"
}
