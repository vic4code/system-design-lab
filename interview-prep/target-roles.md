# Target Roles — Interview Prep

Four roles across Ubiquiti and Amazon AWS. All Taiwan-based, heavy AWS/cloud architecture focus.

| Company | Role | Level | Key Angle |
|---------|------|-------|-----------|
| Ubiquiti | Cloud Architect | Senior IC | AWS infra design, IaC, Java/Go |
| Amazon AWS | Senior Prototyping Architect (PACE) | Senior IC | GenAI prototypes, distributed systems |
| Amazon AWS | PACE Engagement Manager | Technical PM | Customer-facing, scoping, cross-functional |
| Amazon AWS | Data & AI Architect (Pro Services) | Senior IC | Data platforms, ML/AI on AWS, Taiwan enterprise |

---

## Cross-Role Themes (show up in 3–4 of the JDs)

These are the highest-signal topics — study these first.

### 1. AWS Core Services Depth
Every role expects fluency in:
- **Compute**: EC2, ECS/Fargate, Lambda, EKS
- **Storage**: S3, EBS, EFS — and when to choose each
- **Networking**: VPC, subnets, security groups, NACLs, PrivateLink, Transit Gateway
- **IAM**: least-privilege, roles vs. policies, cross-account trust, service control policies (SCPs)
- **Observability**: CloudWatch, X-Ray, CloudTrail

*Key question types:* "Design a secure multi-account AWS architecture" / "How would you restrict blast radius?"

### 2. Distributed Systems Design
- CAP theorem — consistency vs. availability trade-offs
- Event-driven architecture: Kafka / MSK, SQS/SNS, EventBridge
- Database selection: RDS vs. DynamoDB vs. Redshift vs. Aurora — and why
- Caching strategies: Redis/ElastiCache, CDN (CloudFront), write-through vs. read-through
- API design: REST vs. GraphQL vs. gRPC

*Key question types:* "Design a real-time data pipeline for 10M events/day" / "How do you handle eventual consistency?"

### 3. Infrastructure as Code
- **Terraform**: module structure, state management, remote backends, workspaces
- **CloudFormation** / CDK as fallback
- CI/CD: CodePipeline, GitHub Actions, trunk-based development

*Key question:* "Walk me through how you'd structure Terraform for a multi-environment, multi-region deployment."

### 4. Generative AI / ML on AWS
Explicitly required in all three Amazon roles:
- **Amazon Bedrock**: model invocation, knowledge bases (RAG), agents
- **Amazon SageMaker**: training, inference endpoints, MLflow tracking
- RAG architecture: embeddings → vector store (OpenSearch, pgvector) → retrieval → LLM generation
- Fine-tuning vs. RAG vs. prompt engineering — when to use which
- GenAI application patterns: multi-turn conversation, tool use, guardrails

*Key question:* "A customer wants to build a chatbot over internal documents — walk me through the architecture."

### 5. Security & Compliance
- Encryption at rest (KMS, CMKs) and in transit (TLS, ACM)
- Network segmentation: public vs. private subnets, NAT gateway, bastion hosts
- GDPR / data privacy (especially for Data & AI Architect role — Taiwan PDPA)
- Zero-trust principles; WAF + Shield for DDoS

### 6. Cost Optimization
- Reserved Instances vs. Savings Plans vs. Spot
- Right-sizing recommendations
- Data transfer costs (often missed in architecture reviews)
- Tagging strategy for cost allocation

---

## Role-Specific Prep

### Ubiquiti — Cloud Architect

**What they want:** Someone to own AWS architecture end-to-end, hands-on with Terraform, writes Go or Java, can mentor engineers.

**Extra topics:**
- Container orchestration: EKS vs. ECS, Helm charts, pod autoscaling (HPA/KEDA)
- Production reliability: SLOs/SLIs/error budgets, chaos engineering
- Spec-driven development (OpenAPI-first)

**Behavioural angle:** "Tell me about a time you led a cloud migration" — have a STAR story ready covering scope, risk mitigation, and outcome.

---

### Amazon — Senior Prototyping Architect (PACE)

**What they want:** 10+ years, can ship a working GenAI prototype in days, not weeks. Comfort with ambiguity.

**Extra topics:**
- Rapid prototyping patterns: mock-first, vertical slice delivery, MVP scoping
- Multi-agent AI orchestration (Bedrock Agents, LangChain, LangGraph)
- Agentic workflows: tool use, memory, reflection loops
- Python and TypeScript fluency (both listed as required)

**Amazon LP angle:** "Customer Obsession" and "Bias for Action" are the dominant LPs for this role. Prep stories around: shipping fast under uncertainty, turning a vague customer ask into a working demo.

**Key question:** "Given a 4-week engagement, how do you decide what to prototype vs. what to stub?" — answer around vertical slices and de-risking the most uncertain assumption first.

---

### Amazon — PACE Engagement Manager

**What they want:** Technical enough to talk to architects, commercial enough to scope deals. Think: pre-sales + program management.

**Extra topics:**
- Engagement scoping: defining success metrics upfront, managing scope creep
- Executive communication: translating technical value into business outcomes
- AWS Partner/Enterprise support ecosystem

**Amazon LP angle:** "Earn Trust", "Think Big", "Are Right, A Lot". Prep stories around influencing without authority and managing ambiguous programs.

**Key question:** "How do you qualify whether a prototyping engagement is worth pursuing?" — answer around strategic fit, customer commitment, and technical feasibility signal.

---

### Amazon — Data & AI Architect (Pro Services Taiwan)

**What they want:** Deep data platform + ML/AI architecture, Taiwan enterprise customers (manufacturing, semiconductor), bilingual.

**Extra topics:**
- Data lake / lakehouse patterns: S3 + Glue + Athena, Apache Iceberg, Delta Lake
- Data mesh concepts: domain ownership, data products, federated governance
- Streaming: Kinesis Data Streams vs. MSK, Flink on EMR
- Industry 4.0: IoT Core, Greengrass, digital twin patterns
- Taiwan PDPA (個資法) — similar to GDPR, applies to data collected in Taiwan

**Technical deep-dive questions:**
- "Design a data platform for a Taiwan semiconductor manufacturer with real-time quality control requirements"
- "How would you implement a data governance layer across a data lake?"

**Amazon LP angle:** "Deliver Results", "Dive Deep". Have metrics-driven stories: "I reduced data pipeline latency from X to Y" or "enabled customer to cut reporting time by Z%".

---

## Behavioural Questions (Amazon Leadership Principles)

All three Amazon roles will run LP-based interviews (typically 4–5 rounds, each LP-focused).

| LP | Likely probe | Angle for your background |
|----|-------------|---------------------------|
| Customer Obsession | "Tell me about a time you went beyond requirements for a customer" | Beatstream: added audit trail + security headers proactively |
| Bias for Action | "Describe a time you made a decision with incomplete data" | Phase rollout without full spec |
| Dive Deep | "Give an example of finding the root cause others missed" | Phase 2 Kafka idempotency / Go version drift bugs |
| Earn Trust | "Tell me about a time you disagreed with a stakeholder" | — |
| Deliver Results | "Most significant project outcome with metrics" | Quantify: latency, throughput, user count |
| Think Big | "Describe a time you proposed a vision that changed direction" | — |

---

## System Design Questions to Prepare

These span the JD requirements across all four roles:

1. **Design a multi-tenant SaaS platform on AWS** (Ubiquiti angle)
2. **Design a real-time streaming analytics pipeline** (Data & AI Architect angle)
3. **Design a RAG-based enterprise chatbot on AWS Bedrock** (PACE Prototyping angle)
4. **Design a data lake for a manufacturing company with governance** (Data & AI Architect angle)
5. **Design a secure, multi-region AWS infrastructure with IaC** (Cloud Architect angle)
6. **Design a GenAI agent that can query internal databases** (PACE Prototyping angle)

For each: start with requirements → capacity estimation → high-level design → deep-dive on 2–3 components → trade-offs → operational considerations.
