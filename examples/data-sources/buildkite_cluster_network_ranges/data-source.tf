data "buildkite_cluster" "hosted" {
  name = "Hosted"
}

data "buildkite_cluster_network_ranges" "hosted" {
  cluster_uuid = data.buildkite_cluster.hosted.uuid
}

# Create an AWS security group allowing egress from the cluster's hosted agents
resource "aws_security_group" "from_buildkite" {
  name = "from_buildkite"

  ingress {
    from_port   = "443"
    to_port     = "443"
    protocol    = "tcp"
    cidr_blocks = [for r in data.buildkite_cluster_network_ranges.hosted.ranges : r.cidr_range]
  }
}

# Ranges grouped by kind, for when you only want to allow one kind through
output "buildkite_egress_cidrs_by_kind" {
  value = {
    for r in data.buildkite_cluster_network_ranges.hosted.ranges :
    r.kind => r.cidr_range...
  }
}
