# Phase 3 — Kubernetes

## Architecture

```mermaid
graph TD
    Client(["Client"])

    subgraph "kind / Kubernetes Cluster"
        ING["Ingress (nginx-ingress)\nbeatstream.local:80"]

        subgraph "beatstream namespace"
            subgraph "API Deployment (HPA: 2–10 pods)"
                API1["api pod"]
                API2["api pod"]
                API3["api pod"]
            end
            APIS["Service: api\nClusterIP :80"]

            subgraph "Worker Deployment (1 pod)"
                WRK["worker pod"]
            end

            subgraph "StatefulSets"
                PG[("postgres-0\nPVC: 5Gi")]
                RP["redpanda-0\nPVC: 10Gi"]
            end

            REDIS["redis pod\n(Deployment)"]
            MINIO["minio pod\n(Deployment)"]

            CM["ConfigMap\nbeatstream-config"]
            SEC["Secret\nbeatstream-secrets"]
        end
    end

    Client -->|"HTTP"| ING
    ING --> APIS
    APIS --> API1 & API2 & API3
    API1 & API2 & API3 --> PG & REDIS & MINIO & RP
    WRK --> PG & RP
    CM & SEC -.->|"envFrom"| API1
    CM & SEC -.->|"envFrom"| WRK
```

## Goal

Replace Docker Compose with Kubernetes to gain:
1. **Automatic horizontal scaling** (HPA) — react to CPU spikes without manual intervention.
2. **Self-healing** — pods that crash or fail health checks are automatically restarted or replaced.
3. **Rolling zero-downtime deploys** — update the API without dropping requests.
4. **Declarative config separation** — credentials go into Secrets, not baked into YAML.

---

## Files added

```
beatstream/k8s/
├── namespace.yaml             # beatstream namespace
├── configmap.yaml             # non-sensitive env config
├── secret.yaml                # credentials (DB, MinIO, Redis)
├── api-deployment.yaml        # Deployment: 3 replicas, rolling update, probes
├── api-service.yaml           # ClusterIP Service → HPA target
├── api-hpa.yaml               # HPA: 2–10 pods, CPU 60%, memory 70%
├── worker-deployment.yaml     # Deployment: 1 replica, Recreate strategy
├── postgres-statefulset.yaml  # StatefulSet + volumeClaimTemplate 5Gi
├── postgres-service.yaml      # Headless service (stable pod DNS)
├── redis-deployment.yaml      # Deployment + ClusterIP service
├── redpanda-statefulset.yaml  # StatefulSet + volumeClaimTemplate 10Gi
│                              # + headless service for stable broker FQDN
├── minio-deployment.yaml      # Deployment + ClusterIP service
└── ingress.yaml               # Ingress (nginx): routes /v1/, /healthz, /ready
```

---

## Docker Compose → Kubernetes conceptual mapping

| Docker Compose concept | Kubernetes equivalent | Key difference |
|---|---|---|
| `image: foo` + `replicas: 3` (scale) | `Deployment` with `replicas: 3` | K8s manages ReplicaSet and pod lifecycle |
| `healthcheck` | `livenessProbe` + `readinessProbe` | **Two separate probes** with different semantics |
| Container name DNS (`postgres`) | `Service` name DNS | Must create a Service to get DNS; also exposes load balancing |
| Named volumes | `PersistentVolumeClaim` | PVC decouples storage lifecycle from pod |
| `restart: unless-stopped` | Controller reconciliation loop | K8s continuously reconciles desired ↔ actual |
| `environment:` | `ConfigMap` + `Secret` | Separation of sensitivity; mounted at runtime, not bake-in |
| nginx upstream + round-robin | `Service` (kube-proxy) | Built into the platform; no nginx needed for internal LB |
| No autoscaling | `HorizontalPodAutoscaler` | HPA watches metrics, adjusts replicas automatically |
| `docker compose up` | `kubectl apply -f k8s/` | Declarative: K8s figures out the diff |

---

## livenessProbe vs readinessProbe

These are the most important probe distinction in Kubernetes interviews.

```
/healthz  →  livenessProbe   →  "is the process alive?"
/ready    →  readinessProbe  →  "is the pod ready to serve traffic?"
```

**livenessProbe** — if it fails, kubelet **kills and restarts** the pod.
- Use for detecting deadlocks, unrecoverable crashes.
- NEVER check external dependencies (DB, Redis) here. If DB goes down, you don't want all pods to restart — that makes the problem worse.
- Our `/healthz` just returns 200.

**readinessProbe** — if it fails, the pod is **removed from Service endpoints** (no traffic routed to it) but NOT restarted.
- Use for detecting temporary unavailability: starting up, DB overloaded, warming cache.
- Our `/ready` pings PostgreSQL — if DB is unhealthy, the pod stops receiving traffic but keeps running.

```
Timeline:
  pod starts → readiness fails (not in Service) → DB connects → readiness passes → traffic begins
                                                ↕
                               liveness fails → pod killed → pod restarted
```

---

## HPA — How it works

```
HPA controller (runs in control plane, every 15s by default)
  → reads pod CPU metrics from metrics-server
  → desired replicas = ceil(current_replicas × (current_utilization / target_utilization))
  → patches Deployment.spec.replicas
```

Example:
- 3 pods, each requesting 100m CPU
- Current average usage: 90m per pod → 90% utilization
- Target: 60%
- desired = ceil(3 × (90/60)) = ceil(4.5) = 5 pods

**Critical requirement**: `resources.requests.cpu` must be set on the container. HPA calculates utilization relative to requests. Without requests, metrics-server can't compute utilization and HPA does nothing.

**Scale-down stabilization (300s)**: prevents flapping — HPA waits 5 minutes of sustained low load before removing pods. Scale-up has 0s stabilization so it reacts to spikes immediately.

---

## StatefulSet vs Deployment

| | Deployment | StatefulSet |
|---|---|---|
| Pod names | random (`api-7d6f5-abc`) | stable, ordered (`postgres-0`, `postgres-1`) |
| DNS | via Service only | `<pod>.<service>.<ns>.svc.cluster.local` |
| Scaling | parallel | ordered (0→1→2 up, 2→1→0 down) |
| Storage | shared PVC or emptyDir | one PVC **per pod** via `volumeClaimTemplates` |
| Use for | stateless apps | databases, brokers (Postgres, Redpanda) |

We use StatefulSet for **Postgres** and **Redpanda** because:
- They need **stable network identity**: the advertised Kafka address must match the pod's DNS name.
- They need **per-pod persistent storage**: each broker stores its own log data.

---

## Worker scaling and partition assignment

```
topic: track.uploads (1 partition)
consumer group: upload-workers

replicas=1 → consumer-0 owns partition-0  ✓
replicas=2 → consumer-0 owns partition-0, consumer-1 is idle  ✗ (wasted)
```

**Rule**: max useful worker replicas = number of topic partitions. Beyond that, extra consumers sit idle. To scale workers, first increase Redpanda partitions:

```bash
rpk topic alter-config track.uploads --set num.partitions=3
```

Then scale workers to 3. Redpanda rebalances automatically.

Worker uses `strategy: Recreate` (not RollingUpdate) because with 1 partition, a rolling update would briefly create 2 consumers in the same group, causing one to be idle and potentially causing a rebalance during the update. Recreate stops old before starting new.

---

## ConfigMap vs Secret

| | ConfigMap | Secret |
|---|---|---|
| Stores | non-sensitive config | credentials, tokens, TLS certs |
| Encoded | plaintext | base64 (not encrypted by default!) |
| RBAC | standard | can restrict with stricter RBAC |
| Audit | standard | separately auditable |

**Important**: base64 is not encryption. In production, encrypt Secrets at rest (etcd encryption), or use external secret management: Vault, AWS Secrets Manager, Sealed Secrets, or External Secrets Operator.

In this lab: `stringData` → K8s base64-encodes on apply. In production: never commit secret values to Git.

---

## Service types

| Type | Reachable from | Use case |
|---|---|---|
| `ClusterIP` (default) | inside cluster only | service-to-service (API → Postgres) |
| `NodePort` | outside via node IP + high port | dev/testing, no LB |
| `LoadBalancer` | outside via cloud LB IP | production ingress (cloud only) |
| `Headless` (clusterIP: None) | inside, per-pod DNS | StatefulSets, Redpanda broker discovery |

Our stack:
- **api**: ClusterIP (Ingress routes to it)
- **postgres**: Headless (stable pod DNS for StatefulSet)
- **redis, minio**: ClusterIP
- **redpanda**: Headless (broker advertises its pod FQDN)

---

## Local development with kind

```bash
# 1. Create cluster
kind create cluster --name beatstream

# 2. Build images
docker build -t beatstream-api:latest --target api .
docker build -t beatstream-worker:latest --target worker .

# 3. Load images into kind (bypasses registry)
kind load docker-image beatstream-api:latest --name beatstream
kind load docker-image beatstream-worker:latest --name beatstream

# 4. Install NGINX Ingress Controller
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.10.1/deploy/static/provider/kind/deploy.yaml

# 5. Deploy everything
kubectl apply -f k8s/

# 6. Wait for pods
kubectl -n beatstream get pods -w

# 7. Add to /etc/hosts
echo "127.0.0.1 beatstream.local" | sudo tee -a /etc/hosts

# 8. Test
curl http://beatstream.local/healthz
curl -X POST http://beatstream.local/v1/artists -H 'Content-Type: application/json' \
  -d '{"name":"Daft Punk","bio":"French electronic music duo"}'

# Tear down
kind delete cluster --name beatstream
```

---

## Interview questions

**Q: What is the difference between livenessProbe and readinessProbe?**

Liveness: "is the process alive?" — failing causes a pod restart. Use a cheap check that doesn't depend on external services. Readiness: "is the pod ready to serve traffic?" — failing removes the pod from the Service endpoint list but does NOT restart it. Use to block traffic during startup or temporary unavailability (e.g., DB connection not yet established). Never check external dependencies in liveness — if the DB goes down, you'd restart all pods simultaneously, creating a thundering herd on DB reconnect.

**Q: HPA is configured but pods aren't scaling. What would you check?**

1. Is `metrics-server` installed? (`kubectl top pods` — if it fails, metrics-server is missing)
2. Do pods have `resources.requests.cpu` set? HPA calculates utilization as usage/request — no request means no utilization.
3. Is the HPA showing `<unknown>` for current utilization? (`kubectl describe hpa api -n beatstream`)
4. Is the current load actually exceeding the target? Use `kubectl top pods -n beatstream` to check.

**Q: Why use a StatefulSet for Postgres instead of a Deployment?**

Deployments give pods random names and don't guarantee stable storage — on rescheduling, a pod might get a different PVC. StatefulSets give pods stable names (`postgres-0`) and stable network identity (`postgres-0.postgres.beatstream.svc.cluster.local`), and `volumeClaimTemplates` ensure each pod always reconnects to its own PVC regardless of which node it lands on.

**Q: A rolling update is in progress and error rates spike. How do you roll back?**

```bash
kubectl rollout undo deployment/api -n beatstream
# or to a specific revision:
kubectl rollout history deployment/api -n beatstream
kubectl rollout undo deployment/api --to-revision=2 -n beatstream
```

The Deployment keeps a revision history (default: 10). Rollback is instant — K8s updates the ReplicaSet selector, old pods come back up. Our `maxUnavailable: 0` config ensures no requests are dropped during rollout, so you have time to detect the error before all old pods are replaced.
