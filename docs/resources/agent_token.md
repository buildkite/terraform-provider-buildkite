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

* `token` - The value of the created agent token.
* `uuid` - The UUID of the token.


## Import

Tokens can be imported using the `GraphQL ID` (not UUID), e.g.

```
$ terraform import buildkite_agent_token.fleet QWdlbnRUb2tlbi0tLTQzNWNhZDU4LWU4MWQtNDVhZi04NjM3LWIxY2Y4MDcwMjM4ZA==
```