# Resource: team

This resource allows you to create and manage teams.

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
```

## Argument Reference

The following arguments are supported:

* `name` - (Required) The name of the team.
* `privacy` - (Required) The privacy setting for the team, either `VISIBLE` or `SECRET`.
* `default_team` - (Required) A boolean value to control whether to assign this team to a user by default.
* `default_member_role` - (Required) Default role to assign to a team member, either `MEMBER` or `MAINTAINER`.
* `members_can_create_pipelines` - (Optional, Default: `true`) A boolean value to control whether team members can create pipelines.
* `description` - (Optional) The description to assign to the team.

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id`   - The GraphQL ID of the team.
* `slug` - The Buildkite slug of the team.
* `uuid` - The UUID for the team.
