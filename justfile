set dotenv-load

default: run

run:
    go run ./cmd/server

build:
    go build -o bin/server ./cmd/server

test:
    docker compose up -d postgres redis
    TEST_DATABASE_URL="postgres://notifications:secret@localhost:5432/notifications?sslmode=disable" \
    TEST_REDIS_URL="redis://localhost:6379" \
    go test ./... -v -count=1

lint:
    golangci-lint run ./...

up:
    docker compose up --build

down:
    docker compose down -v

seed:
    bash scripts/seed_webhook.sh

k6:
    k6 run k6/load_test.js
