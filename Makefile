COMPOSE := docker compose

.PHONY: build up down restart logs ps test vet fmt run tidy ci

## ---- Docker ----

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

## ---- Go ----

## Download module deps
tidy:
	go mod tidy

## Format packages
fmt:
	go fmt ./...

## Run go vet
vet:
	go vet ./...

## Run unit tests
test:
	go test ./... -count=1

## Build and run the server locally (requires .env / Redis as needed)
run:
	go run ./cmd/server

## CI-style checks
ci: fmt vet test
	go build -o bin/codereviewagent ./cmd/server
