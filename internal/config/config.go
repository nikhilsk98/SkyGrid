// Package config loads config/skygrid.yaml, the single source of truth shared
// by every SkyGrid component, and applies SKYGRID_* environment overrides.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the config file every binary reads unless told otherwise.
const DefaultPath = "config/skygrid.yaml"

// Config mirrors config/skygrid.yaml.
type Config struct {
	Grid        Grid        `yaml:"grid" json:"grid"`
	Metadata    Metadata    `yaml:"metadata" json:"metadata"`
	Shards      ShardList   `yaml:"shards" json:"shards"`
	Coordinator Coordinator `yaml:"coordinator" json:"coordinator"`
	Recovery    Recovery    `yaml:"recovery" json:"recovery"`
	Simulator   Simulator   `yaml:"simulator" json:"simulator"`
}

// Grid describes the city grid and its split into logical regions.
type Grid struct {
	Width          int `yaml:"width" json:"width"`
	Height         int `yaml:"height" json:"height"`
	RegionCols     int `yaml:"region_cols" json:"region_cols"`
	RegionRows     int `yaml:"region_rows" json:"region_rows"`
	SlotDurationMS int `yaml:"slot_duration_ms" json:"slot_duration_ms"`
}

// SlotDuration is the wall-clock length of one time slot.
func (g Grid) SlotDuration() time.Duration { return ms(g.SlotDurationMS) }

// Metadata locates the shared metadata database.
type Metadata struct {
	DSN string `yaml:"dsn" json:"dsn"`
}

// Shard locates one reservation shard.
type Shard struct {
	ID  int    `yaml:"id" json:"id"`
	DSN string `yaml:"dsn" json:"dsn"`
}

// ShardList is the configured set of shards, in file order.
type ShardList []Shard

// Coordinator configures the reservation service.
type Coordinator struct {
	ID               string `yaml:"id" json:"id"`
	ListenAddr       string `yaml:"listen_addr" json:"listen_addr"`
	LockTimeoutMS    int    `yaml:"lock_timeout_ms" json:"lock_timeout_ms"`
	MaxAttempts      int    `yaml:"max_attempts" json:"max_attempts"`
	BackoffBaseMS    int    `yaml:"backoff_base_ms" json:"backoff_base_ms"`
	BackoffMaxMS     int    `yaml:"backoff_max_ms" json:"backoff_max_ms"`
	RoutingRefreshMS int    `yaml:"routing_refresh_ms" json:"routing_refresh_ms"`
	PoolSizePerShard int    `yaml:"pool_size_per_shard" json:"pool_size_per_shard"`
}

// LockTimeout is the Postgres lock_timeout used inside reservation transactions.
func (c Coordinator) LockTimeout() time.Duration { return ms(c.LockTimeoutMS) }

// BackoffBase is the lower bound of the first retry backoff.
func (c Coordinator) BackoffBase() time.Duration { return ms(c.BackoffBaseMS) }

// BackoffMax is the upper bound of any retry backoff.
func (c Coordinator) BackoffMax() time.Duration { return ms(c.BackoffMaxMS) }

// RoutingRefresh is how often the coordinator checks routing_version.
func (c Coordinator) RoutingRefresh() time.Duration { return ms(c.RoutingRefreshMS) }

// Recovery configures the prepared-transaction recovery service.
type Recovery struct {
	ScanIntervalMS         int `yaml:"scan_interval_ms" json:"scan_interval_ms"`
	PreparedAgeThresholdMS int `yaml:"prepared_age_threshold_ms" json:"prepared_age_threshold_ms"`
}

// ScanInterval is how often recovery scans every shard.
func (r Recovery) ScanInterval() time.Duration { return ms(r.ScanIntervalMS) }

// PreparedAgeThreshold is the minimum age of a prepared txn before recovery touches it.
func (r Recovery) PreparedAgeThreshold() time.Duration { return ms(r.PreparedAgeThresholdMS) }

// Simulator configures the load generator.
type Simulator struct {
	CoordinatorURL    string  `yaml:"coordinator_url" json:"coordinator_url"`
	Drones            int     `yaml:"drones" json:"drones"`
	RequestRatePerSec float64 `yaml:"request_rate_per_sec" json:"request_rate_per_sec"`
	CrossRegionRatio  float64 `yaml:"cross_region_ratio" json:"cross_region_ratio"`
	HotSpot           bool    `yaml:"hot_spot" json:"hot_spot"`
	DurationSec       int     `yaml:"duration_sec" json:"duration_sec"`
}

// Duration is how long one simulator run lasts.
func (s Simulator) Duration() time.Duration { return time.Duration(s.DurationSec) * time.Second }

func ms(v int) time.Duration { return time.Duration(v) * time.Millisecond }

// LookupEnv has the signature of os.LookupEnv; tests pass fakes.
type LookupEnv func(key string) (string, bool)

// Load reads the YAML file at path, applies environment overrides from the
// process environment, and validates the result.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is an operator-supplied flag
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	cfg, err := Parse(data, os.LookupEnv)
	if err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	return cfg, nil
}

// Parse decodes YAML, applies overrides found through env (nil means no
// overrides), and validates the result. Unknown YAML keys are rejected so
// typos do not silently fall back to zero values.
func Parse(data []byte, env LookupEnv) (*Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("config is empty")
		}
		return nil, fmt.Errorf("decode yaml: %w", err)
	}
	if env != nil {
		if err := applyEnv(&cfg, env); err != nil {
			return nil, err
		}
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ShardDSN returns the DSN of the shard with the given ID.
func (c *Config) ShardDSN(id int) (string, bool) {
	for _, s := range c.Shards {
		if s.ID == id {
			return s.DSN, true
		}
	}
	return "", false
}
