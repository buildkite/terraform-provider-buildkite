# Data Source: team

Use this data source to look up properties of a team. This can be used to
validate that a team exists before setting the team slug on a pipeline.

Buildkite documentation: https://buildkite.com/docs/pipelines/permissions

## Example Usage

```hcl
data "buildkite_team" "my_team_data" {
    id = "<team id>"
}
```

The above will automatically fill the ID attribute with that of the team created by the resource, then fill the other
attributes of that team.

The datasource can also reference an imported Team. For more on using Import check
[here](https://github.com/buildkite/terraform-provider-buildkite/blob/main/docs/resources/team.md#import).

## Argument Reference

One of:
* `id` - The GraphQL ID of the team, available in the Settings page for the team.
* `slug` - The slug of the team. Available in the URL of the team on buildkite.com; in the format
  "<organizaton/team-name>"

The `team` data-source supports **either** the use of `id` or `slug` for lookup of a team.

## Attribute Reference

* `id` - The GraphQL ID of the team
* `uuid` - The UUID of the team
* `name` - The name of the team
* `privacy` - Whether the team is visible to org members outside this team
* `description` - A description of the team
* `default_team` - Whether new org members will be automatically added to this team
* `default_member_role` - Default role to assign to a team member
* `members_can_create_pipelines` - Whether team members can create new pipelines and add them to the team
