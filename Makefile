.PHONY: run stop build test clean logs restart db-migrate db-logs persistence-logs loadtest loadtest-quick bench bench-db profile monitor

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

# Load Testing
loadtest-quick:
	@echo "Running quick load test (30s @ 1000 orders/s)..."
	go run cmd/loadtest/main.go --target-orders 1000 --test-duration 30s --num-traders 5

loadtest:
	@echo "Running standard load test (60s @ 2500 orders/s)..."
	go run cmd/loadtest/main.go --target-orders 2500 --test-duration 60s --num-traders 6

bench:
	@echo "Running full benchmark suite..."
	./scripts/bench.sh

bench-db:
	@echo "Running DB write capacity benchmark..."
	./scripts/bench-db.sh

profile:
	@echo "Profiling matching engine (30s)..."
	./scripts/profile.sh

monitor:
	@echo "Starting real-time monitor..."
	./scripts/monitor.sh
