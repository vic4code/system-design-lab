# ECS Service Auto Scaling — scales API tasks based on CPU utilization.
# Interview question: "What happens when traffic spikes 10x?"
# Answer: ALB distributes load → CPU rises → Auto Scaling adds tasks (up to 10) → more capacity.

resource "aws_appautoscaling_target" "api" {
  max_capacity       = 10
  min_capacity       = 2
  resource_id        = "service/${aws_ecs_cluster.main.name}/${aws_ecs_service.api.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  service_namespace  = "ecs"
}

# Scale out when average CPU > 70%.
resource "aws_appautoscaling_policy" "api_cpu" {
  name               = "beatstream-api-cpu"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.api.resource_id
  scalable_dimension = aws_appautoscaling_target.api.scalable_dimension
  service_namespace  = aws_appautoscaling_target.api.service_namespace

  target_tracking_scaling_policy_configuration {
    target_value       = 70.0
    scale_in_cooldown  = 300 # wait 5 min before scaling in (avoid flapping)
    scale_out_cooldown = 60  # scale out fast (1 min)
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
  }
}

# Scale out when average memory > 80%.
resource "aws_appautoscaling_policy" "api_memory" {
  name               = "beatstream-api-memory"
  policy_type        = "TargetTrackingScaling"
  resource_id        = aws_appautoscaling_target.api.resource_id
  scalable_dimension = aws_appautoscaling_target.api.scalable_dimension
  service_namespace  = aws_appautoscaling_target.api.service_namespace

  target_tracking_scaling_policy_configuration {
    target_value       = 80.0
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageMemoryUtilization"
    }
  }
}
