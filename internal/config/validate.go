package config

import (
	"errors"
	"fmt"
	"math"
	"net/url"
	"sort"

	"github.com/nikhilsk98/SkyGrid/internal/txnid"
)

// Validate checks every section and returns all problems at once, joined.
func (c *Config) Validate() error {
	var errs []error
	add := func(format string, args ...any) { errs = append(errs, fmt.Errorf(format, args...)) }
	positive := func(name string, v int) {
		if v <= 0 {
			add("%s must be positive, got %d", name, v)
		}
	}

	// grid
	g := c.Grid
	positive("grid.width", g.Width)
	positive("grid.height", g.Height)
	positive("grid.region_cols", g.RegionCols)
	positive("grid.region_rows", g.RegionRows)
	positive("grid.slot_duration_ms", g.SlotDurationMS)
	if g.Width > 0 && g.RegionCols > 0 && g.Width%g.RegionCols != 0 {
		add("grid.width (%d) must be divisible by grid.region_cols (%d)", g.Width, g.RegionCols)
	}
	if g.Height > 0 && g.RegionRows > 0 && g.Height%g.RegionRows != 0 {
		add("grid.height (%d) must be divisible by grid.region_rows (%d)", g.Height, g.RegionRows)
	}
	if g.Width > 0 && g.Height > 0 && int64(g.Width)*int64(g.Height) > math.MaxInt32 {
		add("grid.width*grid.height (%d) exceeds the Postgres INTEGER range used for cell_id", int64(g.Width)*int64(g.Height))
	}

	// metadata
	if c.Metadata.DSN == "" {
		add("metadata.dsn must not be empty")
	}

	// shards
	errs = append(errs, c.Shards.validate()...)

	// coordinator
	co := c.Coordinator
	if err := txnid.ValidateCoordinatorID(co.ID); err != nil {
		add("coordinator.id: %w", err)
	}
	if co.ListenAddr == "" {
		add("coordinator.listen_addr must not be empty")
	}
	positive("coordinator.lock_timeout_ms", co.LockTimeoutMS)
	if co.MaxAttempts < 1 {
		add("coordinator.max_attempts must be at least 1, got %d", co.MaxAttempts)
	}
	positive("coordinator.backoff_base_ms", co.BackoffBaseMS)
	positive("coordinator.backoff_max_ms", co.BackoffMaxMS)
	if co.BackoffBaseMS > 0 && co.BackoffMaxMS > 0 && co.BackoffMaxMS < co.BackoffBaseMS {
		add("coordinator.backoff_max_ms (%d) must be >= coordinator.backoff_base_ms (%d)", co.BackoffMaxMS, co.BackoffBaseMS)
	}
	positive("coordinator.routing_refresh_ms", co.RoutingRefreshMS)
	positive("coordinator.pool_size_per_shard", co.PoolSizePerShard)
	if co.PoolSizePerShard > math.MaxInt32 {
		add("coordinator.pool_size_per_shard (%d) is too large", co.PoolSizePerShard)
	}

	// recovery
	positive("recovery.scan_interval_ms", c.Recovery.ScanIntervalMS)
	positive("recovery.prepared_age_threshold_ms", c.Recovery.PreparedAgeThresholdMS)

	// simulator
	s := c.Simulator
	if u, err := url.Parse(s.CoordinatorURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		add("simulator.coordinator_url must be an http(s) URL with a host, got %q", s.CoordinatorURL)
	}
	positive("simulator.drones", s.Drones)
	if !(s.RequestRatePerSec > 0) || math.IsInf(s.RequestRatePerSec, 0) {
		add("simulator.request_rate_per_sec must be positive, got %v", s.RequestRatePerSec)
	}
	if !(s.CrossRegionRatio >= 0 && s.CrossRegionRatio <= 1) {
		add("simulator.cross_region_ratio must be in [0, 1], got %v", s.CrossRegionRatio)
	}
	positive("simulator.duration_sec", s.DurationSec)

	return errors.Join(errs...)
}

// validate requires at least one shard, non-empty DSNs, and IDs that are
// exactly 0..n-1 with no duplicates or gaps (in any file order).
func (s ShardList) validate() []error {
	if len(s) == 0 {
		return []error{errors.New("shards: at least one shard is required")}
	}
	var errs []error
	seen := make(map[int]bool, len(s))
	ids := make([]int, 0, len(s))
	for i, sh := range s {
		if sh.DSN == "" {
			errs = append(errs, fmt.Errorf("shards[%d] (id %d): dsn must not be empty", i, sh.ID))
		}
		if seen[sh.ID] {
			errs = append(errs, fmt.Errorf("shards[%d]: duplicate shard id %d", i, sh.ID))
			continue
		}
		seen[sh.ID] = true
		ids = append(ids, sh.ID)
	}
	sort.Ints(ids)
	for want, got := range ids {
		if got != want {
			errs = append(errs, fmt.Errorf("shards: ids must be 0..%d with no gaps, found %v", len(s)-1, ids))
			break
		}
	}
	return errs
}
