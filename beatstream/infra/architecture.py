from diagrams import Cluster, Diagram, Edge
from diagrams.aws.network import CloudFront, ALB, NATGateway
from diagrams.aws.compute import ECS
from diagrams.aws.database import Aurora, ElastiCache
from diagrams.aws.storage import S3
from diagrams.aws.integration import MQ
from diagrams.aws.security import IAM, WAF
from diagrams.aws.management import Cloudwatch, Cloudtrail
from diagrams.aws.analytics import ElasticsearchService
from diagrams.onprem.client import Users

with Diagram(
    "Beatstream - Phase 9 AWS Architecture",
    filename="architecture",
    outformat="png",
    show=False,
    direction="TB",
):
    users = Users("Users\n(Taiwan)")

    with Cluster("Edge (Global)"):
        waf = WAF("WAF v2\n(OWASP, Rate Limit)")
        cdn = CloudFront("CloudFront\nPriceClass_200")

    users >> Edge(label="HTTPS") >> waf >> cdn

    with Cluster("Observability"):
        cw = Cloudwatch("CloudWatch\nAlarms + Logs")
        ct = Cloudtrail("CloudTrail\n(audit)")
        guardduty = IAM("GuardDuty\n(threat detection)")

    with Cluster("VPC 10.0.0.0/16 - ap-northeast-1"):
        with Cluster("Public Subnets (AZ-a + AZ-c)"):
            alb = ALB("ALB\n(HTTP :80)")
            nat = NATGateway("NAT Gateway")

        with Cluster("Private Subnets (AZ-a + AZ-c)"):
            with Cluster("ECS Fargate - Compute (Auto Scaling 2-10)"):
                api = ECS("API Service\n(x2 tasks, 0.5 vCPU)")
                worker = ECS("Worker Service\n(ffmpeg transcode,\n1 vCPU)")

            with Cluster("Data Stores"):
                rds = Aurora("Aurora PostgreSQL\nServerless v2\n(0.5-4 ACUs)")
                redis = ElastiCache("Redis\ncache.t4g.micro")
                msk = MQ("MSK Serverless\n(SASL/IAM)")
                opensearch = ElasticsearchService("OpenSearch\nt3.small\n(fuzzy search)")

            s3 = S3("S3 Audio Bucket\n(multi-bitrate OGG)")

    # CDN routing
    cdn >> Edge(label="/audio/*\n(24h cache)") >> s3
    cdn >> Edge(label="default\n(no cache)") >> alb

    # ALB to API
    alb >> Edge(label=":8080") >> api

    # API connections
    api >> Edge(label="port 5432") >> rds
    api >> Edge(label="port 6379") >> redis
    api >> Edge(label="produce\nport 9098") >> msk
    api >> Edge(label="read/write") >> s3
    api >> Edge(label="search/index") >> opensearch

    # Worker connections
    msk >> Edge(label="consume") >> worker
    worker >> Edge(label="port 5432") >> rds
    worker >> Edge(label="transcode\n64/128/320 kbps") >> s3
    worker >> Edge(label="index tracks") >> opensearch

    # NAT for outbound
    api >> Edge(style="dashed", label="outbound") >> nat

    # Observability connections
    api >> Edge(style="dotted", color="gray") >> cw
    alb >> Edge(style="dotted", color="gray") >> cw
