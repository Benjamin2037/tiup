package command

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	"github.com/pingcap/tiup/components/cluster/precheck"
	"github.com/pingcap/tiup/pkg/cluster/spec"
	"github.com/pingcap/tiup/pkg/utils"
)

const (
	snapshotQueryTimeout = 5 * time.Second
)

type tidbSQLCredentials struct {
	user     string
	password string
}

// collectClusterSnapshot aggregates the live-cluster information needed by the
// parameter precheck engine. Errors from individual collectors are surfaced so
// callers can warn but continue the upgrade flow.
func collectClusterSnapshot(ctx context.Context, clusterName string, metadata *spec.ClusterMeta, creds tidbSQLCredentials) (*precheck.Snapshot, error) {
	if metadata == nil || metadata.Topology == nil {
		return nil, nil
	}

	topology := metadata.Topology

	collectors := make([]precheck.SnapshotCollector, 0, 2)

	if fetcher, err := newTiDBSnapshotFetcher(clusterName, topology, creds); err != nil {
		return nil, err
	} else if fetcher != nil {
		collectors = append(collectors, precheck.NewGlobalVariablesCollector(fetcher))
		collectors = append(collectors, precheck.NewClusterConfigCollector(fetcher))
	}

	if cfg := buildTopologyConfigSnapshot(topology); len(cfg) > 0 {
		captured := cfg
		collectors = append(collectors, precheck.SnapshotCollectorFunc(func(_ context.Context, snapshot *precheck.Snapshot) error {
			if snapshot.Config == nil {
				snapshot.Config = make(map[string]any, len(captured))
			}
			for k, v := range captured {
				snapshot.Config[k] = v
			}
			return nil
		}))
	}

	if len(collectors) == 0 {
		return nil, nil
	}

	return precheck.CollectClusterSnapshot(ctx, collectors...)
}

func buildTopologyConfigSnapshot(topology *spec.Specification) map[string]any {
	if topology == nil {
		return nil
	}

	global := make(map[string]any)
	addGlobal := func(key string, value map[string]any) {
		if len(value) > 0 {
			global[key] = value
		}
	}

	addGlobal("tidb", topology.ServerConfigs.TiDB)
	addGlobal("tikv", topology.ServerConfigs.TiKV)
	addGlobal("pd", topology.ServerConfigs.PD)
	addGlobal("tso", topology.ServerConfigs.TSO)
	addGlobal("scheduling", topology.ServerConfigs.Scheduling)
	addGlobal("dashboard", topology.ServerConfigs.Dashboard)
	addGlobal("tiflash", topology.ServerConfigs.TiFlash)
	addGlobal("tiproxy", topology.ServerConfigs.TiProxy)
	addGlobal("tiflash-learner", topology.ServerConfigs.TiFlashLearner)
	addGlobal("pump", topology.ServerConfigs.Pump)
	addGlobal("drainer", topology.ServerConfigs.Drainer)
	addGlobal("cdc", topology.ServerConfigs.CDC)
	addGlobal("kvcdc", topology.ServerConfigs.TiKVCDC)
	addGlobal("grafana", anyMapFromStringMap(topology.ServerConfigs.Grafana))

	instances := make(map[string][]map[string]any)
	appendInstanceConfigs(instances, "tidb", topology.TiDBServers, func(spec *spec.TiDBSpec) map[string]any {
		if len(spec.Config) == 0 {
			return nil
		}
		return map[string]any{
			"address": utils.JoinHostPort(spec.Host, spec.Port),
			"config":  spec.Config,
		}
	})
	appendInstanceConfigs(instances, "tikv", topology.TiKVServers, func(spec *spec.TiKVSpec) map[string]any {
		if len(spec.Config) == 0 {
			return nil
		}
		return map[string]any{
			"address": utils.JoinHostPort(spec.Host, spec.Port),
			"config":  spec.Config,
		}
	})
	appendInstanceConfigs(instances, "pd", topology.PDServers, func(spec *spec.PDSpec) map[string]any {
		if len(spec.Config) == 0 {
			return nil
		}
		return map[string]any{
			"address": utils.JoinHostPort(spec.Host, spec.GetMainPort()),
			"config":  spec.Config,
		}
	})
	appendInstanceConfigs(instances, "tiflash", topology.TiFlashServers, func(spec *spec.TiFlashSpec) map[string]any {
		if len(spec.Config) == 0 {
			return nil
		}
		return map[string]any{
			"address": utils.JoinHostPort(spec.Host, spec.TCPPort),
			"config":  spec.Config,
		}
	})
	appendInstanceConfigs(instances, "tiproxy", topology.TiProxyServers, func(spec *spec.TiProxySpec) map[string]any {
		if len(spec.Config) == 0 {
			return nil
		}
		return map[string]any{
			"address": utils.JoinHostPort(spec.Host, spec.Port),
			"config":  spec.Config,
		}
	})

	if len(global) == 0 && len(instances) == 0 {
		return nil
	}

	payload := make(map[string]any, 2)
	scoped := make(map[string]any)
	if len(global) > 0 {
		scoped["global"] = global
	}
	if len(instances) > 0 {
		scoped["instances"] = instances
	}
	if len(scoped) > 0 {
		payload["topology_spec"] = scoped
	}

	return payload
}

func appendInstanceConfigs[T any](target map[string][]map[string]any, key string, specs []*T, convert func(*T) map[string]any) {
	if len(specs) == 0 {
		return
	}
	buf := make([]map[string]any, 0, len(specs))
	for _, spec := range specs {
		entry := convert(spec)
		if entry != nil {
			buf = append(buf, entry)
		}
	}
	if len(buf) > 0 {
		target[key] = buf
	}
}

func anyMapFromStringMap(src map[string]string) map[string]any {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]any, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

type tidbSnapshotFetcher struct {
	endpoints []string
	tlsName   string
	user      string
	password  string
}

func newTiDBSnapshotFetcher(clusterName string, topology *spec.Specification, creds tidbSQLCredentials) (*tidbSnapshotFetcher, error) {
	if topology == nil || len(topology.TiDBServers) == 0 {
		return nil, nil
	}

	endpoints := make([]string, 0, len(topology.TiDBServers))
	for _, inst := range topology.TiDBServers {
		endpoints = append(endpoints, utils.JoinHostPort(inst.Host, inst.Port))
	}

	username := strings.TrimSpace(creds.user)
	if username == "" {
		username = "root"
	}

	fetcher := &tidbSnapshotFetcher{
		endpoints: endpoints,
		user:      username,
		password:  creds.password,
	}

	if topology.GlobalOptions.TLSEnabled {
		tlsName, err := registerClusterTLSConfig(clusterName, topology)
		if err != nil {
			return nil, err
		}
		fetcher.tlsName = tlsName
	}

	return fetcher, nil
}

func registerClusterTLSConfig(clusterName string, topology *spec.Specification) (string, error) {
	if tidbSpec == nil {
		// Tests may invoke the collector helpers without an initialised spec manager.
		return "", nil
	}

	tlsDir := tidbSpec.Path(clusterName, spec.TLSCertKeyDir)
	tlsCfg, err := topology.TLSConfig(tlsDir)
	if err != nil {
		return "", err
	}
	if tlsCfg == nil {
		return "", nil
	}
	tlsCfg = cloneTLSConfig(tlsCfg)

	name := fmt.Sprintf("tiup-precheck-%s-%d", clusterName, time.Now().UnixNano())
	if err := mysql.RegisterTLSConfig(name, tlsCfg); err != nil {
		return "", err
	}
	return name, nil
}

func cloneTLSConfig(cfg *tls.Config) *tls.Config {
	if cfg == nil {
		return nil
	}
	clone := cfg.Clone()
	if clone.ServerName == "" {
		clone.InsecureSkipVerify = true
	}
	return clone
}

func (f *tidbSnapshotFetcher) FetchGlobalVariables(ctx context.Context) (map[string]string, error) {
	if len(f.endpoints) == 0 {
		return nil, errors.New("no TiDB endpoints available")
	}

	var lastErr error
	for _, endpoint := range f.endpoints {
		vars, err := f.fetchVariablesFromEndpoint(ctx, endpoint)
		if err == nil {
			return vars, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("unable to query TiDB global variables")
	}
	return nil, lastErr
}

func (f *tidbSnapshotFetcher) FetchClusterConfig(ctx context.Context) (map[string]any, error) {
	if len(f.endpoints) == 0 {
		return nil, errors.New("no TiDB endpoints available")
	}

	var lastErr error
	for _, endpoint := range f.endpoints {
		cfg, err := f.fetchClusterConfigFromEndpoint(ctx, endpoint)
		if err == nil {
			return cfg, nil
		}
		if myErr, ok := err.(*mysql.MySQLError); ok && myErr.Number == 1146 {
			// INFORMATION_SCHEMA.CLUSTER_CONFIG not available on very old versions.
			return nil, nil
		}
		lastErr = err
	}
	if lastErr == nil {
		lastErr = errors.New("unable to query cluster configuration")
	}
	return nil, lastErr
}

func (f *tidbSnapshotFetcher) fetchVariablesFromEndpoint(ctx context.Context, endpoint string) (map[string]string, error) {
	db, err := f.openDB(endpoint)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := pingWithTimeout(ctx, db, snapshotQueryTimeout); err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, snapshotQueryTimeout)
	defer cancel()
	rows, err := db.QueryContext(queryCtx, "SHOW GLOBAL VARIABLES")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]string)
	for rows.Next() {
		var name, value string
		if err := rows.Scan(&name, &value); err != nil {
			return nil, err
		}
		key := strings.TrimSpace(strings.ToLower(name))
		result[key] = strings.TrimSpace(value)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (f *tidbSnapshotFetcher) fetchClusterConfigFromEndpoint(ctx context.Context, endpoint string) (map[string]any, error) {
	db, err := f.openDB(endpoint)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	if err := pingWithTimeout(ctx, db, snapshotQueryTimeout); err != nil {
		return nil, err
	}

	queryCtx, cancel := context.WithTimeout(ctx, snapshotQueryTimeout)
	defer cancel()

	const clusterConfigQueryWithDefault = "SELECT type, instance, `key`, value, default_value FROM information_schema.cluster_config"
	const clusterConfigQueryLegacy = "SELECT type, instance, `key`, value FROM information_schema.cluster_config"

	rows, err := db.QueryContext(queryCtx, clusterConfigQueryWithDefault)
	legacyQuery := false
	if err != nil {
		if myErr, ok := err.(*mysql.MySQLError); ok && myErr.Number == 1054 {
			legacyQuery = true
			rows, err = db.QueryContext(queryCtx, clusterConfigQueryLegacy)
		}
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type configPair map[string]string
	runtime := make(map[string]map[string]map[string]configPair)

	for rows.Next() {
		var component, instance, key, value, defaultValue string
		if legacyQuery {
			if err := rows.Scan(&component, &instance, &key, &value); err != nil {
				return nil, err
			}
			defaultValue = ""
		} else if err := rows.Scan(&component, &instance, &key, &value, &defaultValue); err != nil {
			return nil, err
		}

		compKey := strings.TrimSpace(strings.ToLower(component))
		instKey := strings.TrimSpace(strings.ToLower(instance))
		cfgKey := strings.TrimSpace(key)

		if cfgKey == "" {
			continue
		}

		compMap, ok := runtime[compKey]
		if !ok {
			compMap = make(map[string]map[string]configPair)
			runtime[compKey] = compMap
		}
		instMap, ok := compMap[instKey]
		if !ok {
			instMap = make(map[string]configPair)
			compMap[instKey] = instMap
		}
		instMap[cfgKey] = configPair{
			"value":   strings.TrimSpace(value),
			"default": strings.TrimSpace(defaultValue),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(runtime) == 0 {
		return nil, nil
	}

	componentPayload := make(map[string]any, len(runtime))
	for comp, instances := range runtime {
		instancePayload := make(map[string]any, len(instances))
		for inst, cfgs := range instances {
			cfgPayload := make(map[string]any, len(cfgs))
			for name, pair := range cfgs {
				cfgPayload[name] = map[string]string(pair)
			}
			instancePayload[inst] = cfgPayload
		}
		componentPayload[comp] = instancePayload
	}

	result := map[string]any{
		"cluster_runtime": componentPayload,
	}
	return result, nil
}

func (f *tidbSnapshotFetcher) openDB(endpoint string) (*sql.DB, error) {
	cfg := mysql.NewConfig()
	cfg.User = f.user
	cfg.Passwd = f.password
	cfg.Net = "tcp"
	cfg.Addr = endpoint
	cfg.Params = map[string]string{
		"charset": "utf8mb4,utf8",
	}
	cfg.Timeout = snapshotQueryTimeout
	cfg.ReadTimeout = snapshotQueryTimeout
	cfg.WriteTimeout = snapshotQueryTimeout
	cfg.ParseTime = true
	if f.tlsName != "" {
		cfg.TLSConfig = f.tlsName
	}

	dsn := cfg.FormatDSN()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxIdleConns(0)
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Minute)
	return db, nil
}

func pingWithTimeout(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	pingCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return db.PingContext(pingCtx)
}
