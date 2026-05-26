resource "aws_ecs_cluster" "main" {
  name = "beatstream"
  setting {
    name  = "containerInsights"
    value = "enabled"
  }
}

resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/beatstream/api"
  retention_in_days = 7
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ecs/beatstream/worker"
  retention_in_days = 7
}

resource "aws_ecs_task_definition" "api" {
  family                   = "beatstream-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 512  # 0.5 vCPU
  memory                   = 1024 # 1 GB
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "api"
    image = "${aws_ecr_repository.api.repository_url}:${var.api_image_tag}"

    portMappings = [{ containerPort = 8080, protocol = "tcp" }]

    environment = [
      { name = "PORT",         value = "8080" },
      { name = "METRICS_PORT", value = "9090" },
      { name = "AWS_REGION",   value = var.aws_region },
      { name = "KAFKA_AUTH",   value = "iam" },
      { name = "S3_BUCKET",    value = aws_s3_bucket.audio.bucket },
      { name = "REDIS_URL",    value = "redis://${aws_elasticache_cluster.redis.cache_nodes[0].address}:6379" },
      { name = "DATABASE_URL", value = "postgres://beatstream@${aws_rds_cluster.main.endpoint}:5432/beatstream?sslmode=require" },
    ]

    # Secrets are fetched from SSM at container startup — never stored in image or env.
    secrets = [
      { name = "DB_PASSWORD",   valueFrom = aws_ssm_parameter.db_password.arn },
      { name = "KAFKA_BROKERS", valueFrom = aws_ssm_parameter.kafka_brokers.arn },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.api.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "api"
      }
    }

    healthCheck = {
      command     = ["CMD-SHELL", "wget -qO- http://localhost:8080/health || exit 1"]
      interval    = 30
      timeout     = 5
      retries     = 3
      startPeriod = 60
    }
  }])
}

resource "aws_ecs_service" "api" {
  name            = "beatstream-api"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = 2 # one per AZ for HA
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.api.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  # Rolls back automatically if the new task fails health checks.
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  depends_on = [
    aws_lb_listener.http,
    aws_iam_role_policy_attachment.ecs_execution_managed,
    null_resource.fetch_kafka_brokers,
  ]
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "beatstream-worker"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = 256 # 0.25 vCPU — transcoding is simulated, not real
  memory                   = 512
  execution_role_arn       = aws_iam_role.ecs_execution.arn
  task_role_arn            = aws_iam_role.ecs_task.arn

  container_definitions = jsonencode([{
    name  = "worker"
    image = "${aws_ecr_repository.worker.repository_url}:${var.worker_image_tag}"

    environment = [
      { name = "AWS_REGION",   value = var.aws_region },
      { name = "KAFKA_AUTH",   value = "iam" },
      { name = "S3_BUCKET",    value = aws_s3_bucket.audio.bucket },
      { name = "DATABASE_URL", value = "postgres://beatstream@${aws_rds_cluster.main.endpoint}:5432/beatstream?sslmode=require" },
    ]

    secrets = [
      { name = "DB_PASSWORD",   valueFrom = aws_ssm_parameter.db_password.arn },
      { name = "KAFKA_BROKERS", valueFrom = aws_ssm_parameter.kafka_brokers.arn },
    ]

    logConfiguration = {
      logDriver = "awslogs"
      options = {
        "awslogs-group"         = aws_cloudwatch_log_group.worker.name
        "awslogs-region"        = var.aws_region
        "awslogs-stream-prefix" = "worker"
      }
    }
  }])
}

resource "aws_ecs_service" "worker" {
  name            = "beatstream-worker"
  cluster         = aws_ecs_cluster.main.id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = 1
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.worker.id]
    assign_public_ip = false
  }

  depends_on = [
    aws_iam_role_policy_attachment.ecs_execution_managed,
    null_resource.fetch_kafka_brokers,
  ]
}
