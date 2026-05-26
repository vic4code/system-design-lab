resource "aws_elasticache_subnet_group" "main" {
  name       = "beatstream"
  subnet_ids = aws_subnet.private[*].id
}

# Single-node Redis for the lab. For production: use a replication group
# with at least one replica for HA (aws_elasticache_replication_group).
resource "aws_elasticache_cluster" "redis" {
  cluster_id           = "beatstream"
  engine               = "redis"
  node_type            = "cache.t4g.micro" # ~$12/mo — smallest Graviton instance
  num_cache_nodes      = 1
  engine_version       = "7.1"
  subnet_group_name    = aws_elasticache_subnet_group.main.name
  security_group_ids   = [aws_security_group.redis.id]
  parameter_group_name = "default.redis7"
}
