# Resource: test_suite_team

This resources allows you to create, manage and import team access to Test Suites.

Buildkite documentation: https://buildkite.com/docs/test-analytics

## Example Usage

```hcl
provider "buildkite" {}

resource "buildkite_team" "owners" {
	name = "Owning team"
	default_team = false
	privacy = "VISIBLE"
	default_member_role = "MAINTAINER"
}

resource "buildkite_team" "viewers" {
	name = "Viewers team"
	default_team = false
	privacy = "VISIBLE"
	default_member_role = "MAINTAINER"
}

resource "buildkite_test_suite" "rspec_suite" {
	name = "RSpec Test Suite"
	default_branch = "main"
	team_owner_id = buildkite_team.owners.id
}

resource "buildkite_test_suite_team" "viewers_rspec" {
	test_suite_id = buildkite_test_suite.rspec_suite.id
	team_id = buildkite_team.viewers.id
	access_level = "READ_ONLY"
}
```

## Argument Reference

* `test_suite_id` - (Required) The GraphQL ID of the test suite.
* `team_id` - (Required) The GraphQL ID of the team.
* `access_level` - (Required) The access level the team has on the test suite, Either READ_ONLY or MANAGE_AND_READ.

## Attribute Reference

* `id` - This is the GraphQL ID of the test suite team.
* `uuid` - This is the UUID of the test suite team.

## Import

Test suite teams can be imported using the `GraphQL ID` (not UUID), e.g.

```
$ terraform import buildkite_test_suite_team.viewers VGVhbvDf4eRef20tMzIxMGEfYTctNzEF5g00M8f5s6E2YjYtODNlOGNlZgD6HcBi
```

To find the ID to use, you can use the GraphQL query below. Alternatively, you could use this [pre-saved query](https://buildkite.com/user/graphql/console/e8480014-37a8-4150-a011-6d35f33b4dfd), where you will need fo fill in the organization slug and suite search term (SUITE_SEARCH_TERM) for the particular test suite required.

```graphql
query getTeamSuiteIds {
  organization(slug: "ORGANIZATION_SLUG") {
    suites(first: 1, search:"SUITE_SEARCH_TERM") {
      edges {
        node {
          id
          name
          teams(first: 10){
            edges {
              node {
                id
                accessLevel
                team{
                  name
                }
              }
            }
          }
        }
      }
    }
  }
}
```
