# Data Source: organization

Use this data source to look up the organization settings. It currently supports
allowed_api_ip_addresses.

## Example Usage

```hcl
data "buildkite_organization" "testkite" { }

resource "aws_security_group" "from_buildkite" {
  name = "from_buildkite"

  ingress {
    from_port   = "*"
    to_port     = "443"
    protocol    = "tcp"
    cidr_blocks = data.buildkite_organization.allowed_api_ip_addresses
  }
}
```

## Argument Reference

None.

## Attributes Reference

* `allowed_api_ip_addresses` - list of IP addresses in CIDR format that are allowed to access the Buildkite API.
