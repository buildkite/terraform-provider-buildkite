provider "buildkite" {
  timeouts {
    create = "10s"
    update = "15s"
  }
}

resource "buildkite_team" "test" {
  name = "terraform_provider_test"

  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}

resource "buildkite_team" "testtwo" {
  name = "terraform_provider_test_two"

  privacy             = "VISIBLE"
  default_team        = false
  default_member_role = "MEMBER"
}

resource "buildkite_team_member" "member1" {
  role    = "MEMBER"
  team_id = "VXfhnVUS78HavgtP55WhWGzT401guK38Vm9LMMeCgQD124m8xaKBRq0Fth=="
  user_id = "VXNbwSA9hwVPpMgUXu1dWIDWf45ZwU6J7deETygiLUrKBg2TZBxuDr6aKj=="
}


resource "buildkite_pipeline" "repo2" {
  name       = "terraform_provider_buildkite_pipeline"
  repository = "git@github.com:org/repo2"
  steps      = file("./steps.yml") 

  lifecycle {
    prevent_destroy = true
  }
}

resource "buildkite_pipeline_schedule" "weekly" {
  pipeline_id = buildkite_pipeline.repo2
  label       = "Weekly build from default branch"
  cronline    = "@midnight"
  branch      = "default"
  message     = "Weekly scheduled build"
}

resource "buildkite_pipeline_team" "developers" {
  pipeline_id  = buildkite_pipeline.repo2
  team_id      = buildkite_team.test.id
  access_level = "MANAGE_BUILD_AND_READ"
}

data "buildkite_pipeline" "data2" {
  slug = buildkite_pipeline.repo2.slug
}

resource "buildkite_agent_token" "fleet" {
  description = "token used by build fleet"
}

resource "buildkite_cluster" "my_awesome_cluster" {
  name = "best cluster ever"
  description = "This cluster can do it all 😍"
  color = "#BADA55"
  emoji = ":muscle:"
}

resource "buildkite_cluster_queue" "queue1" {
  cluster_id  = "Q2x1c3Rlci0tLTMzMDc0ZDhiLTM4MjctNDRkNC05YTQ3LTkwN2E2NWZjODViNg=="
  key         = "dev"
  description = "Dev cluster queue"
}

resource "buildkite_cluster_agent_token" "token1" {
  cluster_id  = "Q2x1c3Rlci0tLTMzMDc0ZDhiLTM4MjctNDRkNC05YTQ3LTkwN2E2NWZjODViNg=="
  description = "agent token for Dev cluster"
}

resource "buildkite_test_suite" "unit_tests" {
  name           = "Unit tests"
  default_branch = "main"
  team_owner_id  = buildkite_team.test.id
}

resource "buildkite_test_suite_team" "suite_read_only" {
  test_suite_id = buildkite_test_suite.unit_tests.id
  team_id       = buildkite_team.testtwo.id
  access_level  = "READ_ONLY"
}

output "agent_token" {
  value = buildkite_agent_token.fleet.token
}

output "badge_url" {
  value = buildkite_pipeline.test.badge_url
}

