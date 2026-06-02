# Phase 8 — AWS Cloud Deployment (Terraform)

## Architecture

```
Internet
  │
  ▼
CloudFront (PriceClass_200 — covers Asia + US + Europe)
  ├── /audio/*  ──────────────────► S3 (OAC, 24h cache)
  └── default   ──────────────────► ALB (no cache)
                                      │
                                      ▼
                           ECS Fargate — api (2 tasks, private subnets)
                              │              │               │
                           Aurora         Redis          MSK Serverless
                      (Serverless v2)  (t4g.micro)       (IAM auth)
                              │
                       ECS Fargate — worker (1 task)
```

**Region:** ap-northeast-1 (Tokyo) — closest AWS region to Taiwan (~5ms vs 200ms to us-east-1).

**AZs:** ap-northeast-1a + ap-northeast-1c. Two AZs is the minimum for HA; three would survive one AZ + one partial failure but costs 50% more.

---

## Prerequisites

| Tool | Version | Install |
|------|---------|---------|
| Terraform | >= 1.5 | `brew install hashicorp/tap/terraform` or download binary to `~/bin/` |
| AWS CLI | >= 2.x | `brew install awscli` |
| Docker | >= 24.x | Docker Desktop |

**AWS credentials must be configured before deployment:**

```bash
aws configure
# AWS Access Key ID: AKIA...
# AWS Secret Access Key: ****
# Default region name: ap-northeast-1
# Default output format: json

# Verify credentials are valid
aws sts get-caller-identity
```

---

## Resource inventory (55 resources)

`terraform plan` creates the following. Each resource maps to a real AWS service — here's what it does and why we chose it.

### Networking (`vpc.tf`)

| Resource | What it does | Why |
|----------|-------------|-----|
| **VPC** (10.0.0.0/16) | An isolated virtual network — all resources live inside it. Think of it as your own private data center in AWS. | Without a VPC, your database and cache would be exposed to the internet. |
| **Public subnets** (×2, one per AZ) | Subnets where resources CAN have public IPs. Only the ALB and NAT Gateway live here. | The ALB must be internet-facing to receive user traffic. |
| **Private subnets** (×2, one per AZ) | Subnets with NO public IPs. ECS tasks, RDS, Redis, and MSK live here. | Defense in depth — even if a container is compromised, it can't be reached from the internet directly. |
| **Internet Gateway** | Connects the VPC to the public internet (for public subnets). | Without it, nothing in the VPC can reach the outside world. |
| **NAT Gateway** (single AZ) | Lets private-subnet resources (ECS) make outbound requests (pull images, call AWS APIs) without being publicly accessible. | ECS tasks need to pull from ECR and write to S3, but should NOT be directly reachable. Trade-off: single NAT saves ~$32/mo vs one per AZ. |
| **Route tables** (public + private) | Routing rules: public → internet gateway, private → NAT gateway. | Ensures traffic flows through the correct path based on subnet type. |

**Key insight:** Public subnet ≠ "everything is public." Only resources you explicitly place there (ALB) get public IPs. The private subnet resources talk to the internet *through* NAT — outbound only.

### Security (`security_groups.tf`)

Security groups are stateful firewalls attached to each resource. Traffic is denied by default — you explicitly allow what's needed.

| Resource | Inbound allows | Why this restriction |
|----------|---------------|---------------------|
| **SG: ALB** | Port 80/443 from anywhere (0.0.0.0/0) | The load balancer is the only public entry point. |
| **SG: API** | Port 8080 from ALB only | API containers are NOT directly reachable — all traffic must go through ALB. |
| **SG: Worker** | No inbound (egress only) | Workers pull from Kafka — nothing needs to call into them. |
| **SG: RDS** | Port 5432 from API + Worker | Only application containers can connect to the database. |
| **SG: Redis** | Port 6379 from API only | Only the API uses cache — the worker doesn't need it. |
| **SG: MSK** | Port 9098 from API + Worker | Kafka port with IAM auth. Both API (producer) and Worker (consumer) need access. |

**Key insight:** This creates a layered chain: `Internet → ALB → API → {RDS, Redis, MSK} ← Worker`. An attacker who somehow reaches the API container still can't connect to Redis from there unless they're on port 6379 from the API security group.

### Compute (`ecs.tf`)

| Resource | What it does | Why ECS Fargate |
|----------|-------------|-----------------|
| **ECS Cluster** | Logical grouping of services. No servers to manage. | Fargate = serverless containers. AWS handles the underlying EC2 instances. You just define CPU/memory. |
| **Task Definition: API** (0.5 vCPU, 1GB) | Blueprint for the API container — image, env vars, secrets, health check, ports. | Declares what the container needs. Secrets (DB password, Kafka brokers) come from SSM at startup — never baked into the image. |
| **Task Definition: Worker** (0.25 vCPU, 512MB) | Blueprint for the worker container — same structure, lower resources. | Worker just consumes Kafka and writes to DB/S3 — doesn't serve HTTP traffic, needs less CPU. |
| **ECS Service: API** (desired_count=2) | Keeps 2 API tasks running across 2 AZs. Registers with ALB. Auto-restarts on crash. | 2 tasks = HA. If one AZ dies, the other keeps serving. Circuit breaker auto-rolls back bad deploys. |
| **ECS Service: Worker** (desired_count=1) | Keeps 1 worker task running. | Only one consumer needed at this scale. Kafka handles redelivery if the task crashes. |
| **CloudWatch Log Groups** (7-day retention) | Collects stdout/stderr from containers. | You need logs to debug. 7 days keeps costs low for a lab. |

**Why Fargate over EC2:** No AMI updates, no instance patching, no capacity planning. You pay per second of vCPU+memory used. For a lab with variable traffic, this is cheaper and simpler than managing EC2 instances.

### Database (`rds.tf`)

| Resource | What it does | Why Aurora Serverless v2 |
|----------|-------------|--------------------------|
| **Aurora PostgreSQL Cluster** (Serverless v2) | Managed PostgreSQL that auto-scales compute between 0.5–4 ACUs based on load. | At idle (most of the time for a lab), it runs at 0.5 ACU (~$50/mo). Under load it scales up in seconds — no manual intervention. Regular RDS would require choosing and paying for a fixed instance size. |
| **Aurora Instance** (db.serverless) | The actual compute node inside the cluster. | Aurora separates compute from storage. The instance scales; the storage is independent. |
| **DB Subnet Group** | Tells RDS which subnets to deploy into (private only). | Forces the database into private subnets — no public access possible. |
| **SSM Parameter** (/beatstream/db_password) | Stores the DB password as a SecureString in AWS Systems Manager. | ECS reads this at container startup. The password never appears in environment variables or Docker images — only in SSM (encrypted at rest with KMS). |

**Why not plain RDS PostgreSQL:** For a lab that's idle 90% of the time, Serverless v2 avoids paying for a `db.t3.medium` ($60/mo) that sits at 2% CPU utilization. The trade-off is a ~1s cold-start latency spike when scaling from minimum.

### Cache (`elasticache.tf`)

| Resource | What it does | Why ElastiCache Redis |
|----------|-------------|----------------------|
| **ElastiCache Redis** (cache.t4g.micro) | In-memory key-value store — caches track metadata and search results. Same role as local Redis in Phase 1. | Managed Redis = AWS handles patching, failover (if replicated), backups. `t4g.micro` is the cheapest Graviton instance (~$12/mo). |
| **ElastiCache Subnet Group** | Places Redis in private subnets. | Same reason as RDS — no public access. |

**Why single-node for a lab:** A replication group (primary + replica) costs 2x but adds automatic failover. For a lab, the API's cache-aside pattern means a Redis crash just causes cache misses — the app still works (fail-open design from Phase 1).

### Messaging (`msk.tf`)

| Resource | What it does | Why MSK Serverless |
|----------|-------------|-------------------|
| **MSK Serverless Cluster** | Managed Apache Kafka — handles play events and upload processing queues. Replaces Redpanda from local dev. | Serverless = no brokers to manage, auto-scales, pay per data. Same Kafka protocol as local Redpanda — the Go code works unchanged. |
| **SSM Parameter** (/beatstream/kafka_brokers) | Stores the MSK bootstrap broker endpoints. | MSK doesn't expose brokers as a Terraform output. A `null_resource` runs `aws kafka get-bootstrap-brokers` after creation and writes the result to SSM. |
| **null_resource** (fetch_kafka_brokers) | A one-time script that fetches MSK broker addresses and stores them in SSM. | Workaround for Terraform's limitation — MSK Serverless broker addresses aren't available as attributes. |

**Why MSK over self-managed Kafka on ECS:** Operating Kafka (ZooKeeper/KRaft, replication, partition rebalancing) is a full-time job. MSK Serverless handles all of it. Trade-off: ~$8/mo base cost vs $0 for Redpanda in Docker Compose, but zero operational burden.

### Storage (`s3.tf`)

| Resource | What it does | Why S3 |
|----------|-------------|--------|
| **S3 Bucket** (beatstream-audio-dev-{account_id}) | Stores uploaded audio files. Replaces MinIO from local dev. | S3 is the standard object store — 99.999999999% durability, effectively infinite capacity, ~$0.023/GB/mo. Account ID in the name guarantees global uniqueness. |
| **Public Access Block** | Blocks ALL public access at the bucket level — even if someone accidentally adds a public policy. | Belt-and-suspenders security. Audio is served through CloudFront OAC, never directly from S3. |
| **Versioning** | Keeps old versions of objects when overwritten. | Protects against accidental deletion or corruption. Can always roll back to a previous version. |
| **Lifecycle Rules** | (1) Aborts stuck multipart uploads after 7 days. (2) Moves tracks not accessed in 90 days to S3 Infrequent Access (40% cheaper). | Multipart uploads that never complete waste storage silently. IA tiering mirrors real streaming services where most tracks are rarely played (long tail). |

### CDN (`cloudfront.tf`)

| Resource | What it does | Why CloudFront |
|----------|-------------|----------------|
| **CloudFront Distribution** | Global CDN with 400+ edge locations. Routes `/audio/*` to S3, everything else to ALB. | Users in Taiwan hit a Tokyo PoP (~3ms) instead of going through the full ALB path. For users in other regions, latency drops from 180ms to <10ms for cached audio. |
| **Origin Access Control (OAC)** | Signs requests from CloudFront to S3 with SigV4. | The S3 bucket is fully private (no public access). OAC is the only way CloudFront can read from it — and only THIS distribution is allowed (condition on ARN). |
| **Cache Behaviors** | `/audio/*` → aggressive 24h cache (audio is immutable). Default → no cache (API is dynamic). | Audio files never change after upload (key includes track ID). API responses change per request — caching them would serve stale data. |
| **S3 Bucket Policy** | Grants `s3:GetObject` to CloudFront's service principal, scoped to this distribution's ARN. | Without this policy, CloudFront gets 403 from S3 even with OAC configured. The ARN condition prevents other distributions from reading your bucket. |

**Why PriceClass_200:** Covers US, Europe, and Asia (including Taiwan and Japan). PriceClass_All adds South America and Africa — not needed here, costs more per request.

### Load Balancer (`alb.tf`)

| Resource | What it does | Why ALB |
|----------|-------------|---------|
| **Application Load Balancer** | Distributes HTTP traffic across ECS API tasks. Health-checks each task every 30s. | ALB operates at Layer 7 (HTTP) — it can route by path, inspect headers, and terminate TLS. NLB (Layer 4) is cheaper but can't do path-based routing or HTTP health checks. |
| **Target Group** (ip type, :8080/health) | Tracks healthy ECS tasks. Fargate uses IP targeting because tasks don't have fixed instance IPs. | Health check on `/health` means ALB only sends traffic to tasks that are ready. If a task is starting up or crashing, it gets no traffic. |
| **HTTP Listener** (port 80) | Forwards all incoming HTTP requests to the target group. | For a lab, HTTP is fine. Production would add an HTTPS listener with ACM cert and redirect HTTP→HTTPS. |

### Container Registry (`ecr.tf`)

| Resource | What it does | Why ECR |
|----------|-------------|---------|
| **ECR Repos** (beatstream/api, beatstream/worker) | Private Docker registries for your images. ECS pulls from here at deploy time. | ECR is the native registry for ECS — no extra auth setup. Images are scanned for vulnerabilities on push. |
| **Lifecycle Policy** | Deletes untagged images after 10 builds. | Without this, every `docker push` leaves old untagged images that accumulate storage costs forever. |

### IAM (`iam.tf`)

| Resource | What it does | Why two roles |
|----------|-------------|---------------|
| **ECS Execution Role** | Used by the ECS **agent** (not your code) to: pull images from ECR, fetch secrets from SSM, write logs to CloudWatch. | Separation of concerns. Your container code never sees ECR credentials — the ECS platform handles image pulling. |
| **ECS Task Role** | Used by your **application code** to: read/write S3, connect to MSK with IAM auth. | This is what `aws.NewConfig()` in your Go code picks up automatically. No access keys needed — temporary credentials are rotated by ECS every hour. |

**Key insight:** Two roles because the ECS agent and your app need different permissions. The execution role is broad (ECR, SSM, CloudWatch) but only used during startup. The task role is specific (S3 + MSK) and used at runtime. Least-privilege principle.

---

### How it all connects (request flow)

```
User in Taiwan
  │
  ▼
CloudFront (Tokyo PoP)
  │
  ├── GET /audio/track-123.mp3
  │     → S3 (OAC signed request) → cached at edge for 24h
  │
  └── POST /v1/tracks
        → ALB (public subnet)
           → ECS API task (private subnet)
              ├── writes metadata → Aurora (private subnet, port 5432)
              ├── uploads file → S3 (via task role, no keys)
              ├── publishes event → MSK (IAM auth, port 9098)
              └── invalidates cache → Redis (port 6379)
                       │
                       ▼
              ECS Worker task (private subnet)
              ├── consumes from MSK
              ├── processes upload
              └── updates DB status → Aurora
```

---

## Step-by-step deployment

### Step 0: Configure variables

```bash
cd beatstream/infra/terraform

# terraform.tfvars is gitignored — safe to store secrets locally
cp terraform.tfvars.example terraform.tfvars
```

Edit `terraform.tfvars`:
```hcl
aws_region  = "ap-northeast-1"
environment = "dev"
db_password = "YourStrongPassword16+"   # min 16 chars, used as Aurora master password
```

### Step 1: Initialize Terraform

```bash
make infra-init
# Downloads AWS provider (~v5.100) and null provider (~v3.3)
# Creates .terraform/ directory and .terraform.lock.hcl
```

### Step 2: Preview the plan

```bash
make infra-plan
# Shows all 55 resources that will be created
# No cost incurred — this is read-only
```

### Step 3: Apply (create the stack)

```bash
make infra-apply
# Type "yes" when prompted
# Takes ~15 minutes (MSK Serverless is the bottleneck)
# After completion: outputs ALB DNS, CloudFront domain, ECR URLs
```

### Step 4: Build and push container images to ECR

```bash
make infra-push
# 1. Authenticates Docker to ECR
# 2. Builds api and worker images (multi-stage Dockerfile)
# 3. Tags and pushes both images to ECR
```

### Step 5: Deploy to ECS

```bash
make infra-deploy
# Forces ECS services to pull the latest images from ECR
# ECS performs a rolling deployment (zero-downtime)
```

### Step 6: Verify the stack

```bash
# Get the ALB DNS
cd infra/terraform && terraform output alb_dns

# Health check
curl http://$(terraform output -raw alb_dns)/healthz
# → {"status":"ok"}

# Readiness check (confirms DB + Redis + Kafka connectivity)
curl http://$(terraform output -raw alb_dns)/ready
# → {"status":"ready"}
```

### Step 7: Tear down (stop billing)

```bash
# Must empty the S3 bucket first (Terraform won't delete non-empty buckets)
aws s3 rm s3://$(cd infra/terraform && terraform output -raw audio_bucket) --recursive

make infra-destroy
# Type "yes" when prompted
```

---

## Cost estimate (10K DAU, ap-northeast-1)

| Service | Size | ~Monthly |
|---|---|---|
| ECS Fargate API (2 tasks × 0.5 vCPU, 1GB) | 730 hr/mo | $18 |
| ECS Fargate Worker (1 task × 0.25 vCPU) | 730 hr/mo | $4 |
| Aurora Serverless v2 (avg 1 ACU) | | $50 |
| ElastiCache t4g.micro | | $12 |
| MSK Serverless (low volume) | ~$0.011/hr base | $8 |
| ALB | | $20 |
| NAT Gateway | | $35 |
| CloudFront (100GB/mo) | | $9 |
| S3 (50GB audio) | | $1 |
| **Total** | | **~$157/mo** |

> **Cost tip:** NAT Gateway is the surprise expense (~$0.045/GB processed + $32/mo fixed). For production: add VPC endpoints for S3, ECR, and MSK to route traffic over the AWS backbone instead of through NAT (~40% savings).

---

## Files

```
beatstream/infra/terraform/
├── main.tf             provider config, data sources
├── variables.tf        aws_region, environment, db_password, image tags
├── outputs.tf          ALB DNS, CloudFront domain, ECR URLs, MSK ARN
├── vpc.tf              VPC 10.0.0.0/16, 2 public + 2 private subnets, NAT
├── security_groups.tf  ALB → API → RDS/Redis/MSK SG chain
├── iam.tf              ECS execution role + task role (S3 + MSK permissions)
├── ecr.tf              Two ECR repos: beatstream/api, beatstream/worker
├── alb.tf              ALB + target group (:8080/health) + HTTP listener
├── ecs.tf              Cluster, task defs (API + worker), services + circuit breaker
├── rds.tf              Aurora PostgreSQL Serverless v2 (0.5–4 ACUs)
├── elasticache.tf      Redis cache.t4g.micro
├── s3.tf               Audio bucket + public-access-block + lifecycle rules
├── cloudfront.tf       CDN: OAC for S3, CachingDisabled for API
├── msk.tf              MSK Serverless + null_resource to fetch bootstrap brokers
└── terraform.tfvars.example
```

**Go changes (from Phase 0–7 baseline):**
```
internal/storage/s3.go          MinIO SDK → AWS SDK v2 (S3_ENDPOINT for local dev fallback)
internal/queue/kafka.go         + NewProducerIAM / NewConsumerIAM for MSK SASL/IAM
internal/worker/upload.go       + kafkaAuth, awsRegion params
internal/worker/analytics.go    + kafkaAuth, awsRegion params
cmd/api/main.go                 S3_BUCKET/S3_ENDPOINT env vars, KAFKA_AUTH=iam support
cmd/worker/main.go              KAFKA_AUTH=iam for MSK
docker-compose.yml              MINIO_* → S3_* env vars (backward-compatible)
```

---

## Key design decisions

### ECS Fargate vs EKS

| | ECS Fargate | EKS |
|---|---|---|
| Control plane | Fully managed (no cost) | $0.10/hr per cluster = ~$73/mo |
| Ops overhead | Low — no node management | High — node groups, upgrades |
| K8s ecosystem | No (use AWS native) | Full kubectl/Helm/CRD support |
| Portability | AWS-only | Can migrate to any K8s |
| Auto-scaling | Service Auto Scaling + ALB | HPA + KEDA + Cluster Autoscaler |
| Best for | Single cloud, smaller teams | Multi-cloud, large platform teams |

**Why ECS for this phase:** We already learned K8s in Phase 3. ECS tests whether you can navigate the AWS-native approach and explain the trade-offs in an interview.

### Aurora Serverless v2 vs RDS PostgreSQL

| | Aurora Serverless v2 | RDS PostgreSQL |
|---|---|---|
| Scaling | Auto (0.5–128 ACUs, ~seconds) | Manual (instance resize = downtime) |
| Cost at idle | ~$0.10/hr (0.5 ACU) | ~$0.017/hr (t3.micro) |
| Cost at peak | Higher per-unit than provisioned | Predictable |
| Cold start | ~1s lag when scaling from minimum | None |
| HA | Multi-AZ by default | Multi-AZ costs 2x |

**When to choose Aurora:** Spiky or unpredictable workloads where you want automatic scaling without pre-provisioning.

### CloudFront OAC vs direct S3

Without CloudFront:
- User in Taiwan → S3 ap-northeast-1: ~5ms (same region)
- User in Europe → S3 ap-northeast-1: ~180ms (cross-region)

With CloudFront (PriceClass_200):
- User in Europe → Frankfurt PoP: ~2ms after cache fill
- Cache hit rate for popular tracks: ~95%+ (immutable audio files)

**Bandwidth savings:** CloudFront → origin is $0.06/GB vs S3 → internet $0.09/GB. For 1TB/month, CDN saves ~$30 AND is faster.

### MSK Serverless IAM auth

MSK Serverless only supports SASL/IAM — no plaintext, no SCRAM. The franz-go SASL AWS package handles the OAUTHBEARER token flow using SigV4 signing. ECS task role credentials are injected automatically — no static keys in the container.

---

## Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| `terraform plan` fails with "ExpiredToken" | AWS credentials expired | `aws configure` or `aws sso login` |
| `terraform plan` fails with "InvalidClientTokenId" | Wrong access key | Re-run `aws configure`, verify key starts with `AKIA` |
| `infra-apply` times out | MSK Serverless creation (~12 min) | Wait and retry; check AWS Console for status |
| ECS tasks stuck in PENDING | No available capacity or wrong subnets | Check security groups allow outbound to ECR/S3 |
| ALB health check failing | App not listening on :8080 or /health path wrong | Check ECS task logs: `aws ecs describe-tasks` |
| `infra-destroy` fails on S3 | Bucket not empty | `aws s3 rm s3://<bucket> --recursive` first |

---

## Experiments

### Simulate AZ failure

```bash
# Run load test in background
k6 run k6/load.js &

# Force tasks out of one AZ
aws ecs update-service \
  --cluster beatstream \
  --service beatstream-api \
  --placement-constraints '[{"type":"memberOf","expression":"attribute:ecs.availability-zone != ap-northeast-1a"}]' \
  --region ap-northeast-1
```

**Expected:** ALB detects unhealthy tasks (3 × 30s health checks ≈ 90s), drains connections, routes to surviving AZ. k6 shows p99 latency spike but zero HTTP errors.

### Measure real-world latency from Taiwan

```bash
# Via CloudFront
for i in $(seq 10); do
  curl -o /dev/null -s -w "%{time_total}\n" \
    https://$(cd infra/terraform && terraform output -raw cloudfront_domain)/healthz
done | awk '{s+=$1} END {print "avg:", s/NR, "s"}'

# Direct ALB (bypass CDN)
curl -o /dev/null -s -w "time: %{time_total}s\n" \
  http://$(cd infra/terraform && terraform output -raw alb_dns)/healthz
```

Typical results:
- API (ALB, same region): ~5–15ms
- Audio (CloudFront, cache hit): ~3–8ms
- Audio (CloudFront, cache miss → S3): ~25–40ms

---

## Interview questions

> *"Walk me through your AWS architecture. Why ECS over EKS?"*

No control plane cost, simpler ops, AWS-native integrations. Sufficient for a team that isn't running K8s at scale. We proved K8s competency in Phase 3 — this phase shows we can pick the right tool.

> *"What happens when an AZ goes down?"*

ALB stops routing to failed tasks (~90s detection). ECS launches replacements in the surviving AZ. Aurora promotes a replica (<30s). Redis is single-node — cache misses until replaced. Production fix: ElastiCache Replication Group with a replica per AZ.

> *"Why is NAT Gateway expensive?"*

$0.045/GB processed + $0.045/hr fixed. All ECS outbound (ECR pulls, S3 writes, MSK) goes through NAT. Fix: VPC endpoints for S3 (gateway, free), ECR and MSK (interface, fixed hourly).

> *"A user reports stale audio after you updated a track. How do you fix it?"*

CloudFront caches by URL. If the object key didn't change, create an invalidation (`aws cloudfront create-invalidation`) or — better — use content-addressed keys (hash in the filename) so updates get a new URL automatically.

---

**[← Phase 7 — RBAC + GDPR](phase-7.md) · [Back to Beatstream README →](../beatstream/README.md)**
