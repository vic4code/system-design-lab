.PHONY: run build test migrate up down logs \
        k8s-cluster k8s-load k8s-deploy k8s-status k8s-delete

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

# ── Phase 3: Kubernetes (kind) ─────────────────────────────────────────────

# Create a local kind cluster
k8s-cluster:
	kind create cluster --name beatstream

# Build images and load them into the kind cluster (bypasses a registry)
k8s-load:
	docker build -t beatstream-api:latest --target api .
	docker build -t beatstream-worker:latest --target worker .
	kind load docker-image beatstream-api:latest --name beatstream
	kind load docker-image beatstream-worker:latest --name beatstream

# Apply all K8s manifests
k8s-deploy:
	kubectl apply -f k8s/
	kubectl -n beatstream rollout status deployment/api --timeout=120s

# Show pod + HPA status
k8s-status:
	kubectl -n beatstream get pods,svc,hpa

# Tear down the kind cluster
k8s-delete:
	kind delete cluster --name beatstream
