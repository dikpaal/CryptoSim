.PHONY: run stop build test clean logs

run:
	docker-compose up -d

stop:
	docker-compose down

build:
	docker-compose build

test:
	cd cmd && go test -v

clean:
	docker-compose down -v
	rm -f matching-engine

logs:
	docker-compose logs -f

engine-logs:
	docker-compose logs -f matching-engine
