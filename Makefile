
.PHONY: proto lint fmt vet test test-race build \
        ci up down logs run-app run-account \
        run-thirdparty run-worker migrate-up migrate-status migrate-down

proto:
	buf lint
	buf generate

lint:
	buf lint

fmt:
	gofmt -w ./cmd ./internal

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

build:
	go build ./...

ci: lint vet test-race build

# Set DATABASE_URL in the shell. These targets do not load .env.
migrate-up migrate-status migrate-down: export GOOSE_DBSTRING = $(DATABASE_URL)
migrate-up migrate-status migrate-down: export GOOSE_DRIVER = postgres

migrate-up:
	go tool goose -env none -dir migrations up

migrate-status:
	go tool goose -env none -dir migrations status

# Destructive: use only with disposable development/test data.
migrate-down:
	go tool goose -env none -dir migrations down

up:
	docker compose up -d --wait --wait-timeout 120

down:
	docker compose down

logs:
	docker compose logs -f --tail=100

# Run individual services
run-app:
	go run ./cmd/application

run-account:
	go run ./cmd/account

run-thirdparty:
	go run ./cmd/thirdparty

run-worker:
	go run ./cmd/worker

# Run all services concurrently
run:
	$(MAKE) -j4 run-app run-account run-thirdparty run-worker
