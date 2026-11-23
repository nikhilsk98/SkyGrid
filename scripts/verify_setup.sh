#!/usr/bin/env bash
# Phase 0 checkpoint: confirms every database is up and configured correctly.
set -uo pipefail
cd "$(dirname "$0")/.."

PGUSER="${POSTGRES_USER:-skygrid}"
SHARDS=(0 1 2 3)
fail=0

ok()  { echo "  OK    $1"; }
bad() { echo "  FAIL  $1"; fail=1; }
q()   { docker compose exec -T "$1" psql -U "$PGUSER" -d skygrid -tAc "$2" 2>/dev/null; }

echo "Checking containers..."
for svc in metadata "${SHARDS[@]/#/shard-}"; do
  if docker compose exec -T "$svc" pg_isready -U "$PGUSER" -d skygrid >/dev/null 2>&1; then
    ok "$svc is ready"
  else
    bad "$svc is not ready"
  fi
done

echo "Checking shards..."
for i in "${SHARDS[@]}"; do
  svc="shard-$i"
  mpt=$(q "$svc" "SHOW max_prepared_transactions")
  if [[ "${mpt:-0}" -gt 0 ]]; then ok "$svc max_prepared_transactions=$mpt"; else bad "$svc 2PC disabled (max_prepared_transactions=${mpt:-?})"; fi

  tbl=$(q "$svc" "SELECT to_regclass('public.reservations') IS NOT NULL")
  if [[ "$tbl" == "t" ]]; then ok "$svc reservations table exists"; else bad "$svc reservations table missing"; fi
done

echo "Checking metadata..."
regions=$(q metadata "SELECT count(*) FROM routing_table")
if [[ "$regions" == "20" ]]; then ok "routing_table has 20 regions"; else bad "routing_table has ${regions:-?} regions (expected 20)"; fi

version=$(q metadata "SELECT version || ' (' || placement || ')' FROM routing_version")
if [[ -n "$version" ]]; then ok "routing_version = $version"; else bad "routing_version missing"; fi

echo "2PC smoke test on shard-0..."
if docker compose exec -T shard-0 psql -U "$PGUSER" -d skygrid -v ON_ERROR_STOP=1 -q >/dev/null 2>&1 <<'SQL'
BEGIN;
INSERT INTO reservations (cell_id, time_slot, region_id, drone_id, txn_id)
VALUES (-1, -1, 0, -1, 'verify');
PREPARE TRANSACTION 'skygrid:verify';
ROLLBACK PREPARED 'skygrid:verify';
SQL
then
  ok "PREPARE / ROLLBACK PREPARED works"
else
  bad "2PC smoke test failed"
fi

echo
if [[ $fail -eq 0 ]]; then
  echo "Phase 0 checkpoint PASSED"
else
  echo "Phase 0 checkpoint FAILED"
  exit 1
fi
