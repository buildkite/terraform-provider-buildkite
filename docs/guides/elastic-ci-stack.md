---
page _title: Usage with Elastic CI Stack for AWS
---

# Usage with the Elastic CI Stack for AWS

There is not an official Terraform module for deploying Buildkite agents to AWS. However, there is a CloudFormation
stack available: https://github.com/buildkite/elastic-ci-stack-for-aws.

If you want to manage all your infrastructure through Terraform, you can utilize the
[`aws`](https://registry.terraform.io/providers/hashicorp/aws/latest) provider to create a CloudFormation stack using
our template. An example is given below:

For more information on the Elastic CI Stack for AWS, visit https://buildkite.com/docs/agent/v3/elastic-ci-aws/elastic-ci-stack-overview.

```tf
resource "buildkite_agent_token" "elastic_stack" {
  description = "Elastic stack"
}

resource "aws_cloudformation_stack" "buildkite_agent_default" {
  name         = "buildkite-agent-default"
  template_url = "https://s3.amazonaws.com/buildkite-aws-stack/master/aws-stack.yml"
  capabilities = ["CAPABILITY_NAMED_IAM"]
  parameters = {
    AgentsPerInstance        = 5
    AssociatePublicIpAddress = true
    InstanceType             = "i3.2xlarge"
    MaxSize                  = 10
    MinSize                  = 0
    RootVolumeSize           = 50
    BuildkiteAgentToken      = buildkite_agent_token.elastic_stack.token
  }
}
```
