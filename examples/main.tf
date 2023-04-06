provider "buildkite" {}

resource "buildkite_team" "test" {
    name = "terraform_provider_test"

    privacy = "VISIBLE"
    default_team = false
    default_member_role = "MEMBER"
}

resource "buildkite_pipeline" "repo2" {
    name = "terraform_provider_buildkite_pipeline"
    repository = "git@github.com:org/repo2"
    steps = file("./steps.yml")

    team {
        slug = buildkite_team.test.slug
        access_level = "READ_ONLY"
    }

    deletion_protection = true
}

resource "buildkite_agent_token" "fleet" {
    description = "token used by build fleet"
}

output "agent_token" {
    value = buildkite_agent_token.fleet.token
}

output "badge_url" {
    value = buildkite_pipeline.test.badge_url
}
