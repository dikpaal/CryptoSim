.PHONY: up down reset test build

build:
	docker compose build

up:
	docker compose up -d

down:
	docker compose down -v

reset: down up

test: down
	docker compose --profile testing up --build -d
