package precheck

import (
	"context"
	"strings"
)

// GlobalVariable is a global system variable and whether it is user-set.
type GlobalVariable struct {
	Value   string
	UserSet bool
}

// Snapshot captures the subset of live cluster state used by the precheck engine.
// Fields will expand over time as new collectors are introduced.
type Snapshot struct {
	GlobalVariables map[string]GlobalVariable
	Config          map[string]any
}

// Clone produces a deep copy of the snapshot so callers can reuse it safely.
func (s *Snapshot) Clone() *Snapshot {
	if s == nil {
		return nil
	}
	out := &Snapshot{}
	if len(s.GlobalVariables) > 0 {
		out.GlobalVariables = cloneGlobalVariableMap(s.GlobalVariables)
	}
	if len(s.Config) > 0 {
		out.Config = cloneAnyMap(s.Config)
	}
	return out
}

// SnapshotCollector extracts a slice of the live cluster state and populates the
// provided snapshot.
type SnapshotCollector interface {
	Collect(ctx context.Context, snapshot *Snapshot) error
}

// SnapshotCollectorFunc adapts ordinary functions into SnapshotCollector values.
type SnapshotCollectorFunc func(ctx context.Context, snapshot *Snapshot) error

// Collect implements SnapshotCollector.
func (f SnapshotCollectorFunc) Collect(ctx context.Context, snapshot *Snapshot) error {
	return f(ctx, snapshot)
}

// CollectClusterSnapshot executes the provided collectors in order and returns
// the aggregated snapshot. Collection stops at the first error and returns the
// partially populated snapshot alongside the error.
func CollectClusterSnapshot(ctx context.Context, collectors ...SnapshotCollector) (*Snapshot, error) {
	snapshot := &Snapshot{}
	for _, collector := range collectors {
		if collector == nil {
			continue
		}
		if err := collector.Collect(ctx, snapshot); err != nil {
			return snapshot, err
		}
	}
	return snapshot, nil
}

// GlobalVariableFetcher loads the current global system variables from a cluster.
type GlobalVariableFetcher interface {
	FetchGlobalVariables(ctx context.Context) (map[string]string, error)
}

// GlobalVariableFetcherFunc adapts a function into a GlobalVariableFetcher.
type GlobalVariableFetcherFunc func(ctx context.Context) (map[string]string, error)

// FetchGlobalVariables implements GlobalVariableFetcher.
func (f GlobalVariableFetcherFunc) FetchGlobalVariables(ctx context.Context) (map[string]string, error) {
	return f(ctx)
}

// NewGlobalVariablesCollectorWithDefaults returns a SnapshotCollector that marks UserSet as true if the value differs from the old default.
func NewGlobalVariablesCollectorWithDefaults(fetcher GlobalVariableFetcher, oldDefaults map[string]string) SnapshotCollector {
	if fetcher == nil {
		return SnapshotCollectorFunc(func(context.Context, *Snapshot) error { return nil })
	}
	return SnapshotCollectorFunc(func(ctx context.Context, snapshot *Snapshot) error {
		vars, err := fetcher.FetchGlobalVariables(ctx)
		if err != nil {
			return err
		}
		if len(vars) == 0 {
			return nil
		}
		if snapshot.GlobalVariables == nil {
			snapshot.GlobalVariables = make(map[string]GlobalVariable, len(vars))
		}
		for k, v := range vars {
			key := strings.TrimSpace(strings.ToLower(k))
			val := strings.TrimSpace(v)
			def := ""
			if oldDefaults != nil {
				def = strings.TrimSpace(oldDefaults[key])
			}
			userSet := def != "" && val != def
			snapshot.GlobalVariables[key] = GlobalVariable{
				Value:   val,
				UserSet: userSet,
			}
		}
		return nil
	})
}

// ClusterConfigFetcher loads the component configuration values from a cluster.
type ClusterConfigFetcher interface {
	FetchClusterConfig(ctx context.Context) (map[string]any, error)
}

// ClusterConfigFetcherFunc adapts a function into a ClusterConfigFetcher.
type ClusterConfigFetcherFunc func(ctx context.Context) (map[string]any, error)

// FetchClusterConfig implements ClusterConfigFetcher.
func (f ClusterConfigFetcherFunc) FetchClusterConfig(ctx context.Context) (map[string]any, error) {
	return f(ctx)
}

// NewClusterConfigCollector returns a collector that captures component configuration
// data using the provided fetcher.
func NewClusterConfigCollector(fetcher ClusterConfigFetcher) SnapshotCollector {
	if fetcher == nil {
		return SnapshotCollectorFunc(func(context.Context, *Snapshot) error { return nil })
	}
	return SnapshotCollectorFunc(func(ctx context.Context, snapshot *Snapshot) error {
		cfg, err := fetcher.FetchClusterConfig(ctx)
		if err != nil {
			return err
		}
		if len(cfg) == 0 {
			return nil
		}
		snapshot.Config = mergeConfig(snapshot.Config, cfg)
		return nil
	})
}

func cloneGlobalVariableMap(src map[string]GlobalVariable) map[string]GlobalVariable {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]GlobalVariable, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneAnyMap(src map[string]any) map[string]any {
	if len(src) == 0 {
		return nil
	}
	out := make(map[string]any, len(src))
	for k, v := range src {
		out[k] = cloneValue(v)
	}
	return out
}

func mergeConfig(dst map[string]any, src map[string]any) map[string]any {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]any, len(src))
	}
	for k, v := range src {
		dst[k] = cloneValue(v)
	}
	return dst
}

func cloneValue(v any) any {
	switch val := v.(type) {
	case map[string]any:
		return cloneAnyMap(val)
	case []any:
		out := make([]any, len(val))
		for i := range val {
			out[i] = cloneValue(val[i])
		}
		return out
	default:
		return val
	}
}
