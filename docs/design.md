# SkyGrid design decisions (Phase 0)

These are the agreements every component must follow. Change them only by
team agreement, and update this file when you do.

## 1. Grid and IDs

- Grid: `width x height` cells (default 100 x 100).
- `cell_id = y * width + x`
- Regions: `region_cols x region_rows` blocks (default 5 x 4 = 20 regions,
  each 20 x 25 cells).
  - `region_w = width / region_cols`, `region_h = height / region_rows`
  - `region_id = (y / region_h) * region_cols + (x / region_w)` (integer division)
- Shards are 0-based (`shard-0` ... `shard-3`).
- `region -> shard` comes **only** from `routing_table`; never hard-code it.

## 2. Path rule (deterministic)

- L-shaped path: move along x first, then along y, one cell per step.
- Both the start and end cells are included.
- Cell number `i` on the path (0-based) is reserved at slot `start_slot + i`.
- So `path_length = |dx| + |dy| + 1` and `end_slot = start_slot + path_length - 1`.

Everyone (coordinator, simulator, checker) must produce identical paths for the
same input. Keep this in one shared function with unit tests.

### Slot clock

- `slot = unix_ms / slot_duration_ms` (integer division), where `unix_ms` is
  wall-clock Unix time in milliseconds. With the default 1000 ms slots, slot
  `1795000000` starts at `unix_ms = 1795000000000`.
- Clients request slots in the future. The coordinator computes the current
  slot from its own clock and rejects a `start_slot` earlier than the current
  slot with `400 INVALID`. `start_slot` equal to the current slot is accepted.
- Every host's clock must be NTP-synced; skew between the simulator and the
  coordinator shows up directly as rejected or late requests.

## 3. Transaction IDs

- `txn_id = <coordinator_id>-<boot_unix_ms>-<counter>`,
  e.g. `coord-1-1795000000000-000123`.
  - `boot_unix_ms`: Unix time in milliseconds, captured once when the
    coordinator starts its ID generator.
  - `counter`: starts at 1 on every boot, zero-padded to at least 6 digits
    (it grows past 6 digits rather than wrapping).
  - Including the boot time means a restarted coordinator never reuses a
    `txn_id` from a previous run, so it cannot collide with old
    `decision_log` or `flights` rows.
  - `boot_unix_ms` and `counter` never contain `-`, so parse from the right;
    coordinator IDs may contain `-`.
- `coordinator_id` may contain only ASCII letters, digits, `-` and `_`, and is
  at most 150 characters, so the GID never needs escaping and always fits in
  Postgres's 199-byte GID limit:
  `8 ("skygrid:") + 150 + 1 + 19 (max int64) + 1 + 20 (max uint64) = 199`.
- Postgres prepared-transaction GID: `skygrid:<txn_id>`.
  The recovery service only touches GIDs starting with `skygrid:`.

## 4. Reservation protocol

### Single-shard path
One ordinary transaction on that shard. No decision log entry.

### Multi-shard path (2PC)
1. Sort participating shards by `shard_id` ascending (prevents lock-order deadlocks).
2. On each shard, in that order:
   `BEGIN; SET LOCAL lock_timeout = '<lock_timeout_ms>ms';`
   insert this shard's rows (sorted by `cell_id, time_slot`);
   `PREPARE TRANSACTION 'skygrid:<txn_id>';`
3. If any shard fails to prepare: `ROLLBACK PREPARED` on the ones that did
   prepare (no decision log entry is needed; see presumed abort below).
4. If all prepared: `INSERT INTO decision_log (... 'COMMIT' ...)`.
   - If that insert **conflicts** (recovery already logged ABORT), the
     coordinator must abort: `ROLLBACK PREPARED` everywhere.
5. `COMMIT PREPARED 'skygrid:<txn_id>'` on every shard.

### Presumed abort and the decision race
- No decision log entry means the transaction is treated as aborted.
- The recovery service, before rolling back a stale prepared transaction,
  first does `INSERT INTO decision_log (... 'ABORT', decided_by='recovery')
  ON CONFLICT DO NOTHING`, then re-reads the row and follows whatever decision
  is actually stored.
- Because `txn_id` is the primary key, the coordinator and recovery can never
  both "win". This prevents a slow-but-alive coordinator from committing on one
  shard after recovery rolled back another.

## 5. Conflict and retry policy

| Postgres error | Meaning | Action |
|---|---|---|
| `lock_not_available` (55P03) | Cell held by an in-flight txn | Abort, back off, retry the **same** path |
| `unique_violation` (23505) | Cell already committed to another drone | Abort, retry with `start_slot + 1` (drone delays departure) |
| Shard unreachable | Failure | Abort, return 503 after retries |

Backoff ("full jitter"): before attempt `n` (`n >= 2`), wait a uniformly
random duration in

    [0, min(backoff_max_ms, backoff_base_ms * 2^(n-2))]

milliseconds. With the defaults (`10`, `200`) the upper bound is 10 ms before
attempt 2, then 20, 40, 80, and 160 ms before attempt 6; it never exceeds
200 ms. There is no wait before the first attempt.

Attempt budget: `coordinator.max_attempts` is the total number of attempts per
request, **including the first** (`max_attempts = 1` means no retries). The
`lock_not_available` and `unique_violation` retries share this single budget;
each retry of either kind uses one attempt. When the last attempt fails with a
conflict, return 409 with `attempts = max_attempts`.

## 6. Safety invariants (checked by `cmd/checker/`)

1. No `(cell_id, time_slot)` appears on more than one shard (the PK already
   prevents duplicates within a shard).
2. Every row for a `txn_id` is either fully present on all its shards or absent
   everywhere (atomicity).
3. After recovery has run, no `skygrid:*` prepared transactions remain.

## 7. Metric names (fixed now so the dashboard works later)

| Metric | Type | Labels |
|---|---|---|
| `skygrid_reserve_requests_total` | counter | `result` = reserved / conflict / unavailable / invalid |
| `skygrid_reserve_latency_seconds` | histogram | `cross_shard` = true / false |
| `skygrid_2pc_total` | counter | `outcome` = commit / abort |
| `skygrid_retries_total` | counter | `reason` = lock_timeout / unique_violation / shard_error |
| `skygrid_recovery_actions_total` | counter | `action` = commit / rollback |

## 8. Placement strategies

- `scattered` (seeded default): `shard = region_id % shard_count`.
- `neighbor` (Phase 5): adjacent regions grouped on the same shard,
  e.g. each row of 5 regions on one shard.
- Switching placement = rewrite `routing_table` + bump `routing_version`
  (only on an empty system until live rebalancing exists).

## Known limitations

- `migrations/metadata/002_seed_routing.sql` seeds exactly 20 regions with
  `region_id % 4`, and `scripts/verify_setup.sh` expects 20 regions and
  shards 0-3. Changing `grid.region_cols`, `grid.region_rows` or the shard list
  in `config/skygrid.yaml` does **not** update `routing_table`; until a
  routing-seed tool is added in Phase 4, keep the default 5 x 4 regions and
  4 shards.
