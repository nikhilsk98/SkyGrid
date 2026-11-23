-- Initial placement: 'scattered' (region_id % 4).
-- With a 5x4 region layout this puts horizontal neighbours on different
-- shards, which is the deliberate baseline for the placement experiment.
INSERT INTO routing_table (region_id, shard_id)
SELECT r, r % 4
FROM generate_series(0, 19) AS r
ON CONFLICT (region_id) DO NOTHING;

INSERT INTO routing_version (id, version, placement)
VALUES (TRUE, 1, 'scattered')
ON CONFLICT (id) DO NOTHING;
