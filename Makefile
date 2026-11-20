
.PHONY: proto lint fmt vet test test-race build \
        ci up down logs run-app run-account \
        run-thirdparty run-worker

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

up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

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