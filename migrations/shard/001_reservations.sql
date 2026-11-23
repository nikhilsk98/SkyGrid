-- Reservations owned by this shard.
-- The primary key is the database-level safety guarantee:
-- one (cell, slot) can never be held by two drones.
CREATE TABLE IF NOT EXISTS reservations (
    cell_id     INTEGER     NOT NULL,
    time_slot   BIGINT      NOT NULL,
    region_id   INTEGER     NOT NULL,   -- kept so a whole region can be moved during rebalancing
    drone_id    INTEGER     NOT NULL,
    txn_id      TEXT        NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (cell_id, time_slot)
);

CREATE INDEX IF NOT EXISTS idx_reservations_region ON reservations (region_id);
CREATE INDEX IF NOT EXISTS idx_reservations_txn    ON reservations (txn_id);
