COMPOSE := docker compose

.PHONY: build up down restart logs ps

## Build Docker images
build:
	$(COMPOSE) build

## Start containers in background
up:
	$(COMPOSE) up -d

## Stop and remove containers
down:
	$(COMPOSE) down

## Restart containers (down then up)
restart: down up

## Follow container logs
logs:
	$(COMPOSE) logs -f

## Show running containers
ps:
	$(COMPOSE) ps
