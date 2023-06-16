# Resource: agent_token

This resource allows you to create and manage agent tokens.

Buildkite Documentation: https://buildkite.com/docs/agent/v3/tokens

## Example Usage

```hcl
provider "buildkite" {}

resource "buildkite_agent_token" "fleet" {
  description = "token used by build fleet"
}
```

## Argument Reference

* `description` - (Optional) This is the description of the agent token.

-> Changing `description` will cause the resource to be destroyed and re-created.

## Attribute Reference

* `id` - The Graphql ID of the created agent token.
* `uuid` - The UUID of the token.

