# Origin Access Control (OAC) — the modern replacement for OAI.
# Signs requests to S3 with SigV4 so the bucket can stay fully private.
resource "aws_cloudfront_origin_access_control" "s3" {
  name                              = "beatstream-s3-oac"
  origin_access_control_origin_type = "s3"
  signing_behavior                  = "always"
  signing_protocol                  = "sigv4"
}

resource "aws_cloudfront_distribution" "main" {
  enabled         = true
  price_class     = "PriceClass_200" # US + Europe + Asia (covers Taiwan, Japan)
  comment         = "beatstream CDN — audio files + API"
  web_acl_id      = aws_wafv2_web_acl.main.arn

  # Origin 1: S3 bucket (audio files)
  origin {
    domain_name              = aws_s3_bucket.audio.bucket_regional_domain_name
    origin_id                = "s3-audio"
    origin_access_control_id = aws_cloudfront_origin_access_control.s3.id
  }

  # Origin 2: ALB (API)
  origin {
    domain_name = aws_lb.main.dns_name
    origin_id   = "alb-api"
    custom_origin_config {
      http_port              = 80
      https_port             = 443
      origin_protocol_policy = "http-only"
      origin_ssl_protocols   = ["TLSv1.2"]
    }
  }

  # Default behaviour: forward to API, no caching (dynamic content).
  default_cache_behavior {
    target_origin_id       = "alb-api"
    viewer_protocol_policy = "allow-all"
    allowed_methods        = ["DELETE", "GET", "HEAD", "OPTIONS", "PATCH", "POST", "PUT"]
    cached_methods         = ["GET", "HEAD"]
    # CachingDisabled managed policy — every request forwarded to origin
    cache_policy_id = "4135ea2d-6df8-44a3-9df3-4b5a84be39ad"
  }

  # /audio/* → S3 with aggressive caching.
  # Audio files are immutable once uploaded (key includes track ID).
  # CloudFront serves from edge PoP after first cache-fill; origin hit rate is very low.
  ordered_cache_behavior {
    path_pattern           = "/audio/*"
    target_origin_id       = "s3-audio"
    viewer_protocol_policy = "allow-all"
    allowed_methods        = ["GET", "HEAD"]
    cached_methods         = ["GET", "HEAD"]
    # CachingOptimized managed policy — Brotli/gzip, 24h default TTL
    cache_policy_id = "658327ea-f89d-4fab-a63d-7e88639e58f6"
  }

  restrictions {
    geo_restriction { restriction_type = "none" }
  }

  viewer_certificate {
    cloudfront_default_certificate = true
    # For a custom domain + HTTPS:
    # acm_certificate_arn      = aws_acm_certificate.main.arn
    # ssl_support_method       = "sni-only"
    # minimum_protocol_version = "TLSv1.2_2021"
  }
}

# Grant CloudFront OAC permission to read the S3 bucket.
# Without this the bucket returns 403 and CloudFront returns 403 to clients.
resource "aws_s3_bucket_policy" "audio" {
  bucket = aws_s3_bucket.audio.id
  policy = data.aws_iam_policy_document.s3_cloudfront.json
}

data "aws_iam_policy_document" "s3_cloudfront" {
  statement {
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.audio.arn}/*"]
    principals {
      type        = "Service"
      identifiers = ["cloudfront.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "AWS:SourceArn"
      values   = [aws_cloudfront_distribution.main.arn]
    }
  }
}
