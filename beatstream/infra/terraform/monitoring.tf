# CloudWatch Alarms + SNS - "How do you know when something breaks?"
# Interview answer: automated alerts fire before users notice.

# SNS topic for all alerts. Subscribe your email/Slack/PagerDuty here.
resource "aws_sns_topic" "alerts" {
  name = "beatstream-alerts"
}

# Alarm 1: API 5xx errors > 1% of requests in 5 minutes.
# Why: 5xx means the server is failing - users are getting errors.
resource "aws_cloudwatch_metric_alarm" "alb_5xx" {
  alarm_name          = "beatstream-alb-5xx-high"
  alarm_description   = "ALB is returning >1% HTTP 5xx errors"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  threshold           = 1
  treat_missing_data  = "notBreaching"

  metric_query {
    id          = "error_rate"
    expression  = "(errors / requests) * 100"
    label       = "5xx Error Rate %"
    return_data = true
  }

  metric_query {
    id = "errors"
    metric {
      metric_name = "HTTPCode_Target_5XX_Count"
      namespace   = "AWS/ApplicationELB"
      period      = 300
      stat        = "Sum"
      dimensions = {
        LoadBalancer = aws_lb.main.arn_suffix
      }
    }
  }

  metric_query {
    id = "requests"
    metric {
      metric_name = "RequestCount"
      namespace   = "AWS/ApplicationELB"
      period      = 300
      stat        = "Sum"
      dimensions = {
        LoadBalancer = aws_lb.main.arn_suffix
      }
    }
  }

  alarm_actions = [aws_sns_topic.alerts.arn]
  ok_actions    = [aws_sns_topic.alerts.arn]
}

# Alarm 2: API p99 latency > 2 seconds.
# Why: users feel latency above 1s; 2s means something is seriously wrong.
resource "aws_cloudwatch_metric_alarm" "alb_latency" {
  alarm_name          = "beatstream-alb-p99-latency-high"
  alarm_description   = "API p99 response time exceeds 2 seconds"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  metric_name         = "TargetResponseTime"
  namespace           = "AWS/ApplicationELB"
  period              = 60
  extended_statistic  = "p99"
  threshold           = 2.0
  treat_missing_data  = "notBreaching"

  dimensions = {
    LoadBalancer = aws_lb.main.arn_suffix
  }

  alarm_actions = [aws_sns_topic.alerts.arn]
  ok_actions    = [aws_sns_topic.alerts.arn]
}

# Alarm 3: ECS API running tasks < desired count (tasks are crashing).
# Why: if tasks keep dying, users get 503 from ALB.
resource "aws_cloudwatch_metric_alarm" "ecs_running_tasks" {
  alarm_name          = "beatstream-ecs-tasks-low"
  alarm_description   = "ECS API has fewer running tasks than desired - tasks may be crashing"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = 2
  metric_name         = "RunningTaskCount"
  namespace           = "ECS/ContainerInsights"
  period              = 60
  statistic           = "Average"
  threshold           = 2
  treat_missing_data  = "breaching"

  dimensions = {
    ClusterName = aws_ecs_cluster.main.name
    ServiceName = aws_ecs_service.api.name
  }

  alarm_actions = [aws_sns_topic.alerts.arn]
}

# Alarm 4: Aurora CPU > 90% (database is the bottleneck).
resource "aws_cloudwatch_metric_alarm" "rds_cpu" {
  alarm_name          = "beatstream-rds-cpu-high"
  alarm_description   = "Aurora PostgreSQL CPU exceeds 90% - queries may be slow"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = 300
  statistic           = "Average"
  threshold           = 90
  treat_missing_data  = "notBreaching"

  dimensions = {
    DBClusterIdentifier = aws_rds_cluster.main.cluster_identifier
  }

  alarm_actions = [aws_sns_topic.alerts.arn]
}

# Alarm 5: Redis evictions > 0 (cache is full, data is being dropped).
resource "aws_cloudwatch_metric_alarm" "redis_evictions" {
  alarm_name          = "beatstream-redis-evictions"
  alarm_description   = "Redis is evicting keys - cache is full, consider upgrading node type"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "Evictions"
  namespace           = "AWS/ElastiCache"
  period              = 300
  statistic           = "Sum"
  threshold           = 0
  treat_missing_data  = "notBreaching"

  dimensions = {
    CacheClusterId = aws_elasticache_cluster.redis.cluster_id
  }

  alarm_actions = [aws_sns_topic.alerts.arn]
}
