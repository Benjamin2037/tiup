# Precheck Developer Notes

## Testing

The TiUP repository contains packages that rely on Go failpoints. Before running the full test suite (for example `go test ./...` or the repo-wide coverage targets), enable failpoints and remember to disable them afterwards:

```bash
make failpoint-enable
TIUP_HOME=$(pwd)/tests/tiup go test ./...
make failpoint-disable
```

Running the precheck-only packages (for example `go test ./components/cluster/precheck/...`) does not require failpoints, but repository-wide test runs will fail without the enable/disable steps above.
