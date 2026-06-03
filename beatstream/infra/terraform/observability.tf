# VPC Flow Logs - captures all network traffic metadata (src/dst IP, port, action).
# Interview question: "How do you debug 'can't connect to the database'?"
# Answer: VPC Flow Logs show whether the packet was ACCEPTED or REJECTED by the SG.

resource "aws_flow_log" "vpc" {
  vpc_id               = aws_vpc.main.id
  traffic_type         = "ALL"
  iam_role_arn         = aws_iam_role.flow_logs.arn
  log_destination      = aws_cloudwatch_log_group.flow_logs.arn
  log_destination_type = "cloud-watch-logs"
  max_aggregation_interval = 60
}

resource "aws_cloudwatch_log_group" "flow_logs" {
  name              = "/vpc/beatstream/flow-logs"
  retention_in_days = 7
}

resource "aws_iam_role" "flow_logs" {
  name = "beatstream-vpc-flow-logs"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = { Service = "vpc-flow-logs.amazonaws.com" }
    }]
  })
}

resource "aws_iam_role_policy" "flow_logs" {
  name = "beatstream-flow-logs-write"
  role = aws_iam_role.flow_logs.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents",
        "logs:DescribeLogGroups",
        "logs:DescribeLogStreams",
      ]
      Resource = "*"
    }]
  })
}

# CloudTrail - "who did what and when" audit log.
# Interview question: "Someone deleted a security group at 3am. How do you find out who?"
# Answer: CloudTrail logs every AWS API call with caller identity, timestamp, and parameters.

resource "aws_cloudtrail" "main" {
  name                       = "beatstream-trail"
  s3_bucket_name             = aws_s3_bucket.cloudtrail.id
  include_global_service_events = true
  is_multi_region_trail      = false
  enable_logging             = true

  event_selector {
    read_write_type           = "All"
    include_management_events = true
  }

  depends_on = [aws_s3_bucket_policy.cloudtrail]
}

resource "aws_s3_bucket" "cloudtrail" {
  bucket        = "beatstream-cloudtrail-${data.aws_caller_identity.current.account_id}"
  force_destroy = true
}

resource "aws_s3_bucket_policy" "cloudtrail" {
  bucket = aws_s3_bucket.cloudtrail.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:GetBucketAcl"
        Resource  = aws_s3_bucket.cloudtrail.arn
      },
      {
        Effect    = "Allow"
        Principal = { Service = "cloudtrail.amazonaws.com" }
        Action    = "s3:PutObject"
        Resource  = "${aws_s3_bucket.cloudtrail.arn}/AWSLogs/${data.aws_caller_identity.current.account_id}/*"
        Condition = {
          StringEquals = { "s3:x-amz-acl" = "bucket-owner-full-control" }
        }
      },
    ]
  })
}

# GuardDuty - threat detection (brute force, crypto mining, compromised credentials).
# Interview question: "How do you detect if an ECS task is compromised and mining crypto?"
# Answer: GuardDuty analyzes VPC Flow Logs, DNS logs, and CloudTrail for anomalous patterns.
# It's a one-click enable - no agents, no config. Alerts go to EventBridge → SNS.

resource "aws_guardduty_detector" "main" {
  enable = true
  finding_publishing_frequency = "FIFTEEN_MINUTES"
}

# Route GuardDuty findings to SNS so you get notified.
resource "aws_cloudwatch_event_rule" "guardduty" {
  name        = "beatstream-guardduty-findings"
  description = "Forward GuardDuty findings to SNS"
  event_pattern = jsonencode({
    source      = ["aws.guardduty"]
    detail-type = ["GuardDuty Finding"]
  })
}

resource "aws_cloudwatch_event_target" "guardduty_sns" {
  rule = aws_cloudwatch_event_rule.guardduty.name
  arn  = aws_sns_topic.alerts.arn
}

# Allow EventBridge to publish to SNS.
resource "aws_sns_topic_policy" "alerts" {
  arn = aws_sns_topic.alerts.arn
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect    = "Allow"
        Principal = { Service = "events.amazonaws.com" }
        Action    = "SNS:Publish"
        Resource  = aws_sns_topic.alerts.arn
      },
    ]
  })
}
