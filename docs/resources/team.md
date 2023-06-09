# Resource: team

This resource allows you to create and manage teams.

Buildkite Documentation: https://buildkite.com/docs/pipelines/permissions

Note: You must first enable Teams on your organization.

## Example Usage

```hcl
resource "buildkite_team" "team" {
  name = "developers"

  privacy = "VISIBLE"

  default_team        = true
  default_member_role = "MEMBER"
}
```

## Argument Reference

* `name` - (Required) The name of the team.
* `privacy` - (Required) The privacy level to set the team too.
* `default_team` - (Required) Whether to assign this team to a user by default.
* `default_member_role` - (Required) Default role to assign to a team member.
* `members_can_create_pipelines` - (Optional) Whether team members can create.
* `description` - (Optional) The description to assign to the team.

## Attribute Reference

* `id`   - The GraphQL ID of the team.
* `slug` - The name of the team.
* `uuid` - The UUID for the team.

## Import

Teams can be imported using the `GraphQL ID` (not UUID), e.g.

```
$ terraform import buildkite_team.fleet UGlwZWxpbmUtLS00MzVjYWQ1OC1lODFkLTQ1YWYtODYzNy1iMWNmODA3MDIzOGQ=
```

To find the ID to use, you can use the GraphQL query below. Alternatively, you could use this [pre-saved query](https://buildkite.com/user/graphql/console/9cfcca03-df1a-4951-9fa9-a7345aa3daaf2), where you will need fo fill in the organization slug and search term (TEAM_SEARCH_TERM) for the team.

```graphql
query getTeamId {
  organization(slug: "ORGANIZATION_SLUG") {
    teams(first: 1, search: "TEAM_SEARCH_TERM") {
      edges {
        node {
          id
          name
        }
      }
    }
  }
}
```
