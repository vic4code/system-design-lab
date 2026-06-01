# Network Isolation — Local VPC Simulation

This document explains the network segmentation design in `docker-compose.yml` and how it maps to an AWS VPC architecture. Understanding this before going to the cloud makes the Terraform VPC code obvious rather than mysterious.

---

## The Problem Without Isolation

By default, every Docker Compose service lands on a single shared `bridge` network. Every container can reach every other container — nginx can reach postgres directly, the internet can reach your database if you accidentally expose a port.

In AWS this is equivalent to putting every service in a public subnet with no security groups. One misconfigured firewall rule and your database is on the internet.

---

## Three-Tier Architecture

Beatstream uses three tiers, each on its own network:

```
┌─────────────────────────────────────────────────────────────────┐
│  HOST / INTERNET                                                │
│  curl https://localhost   →   only port 80/443 reachable        │
└────────────────────┬────────────────────────────────────────────┘
                     │
         ┌───────────▼───────────┐
         │   public network      │   AWS: public subnet + IGW
         │                       │
         │   nginx (:80, :443)   │   AWS: ALB
         └───────────┬───────────┘
                     │ app network (nginx → api only)
         ┌───────────▼───────────┐
         │   app network         │   AWS: private subnet (ECS)
         │                       │
         │   api-1, api-2, api-3 │   AWS: ECS Fargate tasks
         │   (no host ports)     │         in private subnets
         └───────────┬───────────┘
                     │ data network (api/worker → data only)
         ┌───────────▼───────────┐
         │   data network        │   AWS: private subnet (data)
         │                       │
         │   postgres            │   AWS: Aurora RDS
         │   redis               │   AWS: ElastiCache
         │   redpanda (Kafka)    │   AWS: MSK Serverless
         │   minio               │   AWS: S3
         │   (no host ports*)    │
         └───────────────────────┘

         * minio :9001 exposed for local console access only
```

A fourth `monitoring` network handles observability (Prometheus, Grafana, Jaeger) without crossing the data plane.

---

## What Each Network Allows and Blocks

| From \ To | nginx | api-1/2/3 | postgres | redis | redpanda | worker |
|-----------|-------|-----------|----------|-------|----------|--------|
| **host**  | ✅ 80/443 | ❌ | ❌ | ❌ | ❌ | ❌ |
| **nginx** | — | ✅ 8080 | ❌ | ❌ | ❌ | ❌ |
| **api**   | — | — | ✅ 5432 | ✅ 6379 | ✅ 9092 | ❌ |
| **worker**| ❌ | ❌ | ✅ 5432 | ❌ | ✅ 9092 | — |

**Key restriction:** nginx cannot resolve the hostname `postgres` or `redis` — they are on different networks. A misconfigured nginx rule cannot accidentally proxy requests to the database.

---

## Docker Networks vs AWS Security Groups

These are related but not identical:

| Concept | Docker Compose | AWS |
|---------|---------------|-----|
| Network segmentation | `networks:` per service | Subnets (public / private) |
| Access control | Shared network = reachable | Security Group inbound rules |
| Direction | Bidirectional within a network | Security Groups are stateful (allow return traffic) |
| Port specificity | Any port if on same network | Security Groups restrict by port + source SG |

**The key difference:** In Docker, if container A and B share a network, A can connect to B on any port — and B can connect back to A on any port. In AWS, a Security Group on RDS that allows `:5432 from api-sg` means:

- api → RDS :5432 ✅
- RDS → api :8080 ❌ (no inbound rule on api-sg for rds-sg)

Docker can't enforce this inbound-only direction within a shared network. What Docker *can* enforce is complete network-level separation — nginx cannot even resolve `postgres` because they share no network.

---

## Data Access Control

Beyond network isolation, Beatstream applies several layers of access control for data:

### Layer 1 — Network (where data lives)
Data services (`postgres`, `redis`, `redpanda`, `minio`) are on the `data` network only. No service outside `data` network can reach them at all. No host port exposure means they are not reachable from your local machine either.

### Layer 2 — Credentials (who can authenticate)
Even if something bypassed network isolation, it still needs credentials:

| Service | Credential | Where stored |
|---------|-----------|-------------|
| Postgres | `user:pass` | `DATABASE_URL` env var, only in api + worker containers |
| Redis | no auth (dev) | network isolation is the only guard |
| MinIO | `minioadmin:minioadmin` | `S3_ACCESS_KEY/S3_SECRET_KEY`, only in api containers |
| Redpanda | PLAINTEXT (dev) | network isolation is the only guard |

In production (AWS):
- **RDS**: password in Secrets Manager, ECS task role has `secretsmanager:GetSecretValue`
- **ElastiCache**: auth token in Secrets Manager
- **MSK**: IAM authentication (SASL/OAUTHBEARER) — no password, the ECS task role IS the credential
- **S3**: IAM role on ECS task, no static keys at all

### Layer 3 — Application (what operations are allowed)
The API enforces what authenticated users can do:
- `RequireAuth` — JWT must be valid
- `RequireRole("admin")` — admin-only endpoints
- Parameterized queries — prevents SQL injection
- Audit log — every write recorded (who, what, when, from where)

### Layer 4 — Encryption (data in transit + at rest)
Local:
- nginx terminates TLS (HTTPS only, `Strict-Transport-Security` header)
- Internal container communication is plaintext (trusted network)

AWS production:
- ALB → ECS: HTTPS (ACM certificate)
- ECS → RDS: TLS enforced (`sslmode=require`)
- ECS → MSK: TLS (port 9098, SASL/IAM)
- ECS → ElastiCache: TLS + auth token
- RDS storage: encrypted at rest (KMS CMK)
- S3 objects: SSE-S3 by default

---

## Verifying Isolation Locally

```bash
# nginx cannot reach postgres (different network — DNS resolution fails)
docker exec beatstream-nginx-1 sh -c "nc -zv postgres 5432" 2>&1
# → nc: bad address 'postgres'   ← hostname doesn't resolve across networks

# nginx can reach api (shared app network)
docker exec beatstream-nginx-1 sh -c "nc -zv api-1 8080" 2>&1
# → api-1 (172.x.x.x:8080) open

# api can reach postgres (shared data network)
docker exec beatstream-api-1-1 sh -c "nc -zv postgres 5432" 2>&1
# → postgres (172.x.x.x:5432) open

# postgres port NOT exposed to host
nc -zv localhost 5432 2>&1
# → Connection refused   ← no host port mapping on postgres

# redis port NOT exposed to host
nc -zv localhost 6379 2>&1
# → Connection refused
```

---

## AWS Mapping — Terraform

The `infra/terraform/` directory implements the same design in AWS:

| Local | AWS (Terraform file) |
|-------|----------------------|
| `public` network | `aws_subnet.public` — IGW route, ALB lives here |
| `app` network | `aws_subnet.private` — ECS tasks, no IGW route |
| `data` network | `aws_subnet.private` — same subnets, controlled by SGs |
| nginx | ALB (`alb.tf`) + `aws_security_group.alb` |
| api containers | ECS tasks (`ecs.tf`) + `aws_security_group.api` |
| postgres | Aurora RDS (`rds.tf`) + `aws_security_group.rds` (allows :5432 from api-sg + worker-sg only) |
| redis | ElastiCache (`elasticache.tf`) + `aws_security_group.redis` (allows :6379 from api-sg only) |
| redpanda | MSK Serverless (`msk.tf`) + `aws_security_group.msk` (allows :9098 from api-sg + worker-sg only) |
| minio | S3 (`s3.tf`) — no security group (S3 is a global service, access via IAM + bucket policy) |

One additional AWS layer not present locally:

**NAT Gateway** — ECS tasks in private subnets need internet access (to pull from ECR, push to CloudWatch). They can't have public IPs (private subnet). NAT Gateway sits in the public subnet and provides one-way outbound internet — private resources can initiate connections out but nothing from the internet can initiate connections in.

```
Private subnet (ECS task)
    → NAT Gateway (public subnet, Elastic IP)
        → Internet Gateway
            → Internet (ECR, CloudWatch, Secrets Manager)

Internet
    → Cannot reach ECS task directly (no inbound route to private subnet)
```

This is why the Terraform has `aws_nat_gateway` and why it's the most expensive single line item (~$35/month).
