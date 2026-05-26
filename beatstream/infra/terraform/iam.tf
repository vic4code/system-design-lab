data "aws_iam_policy_document" "ecs_assume_role" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# ECS Task Execution Role — used by the ECS agent to pull images and ship logs.
# Does NOT run inside the container — that's the task role below.
resource "aws_iam_role" "ecs_execution" {
  name               = "beatstream-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

resource "aws_iam_role_policy_attachment" "ecs_execution_managed" {
  role       = aws_iam_role.ecs_execution.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# Extra permission: read SSM SecureString parameters (DB password, Kafka brokers).
resource "aws_iam_role_policy" "ecs_execution_ssm" {
  name = "beatstream-ssm-read"
  role = aws_iam_role.ecs_execution.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = ["ssm:GetParameter", "ssm:GetParameters"]
      Resource = "arn:aws:ssm:${var.aws_region}:${data.aws_caller_identity.current.account_id}:parameter/beatstream/*"
    }]
  })
}

# ECS Task Role — permissions available inside the running container.
resource "aws_iam_role" "ecs_task" {
  name               = "beatstream-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume_role.json
}

resource "aws_iam_role_policy" "ecs_task" {
  name = "beatstream-task-permissions"
  role = aws_iam_role.ecs_task.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        # S3: read/write audio files
        Effect = "Allow"
        Action = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = "${aws_s3_bucket.audio.arn}/*"
      },
      {
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = aws_s3_bucket.audio.arn
      },
      {
        # MSK Serverless IAM auth: sign and connect
        Effect   = "Allow"
        Action   = ["kafka-cluster:Connect", "kafka-cluster:AlterCluster", "kafka-cluster:DescribeCluster"]
        Resource = aws_msk_serverless_cluster.main.arn
      },
      {
        Effect = "Allow"
        Action = [
          "kafka-cluster:*Topic*",
          "kafka-cluster:WriteData",
          "kafka-cluster:ReadData",
          "kafka-cluster:AlterGroup",
          "kafka-cluster:DescribeGroup",
        ]
        Resource = "*"
      },
      {
        Effect   = "Allow"
        Action   = ["kafka:GetBootstrapBrokers", "kafka:DescribeClusterV2"]
        Resource = aws_msk_serverless_cluster.main.arn
      },
    ]
  })
}
