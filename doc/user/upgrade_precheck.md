# TiUP Cluster Upgrade Precheck Integration

## Overview

TiUP `cluster upgrade` command integrates `tidb-upgrade-precheck` tool to perform compatibility checks before upgrading TiDB clusters. The integration uses command-line interface, keeping TiUP and `tidb-upgrade-precheck` completely decoupled.

## Quick Start

### Default Behavior (Recommended)

```bash
tiup cluster upgrade <cluster-name> <version>
```

Example:
```bash
tiup cluster upgrade my-cluster v8.0.0
```

This will:
1. Automatically run precheck
2. Display the report
3. Ask for user confirmation
4. Proceed with upgrade if confirmed

### Run Precheck Only

```bash
tiup cluster upgrade <cluster-name> <version> --precheck
```

This runs precheck and exits without performing the upgrade.

### Skip Precheck (Not Recommended)

```bash
tiup cluster upgrade <cluster-name> <version> --without-precheck
```

This skips precheck and proceeds directly to upgrade.

## Command Flags

| Flag | Description | Default |
|------|-------------|---------|
| `--precheck` | Run precheck only, do not perform upgrade | false |
| `--without-precheck` | Skip precheck and proceed directly to upgrade | false |
| `--precheck-output` | Report format (text, markdown, html, json) | text |
| `--precheck-output-file` | Write report to a file instead of stdout | "" |
| `--precheck-tidb-user` | TiDB SQL username | root |
| `--precheck-tidb-password` | TiDB SQL password | "" |
| `--precheck-tidb-password-file` | Path to file containing TiDB SQL password | "" |
| `--precheck-tidb-password-prompt` | Prompt for TiDB SQL password interactively | false |
| `--precheck-high-risk-params-config` | Path to high-risk parameters config file (JSON) | "" |

## Usage Examples

### Example 1: Generate HTML Report

```bash
tiup cluster upgrade my-cluster v8.0.0 \
  --precheck-output html \
  --precheck-output-file ./precheck-report.html
```

### Example 2: Use Custom TiDB Credentials

```bash
tiup cluster upgrade my-cluster v8.0.0 \
  --precheck-tidb-user admin \
  --precheck-tidb-password-prompt
```

### Example 3: Use High-Risk Parameters Config

```bash
tiup cluster upgrade my-cluster v8.0.0 \
  --precheck-high-risk-params-config ~/.tiup/high_risk_params.json
```

## Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `TIDB_UPGRADE_PRECHECK_BIN` | Path to tidb-upgrade-precheck binary | Auto-detect |
| `TIDB_UPGRADE_PRECHECK_KB` | Path to knowledge base directory | Auto-detect |

## Knowledge Base Location

The knowledge base is automatically located in the following order:

1. Environment variable `TIDB_UPGRADE_PRECHECK_KB`
2. TiUP profile directory: `~/.tiup/storage/cluster/knowledge/`
3. Same directory as tiup-cluster binary: `{binary-dir}/knowledge/`

## Report Files

Reports are stored in: `~/.tiup/storage/cluster/upgrade_precheck/reports/`

Report filename format: `upgrade_precheck_report_{timestamp}.{ext}`

Supported formats:
- `text` → `.txt`
- `markdown` → `.md`
- `html` → `.html`
- `json` → `.json`

## Notes

1. **Binary Location**: Ensure `tidb-upgrade-precheck` is bundled with `tiup-cluster` or available in PATH
2. **Knowledge Base**: Ensure knowledge base is generated and located correctly
3. **Credentials**: Passwords are passed via command-line arguments. Be cautious when logging
4. **Report Files**: Report files are not automatically cleaned up and need manual management

## Related Documentation

- [TiUP Cluster Upgrade Command](./cluster.md#upgrade)
- [tidb-upgrade-precheck Design](../tiup_precheck_design.md)

