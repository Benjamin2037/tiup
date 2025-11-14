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
	"fmt"
	"os"
	"time"

	"github.com/pingcap/tiup/pkg/cluster/spec"
	logprinter "github.com/pingcap/tiup/pkg/logger/printer"
	"github.com/pingcap/tiup/pkg/utils"
	"github.com/spf13/cobra"

	"github.com/pingcap/tiup/components/cluster/precheck"
)

func newUpgradeCmd() *cobra.Command {
	offlineMode := false
	ignoreVersionCheck := false
	var tidbVer, tikvVer, pdVer, tsoVer, schedulingVer, tiflashVer, kvcdcVer, dashboardVer, cdcVer, alertmanagerVer, nodeExporterVer, blackboxExporterVer, tiproxyVer string
	var restartTimeout time.Duration
	var precheckOnlyFlag bool
	var withoutPrecheckFlag bool
	var precheckOutputFormat string
	var precheckOutputFile string
	logger := logprinter.NewLogger("")

	// clusterUpgradeFunc enables injection in tests to observe whether an upgrade would execute
	// clusterUpgradeFunc default binding
	if clusterUpgradeFunc == nil {
		clusterUpgradeFunc = func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error {
			return cm.Upgrade(clusterName, version, compVers, gOpt, skip, offline, ignoreVersion, restartTimeout)
		}
	}

	cmd := &cobra.Command{
		Use:   "upgrade [precheck] <cluster-name> <version>",
		Short: "Upgrade a specified TiDB cluster",
		RunE: func(cmd *cobra.Command, args []string) error {
			precheckFmt, err := precheck.ParseOutputFormat(precheckOutputFormat)
			if err != nil {
				return err
			}

			if len(args) > 0 && args[0] == "precheck" {
				if len(args) != 3 {
					return cmd.Help()
				}
				clusterName := args[1]
				targetRaw := args[2]
				version, err := utils.FmtVer(targetRaw)
				if err != nil {
					return err
				}
				if _, err := runParameterPrecheck(context.Background(), clusterName, version, logger, precheckFmt, precheckOutputFile); err != nil {
					return err
				}
				logger.Infof("Precheck complete. This was a dry run; no upgrade performed.")
				return nil
			}

			if len(args) != 2 {
				return cmd.Help()
			}

			clusterName := args[0]
			targetRaw := args[1]
			version, err := utils.FmtVer(targetRaw)
			if err != nil {
				return err
			}

			// Build component version overrides (may be empty strings -> follow cluster version)
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

			// Mode 3: skip precheck entirely
			if withoutPrecheckFlag {
				logger.Warnf("Skipping parameter precheck (--without-precheck). Proceeding at your own risk.")
				return clusterUpgradeFunc(clusterName, version, componentVersions, skipConfirm, offlineMode, ignoreVersionCheck, restartTimeout)
			}

			_, preErr := runParameterPrecheck(context.Background(), clusterName, version, logger, precheckFmt, precheckOutputFile)
			if preErr != nil {
				logger.Errorf("parameter precheck failed: %v", preErr)
				if precheckOnlyFlag { // in planning mode fail fast
					return preErr
				}
				// allow user to continue despite failure only in execute mode prompt
			}

			if precheckOnlyFlag { // Mode 1: planning – exit after report
				logger.Infof("Precheck complete. No upgrade performed (planning mode).")
				return nil
			}

			// Mode 2: execute – ask for confirmation unless skipConfirm already set
			if !skipConfirm {
				proceed, askErr := precheck.AskForUserConfirmation()
				if askErr != nil {
					return askErr
				}
				if !proceed {
					logger.Infof("Aborting upgrade per user response.")
					return nil
				}
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
	cmd.Flags().BoolVar(&precheckOnlyFlag, "precheck", false, "Run parameter precheck and exit without upgrading")
	cmd.Flags().BoolVar(&withoutPrecheckFlag, "without-precheck", false, "Skip parameter precheck (dangerous)")
	cmd.Flags().StringVar(&precheckOutputFormat, "precheck-output", "text", "Format for the precheck report (text, markdown, html)")
	cmd.Flags().StringVar(&precheckOutputFile, "precheck-output-file", "", "Write the precheck report to a file instead of stdout")
	return cmd
}

func runParameterPrecheck(ctx context.Context, clusterName, targetVersion string, logger *logprinter.Logger, format precheck.OutputFormat, outputPath string) (*precheck.RiskReport, error) {
	logger.Infof("Running parameter precheck...")
	sourceVersion := "(unknown)"
	metadata, err := clusterMetadataFunc(clusterName)
	if err != nil {
		logger.Warnf("unable to read current cluster metadata: %v", err)
	} else {
		sourceVersion = metadata.Version
	}

	report, runErr := precheck.RunPrecheckForUpgrade(ctx, sourceVersion, targetVersion)
	if report != nil {
		if err := outputPrecheckReport(report, format, outputPath, logger); err != nil && runErr == nil {
			runErr = err
		}
	}
	return report, runErr
}

func outputPrecheckReport(report *precheck.RiskReport, format precheck.OutputFormat, outputPath string, logger *logprinter.Logger) error {
	if format == precheck.OutputText && outputPath == "" {
		precheck.PrintReportToConsole(report)
		return nil
	}

	payload, err := precheck.RenderReport(report, format)
	if err != nil {
		return err
	}

	if outputPath == "" {
		if _, err := fmt.Fprintln(os.Stdout, string(payload)); err != nil {
			return err
		}
		return nil
	}

	if err := os.WriteFile(outputPath, payload, 0o644); err != nil {
		return err
	}
	logger.Infof("Precheck report saved to %s", outputPath)
	return nil
}

// clusterUpgradeFunc is a package-level variable so tests can replace it.
var (
	clusterUpgradeFunc  func(clusterName, version string, compVers map[string]string, skip bool, offline bool, ignoreVersion bool, restartTimeout time.Duration) error
	clusterMetadataFunc = spec.ClusterMetadata
)
