package config

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nikhilsk98/SkyGrid/internal/txnid"
)

// repoConfig is the real config file shipped with the repository.
const repoConfig = "../../config/skygrid.yaml"

func noEnv(string) (string, bool) { return "", false }

func mapEnv(m map[string]string) LookupEnv {
	return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
}

func readRepoConfig(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(repoConfig)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestLoadRepoConfig(t *testing.T) {
	cfg, err := Parse(readRepoConfig(t), noEnv)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	want := Config{
		Grid:     Grid{Width: 100, Height: 100, RegionCols: 5, RegionRows: 4, SlotDurationMS: 1000},
		Metadata: Metadata{DSN: "postgres://skygrid:skygrid@localhost:5440/skygrid"},
		Shards: ShardList{
			{0, "postgres://skygrid:skygrid@localhost:5450/skygrid"},
			{1, "postgres://skygrid:skygrid@localhost:5451/skygrid"},
			{2, "postgres://skygrid:skygrid@localhost:5452/skygrid"},
			{3, "postgres://skygrid:skygrid@localhost:5453/skygrid"},
		},
		Coordinator: Coordinator{
			ID: "coord-1", ListenAddr: ":8080", LockTimeoutMS: 100, MaxAttempts: 5,
			BackoffBaseMS: 10, BackoffMaxMS: 200, RoutingRefreshMS: 1000, PoolSizePerShard: 20,
		},
		Recovery: Recovery{ScanIntervalMS: 2000, PreparedAgeThresholdMS: 5000},
		Simulator: Simulator{
			CoordinatorURL: "http://localhost:8080", Drones: 500, RequestRatePerSec: 200,
			CrossRegionRatio: 0.25, HotSpot: false, DurationSec: 60,
		},
	}
	if got := *cfg; !equalConfig(got, want) {
		t.Errorf("config mismatch\n got: %+v\nwant: %+v", got, want)
	}
}

func equalConfig(a, b Config) bool {
	if len(a.Shards) != len(b.Shards) {
		return false
	}
	for i := range a.Shards {
		if a.Shards[i] != b.Shards[i] {
			return false
		}
	}
	return a.Grid == b.Grid && a.Metadata == b.Metadata && a.Coordinator == b.Coordinator &&
		a.Recovery == b.Recovery && a.Simulator == b.Simulator
}

func TestLoadFromFile(t *testing.T) {
	cfg, err := Load(repoConfig)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Coordinator.ID != "coord-1" {
		t.Errorf("coordinator.id = %q", cfg.Coordinator.ID)
	}
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Error("Load of missing file succeeded")
	}
}

func TestLoadUsesProcessEnv(t *testing.T) {
	t.Setenv("SKYGRID_COORDINATOR_LOCK_TIMEOUT_MS", "50")
	cfg, err := Load(repoConfig)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Coordinator.LockTimeoutMS != 50 {
		t.Errorf("lock_timeout_ms = %d, want 50", cfg.Coordinator.LockTimeoutMS)
	}
}

func TestDurations(t *testing.T) {
	cfg, err := Parse(readRepoConfig(t), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"slot", cfg.Grid.SlotDuration(), time.Second},
		{"lock timeout", cfg.Coordinator.LockTimeout(), 100 * time.Millisecond},
		{"backoff base", cfg.Coordinator.BackoffBase(), 10 * time.Millisecond},
		{"backoff max", cfg.Coordinator.BackoffMax(), 200 * time.Millisecond},
		{"routing refresh", cfg.Coordinator.RoutingRefresh(), time.Second},
		{"scan interval", cfg.Recovery.ScanInterval(), 2 * time.Second},
		{"prepared age", cfg.Recovery.PreparedAgeThreshold(), 5 * time.Second},
		{"sim duration", cfg.Simulator.Duration(), time.Minute},
	}
	for _, tt := range tests {
		if tt.got != tt.want {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
}

func TestEnvOverrides(t *testing.T) {
	env := mapEnv(map[string]string{
		"SKYGRID_COORDINATOR_LOCK_TIMEOUT_MS":  "50",
		"SKYGRID_COORDINATOR_ID":               "coord-7",
		"SKYGRID_COORDINATOR_MAX_ATTEMPTS":     "1",
		"SKYGRID_METADATA_DSN":                 "postgres://u:p@meta:5432/skygrid",
		"SKYGRID_SHARD_2_DSN":                  "postgres://u:p@shard2:5432/skygrid",
		"SKYGRID_SIMULATOR_CROSS_REGION_RATIO": "0.75",
		"SKYGRID_SIMULATOR_HOT_SPOT":           "true",
		"SKYGRID_GRID_WIDTH":                   " 200 ",
		"SKYGRID_SHARD_9_DSN":                  "ignored: no such shard",
	})
	cfg, err := Parse(readRepoConfig(t), env)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Coordinator.LockTimeoutMS != 50 || cfg.Coordinator.LockTimeout() != 50*time.Millisecond {
		t.Errorf("lock timeout = %d", cfg.Coordinator.LockTimeoutMS)
	}
	if cfg.Coordinator.MaxAttempts != 1 {
		t.Errorf("max_attempts = %d, want 1", cfg.Coordinator.MaxAttempts)
	}
	if cfg.Coordinator.ID != "coord-7" {
		t.Errorf("coordinator.id = %q", cfg.Coordinator.ID)
	}
	if cfg.Metadata.DSN != "postgres://u:p@meta:5432/skygrid" {
		t.Errorf("metadata.dsn = %q", cfg.Metadata.DSN)
	}
	if dsn, _ := cfg.ShardDSN(2); dsn != "postgres://u:p@shard2:5432/skygrid" {
		t.Errorf("shard 2 dsn = %q", dsn)
	}
	if dsn, _ := cfg.ShardDSN(1); dsn != "postgres://skygrid:skygrid@localhost:5451/skygrid" {
		t.Errorf("shard 1 dsn changed: %q", dsn)
	}
	if cfg.Simulator.CrossRegionRatio != 0.75 || !cfg.Simulator.HotSpot {
		t.Errorf("simulator = %+v", cfg.Simulator)
	}
	if cfg.Grid.Width != 200 {
		t.Errorf("grid.width = %d", cfg.Grid.Width)
	}
}

func TestEnvOverrideErrors(t *testing.T) {
	tests := map[string]string{
		"SKYGRID_COORDINATOR_LOCK_TIMEOUT_MS":  "fast",
		"SKYGRID_SIMULATOR_HOT_SPOT":           "maybe",
		"SKYGRID_SIMULATOR_CROSS_REGION_RATIO": "half",
	}
	for key, val := range tests {
		_, err := Parse(readRepoConfig(t), mapEnv(map[string]string{key: val}))
		if err == nil || !strings.Contains(err.Error(), key) {
			t.Errorf("%s=%q: got err %v, want error naming the variable", key, val, err)
		}
	}
}

func TestEnvOverrideIsValidated(t *testing.T) {
	_, err := Parse(readRepoConfig(t), mapEnv(map[string]string{"SKYGRID_COORDINATOR_LOCK_TIMEOUT_MS": "0"}))
	if err == nil || !strings.Contains(err.Error(), "coordinator.lock_timeout_ms") {
		t.Errorf("got %v, want lock_timeout_ms validation error", err)
	}
}

func TestEnvKeys(t *testing.T) {
	if got := EnvKey("coordinator", "lock_timeout_ms"); got != "SKYGRID_COORDINATOR_LOCK_TIMEOUT_MS" {
		t.Errorf("EnvKey = %q", got)
	}
	if got := EnvKey("coordinator", "max_attempts"); got != "SKYGRID_COORDINATOR_MAX_ATTEMPTS" {
		t.Errorf("EnvKey = %q", got)
	}
	if got := ShardEnvKey(3); got != "SKYGRID_SHARD_3_DSN" {
		t.Errorf("ShardEnvKey = %q", got)
	}
}

func TestParseRejectsBadYAML(t *testing.T) {
	tests := map[string]string{
		"empty":        "",
		"unknown key":  strings.Replace(string(readRepoConfig(t)), "max_attempts:", "max_attempt:", 1),
		"wrong type":   strings.Replace(string(readRepoConfig(t)), "max_attempts: 5", "max_attempts: five", 1),
		"not yaml map": "- just\n- a list\n",
	}
	for name, data := range tests {
		if _, err := Parse([]byte(data), noEnv); err == nil {
			t.Errorf("%s: Parse succeeded, want error", name)
		}
	}
}

func TestValidateFailures(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr string
	}{
		{"width not divisible", func(c *Config) { c.Grid.Width = 101 }, "grid.width (101) must be divisible by grid.region_cols (5)"},
		{"height not divisible", func(c *Config) { c.Grid.Height = 99 }, "grid.height (99) must be divisible by grid.region_rows (4)"},
		{"zero width", func(c *Config) { c.Grid.Width = 0 }, "grid.width must be positive"},
		{"zero region cols", func(c *Config) { c.Grid.RegionCols = 0 }, "grid.region_cols must be positive"},
		{"negative region rows", func(c *Config) { c.Grid.RegionRows = -4 }, "grid.region_rows must be positive"},
		{"zero slot", func(c *Config) { c.Grid.SlotDurationMS = 0 }, "grid.slot_duration_ms must be positive"},
		{"huge grid", func(c *Config) { c.Grid.Width, c.Grid.Height = 100000, 100000 }, "exceeds the Postgres INTEGER range"},
		{"empty metadata dsn", func(c *Config) { c.Metadata.DSN = "" }, "metadata.dsn must not be empty"},
		{"no shards", func(c *Config) { c.Shards = nil }, "at least one shard"},
		{"duplicate shard", func(c *Config) { c.Shards[3].ID = 2 }, "duplicate shard id 2"},
		{"shards start at 1", func(c *Config) {
			for i := range c.Shards {
				c.Shards[i].ID++
			}
		}, "ids must be 0..3"},
		{"shard gap", func(c *Config) { c.Shards[3].ID = 7 }, "ids must be 0..3"},
		{"empty shard dsn", func(c *Config) { c.Shards[1].DSN = "" }, "shards[1] (id 1): dsn must not be empty"},
		{"empty coordinator id", func(c *Config) { c.Coordinator.ID = "" }, "coordinator.id"},
		{"bad coordinator id", func(c *Config) { c.Coordinator.ID = "coord'1" }, "coordinator.id"},
		{"coordinator id too long", func(c *Config) { c.Coordinator.ID = strings.Repeat("a", txnid.MaxCoordinatorIDLen+1) }, "coordinator.id"},
		{"empty listen addr", func(c *Config) { c.Coordinator.ListenAddr = "" }, "coordinator.listen_addr"},
		{"zero lock timeout", func(c *Config) { c.Coordinator.LockTimeoutMS = 0 }, "coordinator.lock_timeout_ms must be positive"},
		{"zero attempts", func(c *Config) { c.Coordinator.MaxAttempts = 0 }, "coordinator.max_attempts must be at least 1"},
		{"negative attempts", func(c *Config) { c.Coordinator.MaxAttempts = -1 }, "coordinator.max_attempts must be at least 1"},
		{"zero backoff base", func(c *Config) { c.Coordinator.BackoffBaseMS = 0 }, "coordinator.backoff_base_ms must be positive"},
		{"backoff max < base", func(c *Config) { c.Coordinator.BackoffMaxMS = 5 }, "backoff_max_ms (5) must be >= coordinator.backoff_base_ms (10)"},
		{"zero routing refresh", func(c *Config) { c.Coordinator.RoutingRefreshMS = 0 }, "coordinator.routing_refresh_ms must be positive"},
		{"zero pool", func(c *Config) { c.Coordinator.PoolSizePerShard = 0 }, "coordinator.pool_size_per_shard must be positive"},
		{"zero scan interval", func(c *Config) { c.Recovery.ScanIntervalMS = 0 }, "recovery.scan_interval_ms must be positive"},
		{"negative age", func(c *Config) { c.Recovery.PreparedAgeThresholdMS = -1 }, "recovery.prepared_age_threshold_ms must be positive"},
		{"bad url", func(c *Config) { c.Simulator.CoordinatorURL = "localhost:8080" }, "simulator.coordinator_url"},
		{"zero drones", func(c *Config) { c.Simulator.Drones = 0 }, "simulator.drones must be positive"},
		{"zero rate", func(c *Config) { c.Simulator.RequestRatePerSec = 0 }, "simulator.request_rate_per_sec must be positive"},
		{"ratio above 1", func(c *Config) { c.Simulator.CrossRegionRatio = 1.5 }, "simulator.cross_region_ratio must be in [0, 1]"},
		{"ratio below 0", func(c *Config) { c.Simulator.CrossRegionRatio = -0.1 }, "simulator.cross_region_ratio must be in [0, 1]"},
		{"zero duration", func(c *Config) { c.Simulator.DurationSec = 0 }, "simulator.duration_sec must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(readRepoConfig(t), noEnv)
			if err != nil {
				t.Fatal(err)
			}
			tt.mutate(cfg)
			err = cfg.Validate()
			if err == nil {
				t.Fatalf("Validate succeeded, want error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestOldMaxRetriesKeyRejected(t *testing.T) {
	old := strings.Replace(string(readRepoConfig(t)), "max_attempts:", "max_retries:", 1)
	if _, err := Parse([]byte(old), noEnv); err == nil || !strings.Contains(err.Error(), "max_retries") {
		t.Errorf("old max_retries key: got %v, want unknown-field error", err)
	}
}

func TestMaxAttemptsOneAccepted(t *testing.T) {
	cfg, err := Parse(readRepoConfig(t), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Coordinator.MaxAttempts = 1
	if err := cfg.Validate(); err != nil {
		t.Errorf("max_attempts 1 rejected: %v", err)
	}
}

func TestValidateBoundaryRatios(t *testing.T) {
	for _, r := range []float64{0, 1} {
		cfg, err := Parse(readRepoConfig(t), noEnv)
		if err != nil {
			t.Fatal(err)
		}
		cfg.Simulator.CrossRegionRatio = r
		if err := cfg.Validate(); err != nil {
			t.Errorf("ratio %v rejected: %v", r, err)
		}
	}
}

func TestValidateReportsAllErrors(t *testing.T) {
	cfg, err := Parse(readRepoConfig(t), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Grid.Width = 101
	cfg.Recovery.ScanIntervalMS = 0
	err = cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "grid.width") || !strings.Contains(err.Error(), "recovery.scan_interval_ms") {
		t.Errorf("want both errors reported, got %v", err)
	}
}

func TestLongestCoordinatorIDAccepted(t *testing.T) {
	cfg, err := Parse(readRepoConfig(t), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Coordinator.ID = strings.Repeat("a", txnid.MaxCoordinatorIDLen)
	if err := cfg.Validate(); err != nil {
		t.Errorf("%d-char coordinator id rejected: %v", txnid.MaxCoordinatorIDLen, err)
	}
	if txnid.MaxCoordinatorIDLen != 150 {
		t.Errorf("MaxCoordinatorIDLen = %d, want 150 (199 - len(\"skygrid:\") - 1 - 19 - 1 - 20)", txnid.MaxCoordinatorIDLen)
	}
}

func TestShardsInAnyOrder(t *testing.T) {
	cfg, err := Parse(readRepoConfig(t), noEnv)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Shards[0], cfg.Shards[3] = cfg.Shards[3], cfg.Shards[0]
	if err := cfg.Validate(); err != nil {
		t.Errorf("reordered shards rejected: %v", err)
	}
}

func TestRedactDSN(t *testing.T) {
	tests := []struct{ in, want string }{
		{"postgres://skygrid:secret@localhost:5440/skygrid", "postgres://skygrid:xxxxx@localhost:5440/skygrid"},
		{"postgres://skygrid@localhost/skygrid", "postgres://skygrid@localhost/skygrid"},
		{"postgres://u@h/db?password=secret&sslmode=disable", "postgres://u@h/db?password=xxxxx&sslmode=disable"},
		{"host=localhost user=skygrid password=secret dbname=skygrid", "host=localhost user=skygrid password=xxxxx dbname=skygrid"},
		{"host=localhost password='se cret' dbname=skygrid", "host=localhost password=xxxxx dbname=skygrid"},
	}
	for _, tt := range tests {
		if got := RedactDSN(tt.in); got != tt.want {
			t.Errorf("RedactDSN(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestLoggingNeverLeaksPasswords(t *testing.T) {
	cfg, err := Parse(readRepoConfig(t), mapEnv(map[string]string{
		"SKYGRID_METADATA_DSN": "postgres://u:topsecret@meta/skygrid",
		"SKYGRID_SHARD_0_DSN":  "postgres://u:topsecret@s0/skygrid",
	}))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&buf, nil))
	log.Info("x", "config", cfg, "metadata", cfg.Metadata, "shards", cfg.Shards, "shard", cfg.Shards[0])
	out := buf.String()
	if strings.Contains(out, "topsecret") || strings.Contains(out, ":skygrid@") {
		t.Errorf("log output leaks a password: %s", out)
	}
	if !strings.Contains(out, "u:xxxxx@meta") || !strings.Contains(out, "u:xxxxx@s0") {
		t.Errorf("log output missing redacted DSNs: %s", out)
	}
}
