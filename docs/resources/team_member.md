# Resource: team_member

This resource allows manage team membership for existing organization users.

The user must already be part of the organization to which you are managing team membership. This will not invite a new user to the organization.

Buildkite Documentation: https://buildkite.com/docs/pipelines/permissions

Note: You must first enable Teams on your organization.

## Example Usage

```hcl
resource "buildkite_team" "team" {
    name = "developers"

    privacy = "VISIBLE"

    default_team = true
    default_member_role = "MEMBER"
}

resource "buildkite_team_member" "a_smith" {
    role = "MEMBER"
    team_id = buildkite_team.team.id
    user_id = "VXNlci0tLWRlOTdmMjBiLWJkZmMtNGNjOC1hOTcwLTY1ODNiZTk2ZGEyYQ=="
}
```

## Argument Reference

* `role` - (Required) Either MEMBER or MAINTAINER.
* `team_id` - (Required) The GraphQL ID of the team to add to/remove from.
* `user_id` - (Required) The GraphQL ID of the user to add/remove.

## Attribute Reference

* `id`   - The GraphQL ID of the team membership.
* `uuid` - The UUID for the team membership.

## Import

Team members can be imported using the GraphQL ID of the membership. Note this is different to the ID of the user.

```
$ terraform import buildkite_team_member.a_smith VGVhbU1lbWJlci0tLTVlZDEyMmY2LTM2NjQtNDI1MS04YzMwLTc4NjRiMDdiZDQ4Zg==
```

To find the ID of a team member you are trying to import you can use the GraphQL snippet below. A link to this snippet can also be found at https://buildkite.com/user/graphql/console/c6a2cc65-dc59-49df-95c6-7167b68dbd5d.

You will need fo fill in the organization slug and search terms for teams and members. Both search terms work on the name of the associated object.

```graphql
query {
  organization(slug: "") {
    teams(first: 2, search: "") {
      edges {
        node {
          members(first: 2, search: "") {
            edges {
              node {
                id
              }
            }
          }
        }
      }
    }
  }
}
```
