# Data Source: meta

Use this data source to look up the source IP addresses that Buildkite may use
to send external requests, including webhooks and API calls to source control
systems (like GitHub Enterprise Server and BitBucket Server).

Buildkite Documentation: https://buildkite.com/docs/apis/rest-api/meta

## Example Usage

This is intended to be used in other terraform projects to dynamically
set firewall and ingress rules to allow traffic from Buildkite. For
customers that use AWS, that might look like this:

```hcl
data "buildkite_meta" "ips" { }

resource "aws_security_group" "from_buildkite" {
    name = "from_buildkite"

    ingress {
        from_port        = "*"
        to_port          = "443"
        protocol         = "tcp"
        cidr_blocks      = data.buildkite_meta.ips.webhook_ips
    }
}
```

## Argument Reference

None.

## Attributes Reference

* `webhook_ips` - A list of strings, each one an IP address (x.x.x.x) or CIDR address (x.x.x.x/32) that Buildkite may use to send webhooks and other external requests.
