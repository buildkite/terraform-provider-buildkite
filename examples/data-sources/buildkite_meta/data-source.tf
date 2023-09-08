data "buildkite_meta" "ips" {}

# Create an AWS security group allowing ingress from Buildkite
resource "aws_security_group" "from_buildkite" {
  name = "from_buildkite"

  ingress {
    from_port   = "*"
    to_port     = "443"
    protocol    = "tcp"
    cidr_blocks = data.buildkite_meta.ips.webhook_ips
  }
}
