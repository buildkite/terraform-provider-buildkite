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
* `token` - The value of the created agent token.
* `uuid` - The UUID of the token.


## Import

Tokens can be imported using the `GraphQL ID` (not UUID), e.g.

```
$ terraform import buildkite_agent_token.fleet QWdlbnRUb2tlbi0tLTQzNWNhZDU4LWU4MWQtNDVhZi04NjM3LWIxY2Y4MDcwMjM4ZA==
```

To find the ID to use, you can use the GraphQL query below. Alternatively, you use this [pre-saved query](https://buildkite.com/user/graphql/console/747fb309-e2f3-452a-aea3-ee3962a7e92b).

```
query agentToken {
  organization(slug: "ORGANIZATION_SLUG") {
    agentTokens(first: 20) {
      edges {
        node {
          id
          description
        }
      }
    }
  }
}
```
