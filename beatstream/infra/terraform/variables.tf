variable "aws_region" {
  description = "AWS region. ap-northeast-1 (Tokyo) minimises latency from Taiwan."
  type        = string
  default     = "ap-northeast-1"
}

variable "environment" {
  description = "Deployment environment tag (e.g. dev, staging, prod)."
  type        = string
  default     = "dev"
}

variable "db_password" {
  description = "Master password for Aurora PostgreSQL. Never commit this value."
  type        = string
  sensitive   = true
}

variable "api_image_tag" {
  description = "ECR image tag for the API container."
  type        = string
  default     = "latest"
}

variable "worker_image_tag" {
  description = "ECR image tag for the worker container."
  type        = string
  default     = "latest"
}
