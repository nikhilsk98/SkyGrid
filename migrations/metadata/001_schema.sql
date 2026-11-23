-- Which shard owns each logical region.
CREATE TABLE IF NOT EXISTS routing_table (
    region_id   INTEGER     PRIMARY KEY,
    shard_id    INTEGER     NOT NULL,
    status      TEXT        NOT NULL DEFAULT 'ACTIVE'
                            CHECK (status IN ('ACTIVE', 'MIGRATING')),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Single-row table. Coordinators cache the routing table and reload it
-- whenever this version changes.
CREATE TABLE IF NOT EXISTS routing_version (
    id          BOOLEAN     PRIMARY KEY DEFAULT TRUE CHECK (id),
    version     BIGINT      NOT NULL,
    placement   TEXT        NOT NULL,   -- e.g. 'scattered', 'neighbor'
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- 2PC decisions for cross-shard transactions.
-- The PRIMARY KEY on txn_id is what resolves the coordinator-vs-recovery race:
-- whoever inserts a decision first wins (see docs/design.md).
CREATE TABLE IF NOT EXISTS decision_log (
    txn_id        TEXT        PRIMARY KEY,
    decision      TEXT        NOT NULL CHECK (decision IN ('COMMIT', 'ABORT')),
    participants  INTEGER[]   NOT NULL,   -- shard ids involved
    decided_by    TEXT        NOT NULL,   -- coordinator id or 'recovery'
    decided_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Non-critical data (not on the reservation hot path).
CREATE TABLE IF NOT EXISTS drones (
    drone_id    INTEGER     PRIMARY KEY,
    home_cell   INTEGER     NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS flights (
    txn_id       TEXT        PRIMARY KEY,
    drone_id     INTEGER     NOT NULL,
    from_cell    INTEGER     NOT NULL,
    to_cell      INTEGER     NOT NULL,
    start_slot   BIGINT      NOT NULL,
    end_slot     BIGINT      NOT NULL,
    cross_shard  BOOLEAN     NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
