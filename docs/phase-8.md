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

`terraform plan` creates the following:

| Category | Resources |
|----------|-----------|
| Networking | VPC (10.0.0.0/16), 2 public + 2 private subnets, NAT Gateway, IGW, route tables |
| Security | 6 Security Groups: ALB, API, Worker, RDS, Redis, MSK |
| Compute | ECS Cluster, 2 Task Definitions (API + Worker), 2 ECS Services |
| Database | Aurora PostgreSQL Serverless v2 (0.5–4 ACUs) |
| Cache | ElastiCache Redis (cache.t4g.micro) |
| Messaging | MSK Serverless (SASL/IAM auth) |
| Storage | S3 bucket (`beatstream-audio-dev-<account-id>`) |
| CDN | CloudFront distribution (OAC for S3, pass-through for ALB) |
| Load Balancer | ALB + Target Group (health check on :8080/health) |
| Container Registry | 2 ECR repos: `beatstream/api`, `beatstream/worker` |
| IAM | ECS execution role + task role (S3 + MSK permissions) |

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
