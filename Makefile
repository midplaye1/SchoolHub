.PHONY: up down run test build fmt integration
up:
	docker compose up --build

down:
	docker compose down

run:
	go run ./cmd/server

test:
	go test ./...

build:
	go build -o bin/libraryhub ./cmd/server

fmt:
	gofmt -w cmd internal migrations tests

integration:
	@test -n "$$TEST_DATABASE_URL" || (echo "Set TEST_DATABASE_URL to a local test PostgreSQL database"; exit 1)
	go test -race ./tests -v
