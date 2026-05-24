.PHONY: run build test migrate up down logs

# Start all services
up:
	docker compose up -d

# Stop all services
down:
	docker compose down

# View logs
logs:
	docker compose logs -f api-1

# Build API and worker binaries
build:
	go build -o beatstream-api ./cmd/api
	go build -o beatstream-worker ./cmd/worker

# Run API locally (requires postgres, minio, redis, redpanda running)
run:
	go run ./cmd/api

# Run worker locally
run-worker:
	go run ./cmd/worker

# Run tests
test:
	go test ./... -v

# Run database migrations
migrate:
	docker compose exec postgres psql -U user -d beatstream \
		-f /migrations/001_create_tracks.sql \
		-f /migrations/002_create_playlists.sql \
		-f /migrations/003_create_play_events.sql

# Run k6 load test (requires TRACK_ID env var)
load-test:
	k6 run k6/load.js

# Seed sample data
seed:
	docker compose exec postgres psql -U user -d beatstream -f /migrations/seed.sql

# Format Go code
fmt:
	gofmt -w .

# Lint Go code
lint:
	golangci-lint run ./...
