# Resource: test_suite

This resources allows you to create and manage a Test Suite.

Buildkite documentation: https://buildkite.com/docs/test-analytics

## Example Usage

```hcl
provider "buildkite" {}

resource "buildkite_team" "test" {
  name = "Manage unit tests"

  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}

resource "buildkite_test_suite" "unit_tests" {
  name = "Unit tests"
  default_branch = "main"
  team_owner_id = buildkite_team.test.id
}
```

## Argument Reference

* `name` - (Required) This is the name of the test suite.
* `default_branch` - (Required) This is the default branch used to compare tests against.
* `team_owner_id` - (Required) This is a single team linked to the test suite upon creation.

## Attribute Reference

* `api_token` - This is the unique API token used when send test results.
* `id` - This is the GraphQL ID of the suite.
* `uuid` - This is the UUID of the suite.
* `slug` - This is the unique slug generated from the name upon creation.
