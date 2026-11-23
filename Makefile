.PHONY: up down reset verify logs psql-meta psql-shard

up:
	docker compose up -d --wait

down:
	docker compose down

# Wipes all data and re-runs migrations.
reset:
	docker compose down -v
	docker compose up -d --wait

verify:
	./scripts/verify_setup.sh

logs:
	docker compose logs -f

psql-meta:
	docker compose exec metadata psql -U skygrid -d skygrid

# Usage: make psql-shard N=2
psql-shard:
	docker compose exec shard-$(N) psql -U skygrid -d skygrid
