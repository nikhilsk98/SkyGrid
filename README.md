# SkyGrid

A sharded airspace reservation system for delivery drones.

Drones reserve every (cell, time slot) on their flight path before flying.
The city grid is split into 20 logical regions spread across Postgres shards.
Cross-shard paths use two-phase commit, and the system guarantees that no two
drones ever hold the same cell at the same time, even through crashes and
network faults.

## Repository layout

```
config/          skygrid.yaml: single source of truth for all components
migrations/      SQL run automatically when containers are first created
  shard/         reservations table (every shard)
  metadata/      routing table, decision log, drones, flights
docs/
  api.md         coordinator API contract
  design.md      grid math, path rule, 2PC protocol, retry policy, metrics
cmd/             one main package per binary
  coordinator/   stateless reservation service (Phases 1-2)
  recovery/      resolves stuck prepared transactions (Phase 3)
  simulator/     load generator (Phase 1+)
  checker/       safety invariant checker (Phase 1+)
internal/        shared Go packages
  config/        loads skygrid.yaml + SKYGRID_* env overrides, validation
  grid/          grid math and the shared path rule (design.md sections 1-2)
  txnid/         transaction IDs and prepared-transaction GIDs (section 3)
  db/            pgx connection pools
deploy/          Dockerfile for all binaries (ARG SERVICE)
infra/terraform/ AWS deployment (Phase 4)
experiments/     experiment configs, runners, results (Phases 4-5)
scripts/         helper scripts
```

## Quick start

Requirements: Docker with Compose v2, make.

```bash
cp .env.example .env
make up        # start metadata DB + 4 shards
make verify    # Phase 0 checkpoint
```

Other commands:

```bash
make psql-meta        # open psql on the metadata DB
make psql-shard N=2   # open psql on shard-2
make logs             # follow container logs
make reset            # wipe all data and re-run migrations
make down             # stop everything
```

Ports: metadata `5440`, shards `5450`-`5453` (user/password `skygrid` locally).

## Team conventions

- Read `docs/design.md` before writing any component; it is the contract.
- Change the API, schema or design only by team agreement, in the same PR
  as the docs update.
- Branch per task (`feat/coordinator-single-shard`), PR into `main`, one reviewer.
- Never commit `.env`, Terraform state, or raw results.

## Phase checklist

- [ ] 0: Setup and contracts (`make verify` passes on every laptop)
- [ ] 1: Single-shard core
- [ ] 2: Sharding and 2PC
- [ ] 3: Fault tolerance
- [ ] 4: Observability and cloud
- [ ] 5: Experiments
- [ ] 6: Report and demo
