resource "aws_db_subnet_group" "main" {
  name       = "beatstream"
  subnet_ids = aws_subnet.private[*].id
}

# Aurora PostgreSQL Serverless v2.
# Scales between 0.5–4 ACUs automatically based on load.
# 0.5 ACU ≈ 1 GB RAM — enough for a lab that's idle most of the time.
# Trade-off: cold start when scaling from minimum adds ~1s of latency on first connection burst.
resource "aws_rds_cluster" "main" {
  cluster_identifier     = "beatstream"
  engine                 = "aurora-postgresql"
  engine_mode            = "provisioned"
  engine_version         = "15.4"
  database_name          = "beatstream"
  master_username        = "beatstream"
  master_password        = var.db_password
  db_subnet_group_name   = aws_db_subnet_group.main.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  skip_final_snapshot    = true # change to false before real production use

  serverlessv2_scaling_configuration {
    min_capacity = 0.5
    max_capacity = 4.0
  }
}

resource "aws_rds_cluster_instance" "main" {
  identifier         = "beatstream-1"
  cluster_identifier = aws_rds_cluster.main.id
  instance_class     = "db.serverless"
  engine             = aws_rds_cluster.main.engine
  engine_version     = aws_rds_cluster.main.engine_version
}

# DB password in SSM — ECS execution role reads it at container startup.
resource "aws_ssm_parameter" "db_password" {
  name  = "/beatstream/db_password"
  type  = "SecureString"
  value = var.db_password
}
