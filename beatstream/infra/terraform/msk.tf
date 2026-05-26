resource "aws_msk_serverless_cluster" "main" {
  cluster_name = "beatstream"

  vpc_config {
    subnet_ids         = aws_subnet.private[*].id
    security_group_ids = [aws_security_group.msk.id]
  }

  # IAM auth only — no static credentials, no plaintext.
  # ECS task role is granted kafka-cluster:* permissions in iam.tf.
  client_authentication {
    sasl {
      iam { enabled = true }
    }
  }
}

# MSK Serverless does not expose bootstrap brokers as a Terraform attribute.
# This null_resource runs `aws kafka get-bootstrap-brokers` after cluster creation
# and stores the result in SSM so ECS tasks can read it at startup.
resource "null_resource" "fetch_kafka_brokers" {
  depends_on = [aws_msk_serverless_cluster.main, aws_ssm_parameter.kafka_brokers]

  triggers = {
    cluster_arn = aws_msk_serverless_cluster.main.arn
  }

  provisioner "local-exec" {
    command = <<-EOT
      BROKERS=$(aws kafka get-bootstrap-brokers \
        --cluster-arn ${aws_msk_serverless_cluster.main.arn} \
        --query BootstrapBrokerStringSaslIam \
        --output text \
        --region ${var.aws_region})
      aws ssm put-parameter \
        --name /beatstream/kafka_brokers \
        --value "$BROKERS" \
        --overwrite \
        --region ${var.aws_region}
      echo "Stored MSK bootstrap brokers in SSM."
    EOT
  }
}

# Placeholder — updated by null_resource above after MSK is ready.
resource "aws_ssm_parameter" "kafka_brokers" {
  name  = "/beatstream/kafka_brokers"
  type  = "String"
  value = "placeholder"

  # Terraform must not overwrite the real value written by null_resource on subsequent applies.
  lifecycle { ignore_changes = [value] }
}
