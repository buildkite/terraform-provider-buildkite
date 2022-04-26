# Data Source: team

Use this data source to look up properties of a team. This can be used to
validate that a team exists before setting the team slug on a pipeline.

Buildkite documentation: https://buildkite.com/docs/pipelines/permissions

## Example Usage

```hcl
data "buildkite_team" "my_team" {
    slug = "my_team"
}
```

## Argument Reference

The following arguments are supported:

* `slug` - (Required) The slug of the team, available in the URL of the team on buildkite.com

## Attributes Reference

In addition to all arguments above, the following attributes are exported:

* `id` - The GraphQL ID of the team.
* `uuid` - The UUID of the team.
* `name` - The name of the team.
* `privacy` - Whether the team is visible to org members outside this team.
* `description` - The description of the team.
* `default_team` - Whether new org members will be automatically added to this team.
* `default_member_role` - Default role to assign to a team member.
* `members_can_create_pipelines` - Whether team members can create new pipelines and add them to the team.
