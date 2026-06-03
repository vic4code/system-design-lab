terraform {
  required_version = ">= 1.6"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    null = {
      source  = "hashicorp/null"
      version = "~> 3.0"
    }
  }

  # Uncomment after manually creating the state bucket:
  # backend "s3" {
  #   bucket = "beatstream-terraform-state"
  #   key    = "beatstream/terraform.tfstate"
  #   region = "ap-northeast-1"
  # }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "beatstream"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}

# CloudFront WAF must be in us-east-1 regardless of where other resources live.
provider "aws" {
  alias  = "us_east_1"
  region = "us-east-1"

  default_tags {
    tags = {
      Project     = "beatstream"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  }
}

data "aws_caller_identity" "current" {}
