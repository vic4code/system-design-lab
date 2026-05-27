.PHONY: run build test migrate up down logs \
        k8s-cluster k8s-load k8s-deploy k8s-status k8s-delete \
        infra-init infra-plan infra-apply infra-push infra-deploy infra-destroy \
        certs certs-trust \
        web-install web-dev

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
# Namespace must be applied first and given time to propagate before other
# resources are created, otherwise the API server returns NotFound errors.
k8s-deploy:
	kubectl apply -f k8s/namespace.yaml
	kubectl wait --for=jsonpath='{.status.phase}'=Active namespace/beatstream --timeout=15s
	kubectl apply -f k8s/
	kubectl -n beatstream rollout status deployment/api --timeout=120s

# Show pod + HPA status
k8s-status:
	kubectl -n beatstream get pods,svc,hpa

# Tear down the kind cluster
k8s-delete:
	kind delete cluster --name beatstream

# ── Phase 4: AWS Cloud Deployment ─────────────────────────────────────────────

TERRAFORM_DIR = infra/terraform

# Step 1: initialise Terraform and install providers
infra-init:
	cd $(TERRAFORM_DIR) && terraform init

# Step 2: preview what will be created (requires terraform.tfvars)
infra-plan:
	cd $(TERRAFORM_DIR) && terraform plan

# Step 3: create the full AWS stack (~15 min, MSK Serverless is the bottleneck)
infra-apply:
	cd $(TERRAFORM_DIR) && terraform apply

# Step 4: build images, push to ECR, force new ECS deployment
# Usage: AWS_ACCOUNT_ID=123456789012 make infra-push
infra-push:
	$(eval REGION    := $(shell cd $(TERRAFORM_DIR) && terraform output -raw alb_dns | xargs -I{} echo ap-northeast-1))
	$(eval API_URL   := $(shell cd $(TERRAFORM_DIR) && terraform output -raw ecr_api_url))
	$(eval WORKER_URL := $(shell cd $(TERRAFORM_DIR) && terraform output -raw ecr_worker_url))
	aws ecr get-login-password --region ap-northeast-1 | docker login --username AWS --password-stdin $(API_URL)
	docker build -t beatstream-api --target api .
	docker build -t beatstream-worker --target worker .
	docker tag beatstream-api:latest $(API_URL):latest
	docker tag beatstream-worker:latest $(WORKER_URL):latest
	docker push $(API_URL):latest
	docker push $(WORKER_URL):latest

# Step 5: force ECS to pull the latest images
infra-deploy:
	aws ecs update-service --cluster beatstream --service beatstream-api   --force-new-deployment --region ap-northeast-1
	aws ecs update-service --cluster beatstream --service beatstream-worker --force-new-deployment --region ap-northeast-1

# Tear down everything (will NOT delete the S3 bucket if it has objects — drain first)
infra-destroy:
	cd $(TERRAFORM_DIR) && terraform destroy

# ── Phase 6: Security ─────────────────────────────────────────────────────────

# Generate local TLS certs using mkcert (run once; requires: brew install mkcert)
# The .key file is gitignored; only the .crt is committed.
certs:
	@which mkcert > /dev/null || (echo "Install mkcert: brew install mkcert" && exit 1)
	mkdir -p nginx/certs
	cd nginx/certs && mkcert -cert-file localhost.crt -key-file localhost.key localhost 127.0.0.1

# Trust the mkcert CA in the system keychain (run once, requires sudo)
certs-trust:
	mkcert -install

# ── Frontend ───────────────────────────────────────────────────────────────────

# Install frontend dependencies
web-install:
	cd web && npm install

# Start Next.js dev server (requires docker compose up for the backend)
web-dev:
	cd web && npm run dev
