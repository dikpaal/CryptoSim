.PHONY: run stop build test clean

run:
	docker-compose up -d

stop:
	docker-compose down

build:
	docker-compose build

test:
	cd services/matching-engine && go test -v ./...

clean:
	docker-compose down -v
	rm -rf services/matching-engine/matching-engine

logs:
	docker-compose logs -f

engine-logs:
	docker-compose logs -f matching-engine
