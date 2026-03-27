.PHONY: run stop build test clean logs restart db-migrate db-logs persistence-logs

run:
	docker-compose up -d

stop:
	docker-compose down

build:
	docker-compose build

restart: stop build run

test:
	cd cmd && go test -v

clean:
	docker-compose down -v
	rm -f matching-engine

logs:
	docker-compose logs -f

engine-logs:
	docker-compose logs -f matching-engine

persistence-logs:
	docker-compose logs -f persistence

db-logs:
	docker-compose logs -f timescaledb

db-shell:
	docker-compose exec timescaledb psql -U postgres -d cryptosim

db-migrate:
	@echo "Migrations auto-run on DB startup via docker-entrypoint-initdb.d"
