# Phase 8 — AWS Cloud Deployment

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

## Files added

```
beatstream/
├── infra/terraform/
│   ├── main.tf             provider, backend (S3 optional)
│   ├── variables.tf        aws_region, environment, db_password
│   ├── outputs.tf          ALB DNS, CloudFront domain, ECR URLs, MSK ARN
│   ├── vpc.tf              VPC 10.0.0.0/16, 2 public + 2 private subnets, NAT
│   ├── security_groups.tf  ALB, API, worker, RDS, Redis, MSK SGs
│   ├── iam.tf              ECS execution role + task role (S3 + MSK permissions)
│   ├── ecr.tf              Two ECR repos: beatstream/api, beatstream/worker
│   ├── alb.tf              ALB + target group (:8080/health) + HTTP listener
│   ├── ecs.tf              Cluster, task defs (API + worker), services + circuit breaker
│   ├── rds.tf              Aurora PostgreSQL Serverless v2 (0.5–4 ACUs)
│   ├── elasticache.tf      Redis cache.t4g.micro
│   ├── s3.tf               Audio bucket + public-access-block + lifecycle
│   ├── cloudfront.tf       CDN: OAC for S3, CachingDisabled for API
│   ├── msk.tf              MSK Serverless + null_resource to fetch bootstrap brokers
│   └── terraform.tfvars.example
```

**Go changes:**
```
internal/storage/s3.go     rewrote: MinIO SDK → AWS SDK v2 (auto-detects local vs cloud)
internal/queue/kafka.go    added NewProducerIAM / NewConsumerIAM for MSK IAM SASL
internal/worker/upload.go  added kafkaAuth, awsRegion params
internal/worker/analytics.go added kafkaAuth, awsRegion params
cmd/api/main.go            new S3 env vars (S3_ENDPOINT/S3_BUCKET), KAFKA_AUTH support
cmd/worker/main.go         KAFKA_AUTH=iam for MSK
docker-compose.yml         MINIO_* → S3_* env vars
```

---

## Deployment sequence

```bash
# 1. Copy and fill in the vars file
cp infra/terraform/terraform.tfvars.example infra/terraform/terraform.tfvars
# edit terraform.tfvars: set db_password to something strong

# 2. Initialise Terraform
make infra-init

# 3. Preview (check costs before applying)
make infra-plan

# 4. Create the stack (~15 min — MSK Serverless is the bottleneck)
make infra-apply
# After apply: null_resource auto-fetches MSK bootstrap brokers → SSM

# 5. Build images and push to ECR
make infra-push

# 6. Force ECS to pull new images
make infra-deploy

# 7. Check it works
curl http://$(cd infra/terraform && terraform output -raw alb_dns)/healthz
# → {"status":"ok"}
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

**Why ECS for Phase 4:** We already learned K8s in Phase 3. ECS tests whether you can navigate the AWS-native approach and explain the trade-offs in an interview.

### Aurora Serverless v2 vs RDS PostgreSQL

| | Aurora Serverless v2 | RDS PostgreSQL |
|---|---|---|
| Scaling | Auto (0.5–128 ACUs, ~seconds) | Manual (instance resize = downtime) |
| Cost at idle | ~$0.10/hr (0.5 ACU) | ~$0.017/hr (t3.micro) |
| Cost at peak | Higher per-unit than provisioned | Predictable |
| Cold start | ~1s lag when scaling from minimum | None |
| HA | Multi-AZ by default | Multi-AZ costs 2x |

**When to choose Aurora:** Spiky or unpredictable workloads where you want automatic scaling without pre-provisioning. Good default for a dev environment.

### CloudFront OAC vs direct S3

Without CloudFront:
- User in Taiwan → S3 ap-northeast-1: ~5ms (same region, fast)
- User in Europe → S3 ap-northeast-1: ~180ms (cross-region)

With CloudFront (PriceClass_200 includes Europe, Asia):
- User in Europe → Frankfurt PoP: ~2ms after cache fill
- Cache hit rate for popular tracks: ~95%+ (immutable files)

The real win: **bandwidth cost**. CloudFront → origin (S3) data transfer is $0.06/GB. S3 → internet is $0.09/GB. For 1TB/month, CloudFront saves ~$30 AND is faster. For audio streaming at scale the math heavily favours CDN.

### MSK Serverless IAM auth

MSK Serverless only supports SASL/IAM — no plaintext, no SCRAM. The franz-go SASL AWS package handles the OAUTHBEARER token flow using SigV4 signing.

Key insight: the ECS task role credentials are injected via the [ECS credential provider](https://docs.aws.amazon.com/sdkref/latest/guide/feature-container-credentials.html). The container never handles actual AWS keys — the ECS agent rotates temporary credentials automatically.

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

NAT Gateway is surprisingly expensive (~$0.045/GB data processed + $32/mo fixed). For a production system: use S3/ECR VPC endpoints to avoid routing S3 and ECR traffic through NAT (~40% of NAT cost at scale).

---

## Simulating AZ failure

```bash
# Find tasks in AZ-a
aws ecs list-tasks --cluster beatstream --query taskArns

# Stop all tasks (ECS will restart them; ALB drains connections first)
# In practice: terminate EC2 instances in AZ-a, or use AWS Fault Injection Simulator

# Watch ALB route around the failure — should see 0 errors
k6 run k6/load.js &
aws ecs update-service --cluster beatstream --service beatstream-api \
  --placement-constraints '[{"type":"memberOf","expression":"attribute:ecs.availability-zone != ap-northeast-1a"}]'
```

Expected: ALB detects unhealthy tasks (3 failed health checks × 30s = ~90s), drains connections, routes to healthy tasks. p99 latency spikes during failover but error rate stays 0% if connection draining works.

---

## Measuring real-world latency

```bash
# From Taiwan to ap-northeast-1 CloudFront
for i in $(seq 10); do
  curl -o /dev/null -s -w "%{time_total}\n" \
    https://$(cd infra/terraform && terraform output -raw cloudfront_domain)/healthz
done | awk '{s+=$1} END {print "p50:", s/NR}'

# Compare: direct ALB (bypasses CDN)
curl -o /dev/null -s -w "time: %{time_total}s\n" \
  http://$(cd infra/terraform && terraform output -raw alb_dns)/healthz
```

Typical results from Taiwan:
- API (ALB, same region): ~5–15ms
- Audio (CloudFront, cache hit): ~3–8ms
- Audio (CloudFront, cache miss → S3): ~25–40ms

---

## Interview questions

> *"Walk me through your AWS architecture. Why did you choose ECS over EKS?"*
> See design decisions above. Key points: no control plane cost, simpler ops, AWS-native integrations, sufficient for a team that isn't already running K8s at scale.

> *"What happens when an availability zone goes down?"*
> ALB stops routing to unhealthy tasks (after 3 health check failures × 30s). ECS launches replacement tasks in the surviving AZ. RDS Aurora promotes a replica to writer (usually <30s). Redis is a single node in this setup — it would be unavailable until ECS brings up a replacement (cache miss fallback in the API). For production: use ElastiCache Replication Group with a replica in each AZ.

> *"Why is the NAT Gateway so expensive?"*
> NAT charges $0.045/GB processed + $0.045/hr. ECS tasks pulling from ECR, writing to S3, and connecting to MSK all go through NAT. The fix: VPC endpoints for S3, ECR, and MSK so that traffic stays on the AWS backbone (free for gateway endpoints, fixed hourly rate for interface endpoints).

---

## Demo

> Phase 8 deploys to real AWS infrastructure. Run `make infra-plan` first to preview costs before applying.

### 1. Deploy the full stack

```bash
cd beatstream

# Fill in DB password and region
cp infra/terraform/terraform.tfvars.example infra/terraform/terraform.tfvars
# edit terraform.tfvars: set db_password

make infra-init    # terraform init
make infra-plan    # preview what will be created + estimated cost
make infra-apply   # create the stack (~15 min — MSK Serverless is the bottleneck)
```

**Expected output from `infra-plan`:** A list of ~40 resources to create — VPC, subnets, ALB, ECS cluster, RDS, ElastiCache, MSK, CloudFront, S3, IAM roles.

---

### 2. Build and push images to ECR

```bash
make infra-push    # docker build → ECR login → docker push api + worker images
make infra-deploy  # force new ECS deployment to pull the latest images
```

**What this demonstrates:** ECS pulls from ECR (AWS-native registry). No credentials needed in the container — the ECS task role (`iam.tf`) grants `ecr:GetDownloadUrlForLayer` automatically.

---

### 3. Verify the stack is healthy

```bash
# Get the ALB DNS name
cd infra/terraform && terraform output alb_dns

# Hit the health endpoint
curl http://$(terraform output -raw alb_dns)/healthz
# → {"status":"ok"}

curl http://$(terraform output -raw alb_dns)/ready
# → {"status":"ready"}
```

---

### 4. Observe ECS task networking — confirm private subnet isolation

In AWS Console → ECS → Cluster: beatstream → Tasks → click any task

**Expected output:**
- Subnet: one of the **private** subnets (10.0.10.x or 10.0.11.x)
- No public IP assigned
- Security group: `beatstream-api` — inbound only from `beatstream-alb` on port 8080

**What this demonstrates:** ECS tasks have no public IP. All inbound traffic must flow through the ALB. The task role provides AWS API access (S3, MSK) without any static credentials.

---

### 5. Simulate AZ failure

```bash
# Watch live traffic while forcing tasks out of one AZ
k6 run k6/load.js &

aws ecs update-service \
  --cluster beatstream \
  --service beatstream-api \
  --placement-constraints '[{"type":"memberOf","expression":"attribute:ecs.availability-zone != ap-northeast-1a"}]' \
  --region ap-northeast-1
```

**Expected output:** ALB detects unhealthy tasks (3 failed health checks × 30s ≈ 90s), drains connections, routes to the surviving AZ. k6 should show p99 latency spike but **zero HTTP errors**.

---

### 6. Tear down

```bash
# Empty the S3 bucket first (Terraform won't delete non-empty buckets)
aws s3 rm s3://$(cd infra/terraform && terraform output -raw audio_bucket) --recursive

make infra-destroy
```

> **Cost reminder:** Stop the stack when not in use. The ALB (~$20/mo), NAT Gateway (~$35/mo), and MSK base fee (~$8/mo) run continuously.

---

**[← Phase 7 — RBAC + GDPR](phase-7.md) · [Back to Beatstream README →](../beatstream/README.md)**
