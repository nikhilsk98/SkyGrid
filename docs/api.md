# Coordinator API (v1)

All bodies are JSON. Coordinates are 0-based: `0 <= x < grid.width`, `0 <= y < grid.height`.

## POST /reserve

Reserve the full flight path for a drone. All-or-nothing: either every
(cell, slot) on the path is reserved, or none are.

### Request
```json
{
  "drone_id": 57,
  "from": { "x": 3,  "y": 10 },
  "to":   { "x": 80, "y": 62 },
  "start_slot": 1795000100
}
```
`start_slot` uses the slot clock from `design.md` section 2:
`slot = unix_ms / slot_duration_ms`. It must not be earlier than the
coordinator's current slot, or the request is rejected with 400.

### 200 OK: reserved
```json
{
  "status": "RESERVED",
  "txn_id": "coord-1-1795000000000-000123",
  "start_slot": 1795000102,
  "end_slot": 1795000231,
  "path_length": 130,
  "shards": [0, 1, 2, 3],
  "cross_shard": true,
  "attempts": 3
}
```
`start_slot` may be later than requested if the drone had to delay departure
(see retry policy in `design.md`). Here two `unique_violation` retries pushed
departure from slot `1795000100` to `1795000102`, so `attempts` is 3.
`path_length = |80-3| + |62-10| + 1 = 130` and
`end_slot = start_slot + path_length - 1`. The path crosses regions
0, 1, 2, 3, 4 (along `y = 10`) and 9, 14 (along `x = 80`), which the seeded
`region_id % 4` placement maps to shards 0, 1, 2, 3.

### 409 Conflict: could not reserve within `max_attempts`
```json
{
  "status": "CONFLICT",
  "txn_id": "coord-1-1795000000000-000124",
  "attempts": 5,
  "reason": "cell 4580 slot 1795000216 already reserved"
}
```
`attempts` counts every try including the first, so on a 409 it equals
`coordinator.max_attempts` (default 5). Lock timeouts and `start_slot + 1`
delays share that one budget (see retry policy in `design.md`).
In this example a request identical to the one above hit `unique_violation` on every
attempt, so the fifth attempt started at slot `1795000104` and failed at cell
4580 = (80, 45), which is step 112 of the path: `1795000104 + 112 = 1795000216`.

### 503 Service Unavailable
```json
{ "status": "UNAVAILABLE", "reason": "region 7 is migrating" }
```
Also returned when a shard is unreachable.

### 400 Bad Request
```json
{ "status": "INVALID", "reason": "to.x out of range" }
```
Also returned when `start_slot` is earlier than the current slot:
```json
{ "status": "INVALID", "reason": "start_slot 1794999990 is in the past (current slot 1795000000)" }
```

## GET /health
`200 {"status":"ok"}` when the coordinator can reach the metadata DB.

## GET /metrics
Prometheus format (added in Phase 4). Metric names are fixed in `design.md`.
