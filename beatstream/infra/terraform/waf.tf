# WAF v2 attached to CloudFront - filters malicious requests before they reach the ALB.
# Uses AWS Managed Rule Groups (free tier covers most use cases).
resource "aws_wafv2_web_acl" "main" {
  name        = "beatstream-waf"
  description = "WAF for CloudFront - blocks OWASP Top 10, bots, and rate-limits abusers"
  scope       = "CLOUDFRONT"
  provider    = aws.us_east_1 # CloudFront WAF must be in us-east-1

  default_action {
    allow {}
  }

  # Rule 1: AWS Managed - Core Rule Set (XSS, SQLi, path traversal, etc.)
  rule {
    name     = "aws-managed-common"
    priority = 1
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        vendor_name = "AWS"
        name        = "AWSManagedRulesCommonRuleSet"
      }
    }
    visibility_config {
      sampled_requests_enabled   = true
      cloudwatch_metrics_enabled = true
      metric_name                = "aws-common-rules"
    }
  }

  # Rule 2: AWS Managed - SQL Injection
  rule {
    name     = "aws-managed-sqli"
    priority = 2
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        vendor_name = "AWS"
        name        = "AWSManagedRulesSQLiRuleSet"
      }
    }
    visibility_config {
      sampled_requests_enabled   = true
      cloudwatch_metrics_enabled = true
      metric_name                = "aws-sqli-rules"
    }
  }

  # Rule 3: Rate limiting - 2000 requests per 5 min per IP.
  # Protects against brute-force and simple DDoS at Layer 7.
  rule {
    name     = "rate-limit"
    priority = 3
    action {
      block {}
    }
    statement {
      rate_based_statement {
        limit              = 2000
        aggregate_key_type = "IP"
      }
    }
    visibility_config {
      sampled_requests_enabled   = true
      cloudwatch_metrics_enabled = true
      metric_name                = "rate-limit"
    }
  }

  # Rule 4: AWS Managed - Known Bad Inputs (Log4j, etc.)
  rule {
    name     = "aws-managed-bad-inputs"
    priority = 4
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        vendor_name = "AWS"
        name        = "AWSManagedRulesKnownBadInputsRuleSet"
      }
    }
    visibility_config {
      sampled_requests_enabled   = true
      cloudwatch_metrics_enabled = true
      metric_name                = "aws-bad-inputs"
    }
  }

  visibility_config {
    sampled_requests_enabled   = true
    cloudwatch_metrics_enabled = true
    metric_name                = "beatstream-waf"
  }
}

# Associate WAF with CloudFront distribution.
resource "aws_wafv2_web_acl_association" "cloudfront" {
  resource_arn = aws_cloudfront_distribution.main.arn
  web_acl_arn  = aws_wafv2_web_acl.main.arn
  provider     = aws.us_east_1
}
