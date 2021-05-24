# Resource: team

This resource allows you to create and manage teams.

Buildkite Documentation: https://buildkite.com/docs/pipelines/permissions

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
