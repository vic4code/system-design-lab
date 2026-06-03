output "alb_dns" {
  description = "ALB DNS name — use this to test the API before CloudFront is fully propagated."
  value       = aws_lb.main.dns_name
}

output "cloudfront_domain" {
  description = "CloudFront distribution domain — public entry point for the API and audio CDN."
  value       = aws_cloudfront_distribution.main.domain_name
}

output "ecr_api_url" {
  description = "ECR repository URL for the API image."
  value       = aws_ecr_repository.api.repository_url
}

output "ecr_worker_url" {
  description = "ECR repository URL for the worker image."
  value       = aws_ecr_repository.worker.repository_url
}

output "msk_cluster_arn" {
  description = "MSK Serverless cluster ARN. Use with aws kafka get-bootstrap-brokers."
  value       = aws_msk_serverless_cluster.main.arn
}

output "rds_endpoint" {
  description = "Aurora cluster writer endpoint."
  value       = aws_rds_cluster.main.endpoint
}

output "redis_endpoint" {
  description = "ElastiCache Redis endpoint."
  value       = "${aws_elasticache_cluster.redis.cache_nodes[0].address}:6379"
}

output "audio_bucket" {
  description = "S3 bucket name for audio files."
  value       = aws_s3_bucket.audio.bucket
}

output "opensearch_endpoint" {
  description = "OpenSearch domain endpoint for search queries."
  value       = "https://${aws_opensearch_domain.main.endpoint}"
}
