# Resource: pipeline

This resource allows you to create and manage pipelines for repositories.

Buildkite Documentation: https://buildkite.com/docs/pipelines

## Example Usage

```hcl
# in ./steps.yml:
# steps:
#   - label: ':pipeline:'
#     command: buildkite-agent pipeline upload

resource "buildkite_pipeline" "repo2" {
  name       = "repo2"
  repository = "git@github.com:org/repo2"
  steps      = file("./steps.yml")
}
```

## Example Usage with command timeouts

```hcl
resource "buildkite_pipeline" "test_new" {
  name       = "Testing Timeouts"
  repository = "https://github.com/buildkite/terraform-provider-buildkite.git"

  steps = file("./deploy-steps.yml")

  default_timeout_in_minutes = 60
  maximum_timeout_in_minutes = 120
}
```

Currently, the `default_timeout_in_minutes` and `maximum_timeout_in_minutes` will be retained in state even if removed from the configuration. In order to remove them, you must set them to `0` in either the configuration or the web UI.

## Example Usage with Lifecycles

```hcl
resource "buildkite_pipeline" "test_new" {
  name       = "Testing Timeouts"
  repository = "https://github.com/buildkite/terraform-provider-buildkite.git"

  steps = file("./deploy-steps.yml")

  lifecycle {
      prevent_destroy = true
  }
}
```

[Lifecycles](https://developer.hashicorp.com/terraform/language/meta-arguments/lifecycle) allow you to set general rules
for resources, including the prevention of destruction of a resource. In the above example, the `destroy` command on the `Testing TImeouts` pipeline will fail.

Lifecycles will replace the deprecated `deletion_protection` in `v1` of the provider in favour of `lifecycle` rules.

#### Deprecation notice
`archive_on_delete` has been deprecated and moved to provider configuration instead. The usage through provider
configuration makes more sense and will apply to all pipelines managed through terraform. Please see `archive_pipeline_on_delete`.

## Example Usage with GitHub Provider Settings

```hcl
# Pipeline that should not be triggered from a GitHub webhook
resource "buildkite_pipeline" "repo2-deploy" {
  name       = "repo2"
  repository = "git@github.com:org/repo2"
  steps      = file("./deploy-steps.yml")

  provider_settings {
    trigger_mode = "none"
  }
}

# Release pipeline (triggered only when tags are pushed)
resource "buildkite_pipeline" "repo2-release" {
  name       = "repo2"
  repository = "git@github.com:org/repo2"
  steps      = file("./release-steps.yml")

  provider_settings {
    build_branches      = false
    build_tags          = true
    build_pull_requests = false
    trigger_mode        = "code"
  }
}
```

## Argument Reference

-   `name` - (Required) The name of the pipeline.
-   `repository` - (Required) The git URL of the repository.
-   `steps` - (Optional) The string YAML steps to run the pipeline. Defaults to `buildkite-agent pipeline upload` if not specified.
-   `description` - (Optional) A description of the pipeline.
-   `default_branch` - (Optional) The default branch to prefill when new builds are created or triggered, usually main or master but can be anything.
-   `default_timeout_in_minutes` - (Optional) The default timeout for commands in this pipeline, in minutes.
-   `maximum_timeout_in_minutes` - (Optional) The maximum timeout for commands in this pipeline, in minutes.
-   `branch_configuration` - (Optional) Limit which branches and tags cause new builds to be created, either via a code push or via the Builds REST API.
-   `skip_intermediate_builds` - (Optional, Default: `false` ) A boolean to enable automatically skipping any unstarted builds on the same branch when a new build is created.
-   `skip_intermediate_builds_branch_filter` - (Optional) Limit which branches build skipping applies to, for example `!master` will ensure that the master branch won't have its builds automatically skipped.
-   `cancel_intermediate_builds` - (Optional, Default: `false` ) A boolean to enable automatically cancelling any running builds on the same branch when a new build is created.
-   `cancel_intermediate_builds_branch_filter` - (Optional) Limit which branches build cancelling applies to, for example !master will ensure that the master branch won't have its builds automatically cancelled.
-   `allow_rebuilds` - (Optional, Default: `true` ) A boolean on whether or not to allow rebuilds for the pipeline.
-   `cluster_id` - (Optional) The GraphQL ID of the cluster you want to use for the pipeline.
-   `team` - (Optional) **DEPRECATED** Set team access for the pipeline. Can be specified multiple times for each team.
-   `tags` - (Optional) A set of tags to be set to the pipeline. For example `["terraform", "provider"]`.
-   `provider_settings` - (Optional) Source control provider settings for the pipeline. See [Provider Settings Configuration](#provider-settings-configuration) below for details.
-   `deletion_protection` - **DEPRECATED** (Optional) Set to either `true` or `false`. When set to `true`, `destroy` actions on a pipeline will be blocked and fail with a message "Deletion protection is enabled for pipeline: <pipeline name>"

### Provider Settings Configuration

-> **Note:** Supported provider settings depend on a source version control provider used by your organization.

Properties available for Bitbucket Server:

-   `build_pull_requests` - (Optional) Whether to create builds for commits that are part of a Pull Request.
-   `build_branches` - (Optional) Whether to create builds when branches are pushed.
-   `build_tags` - (Optional) Whether to create builds when tags are pushed.

Properties available for Bitbucket Cloud, GitHub, and GitHub Enterprise:

-   `build_pull_requests` - (Optional) Whether to create builds for commits that are part of a Pull Request.
-   `build_branches` - (Optional) Whether to create builds when branches are pushed.
-   `build_tags` - (Optional) Whether to create builds when tags are pushed.
-   `filter_enabled` - (Optional) [true/false] Whether to filter builds to only run when the condition in `filter_condition` is true
-   `filter_condition` - (Optional) The condition to evaluate when deciding if a build should run. More details available in [the documentation](https://buildkite.com/docs/pipelines/conditionals#conditionals-in-pipelines)
-   `pull_request_branch_filter_enabled` - (Optional) Whether to limit the creation of builds to specific branches or patterns.
-   `pull_request_branch_filter_configuration` - (Optional) The branch filtering pattern. Only pull requests on branches matching this pattern will cause builds to be created.
-   `skip_builds_for_existing_commits` - (Optional) Whether to skip creating a new build if an existing build for the commit and branch already exists.
-   `skip_pull_request_builds_for_existing_commits` - (Optional) Whether to skip creating a new build for a pull request if an existing build for the commit and branch already exists.
-   `publish_commit_status` - (Optional) Whether to update the status of commits in Bitbucket or GitHub.
-   `publish_commit_status_per_step` - (Optional) Whether to create a separate status for each job in a build, allowing you to see the status of each job directly in Bitbucket or GitHub.
-   `cancel_deleted_branch_builds` - (Optional, Default: `false` ) A boolean to enable automatically cancelling any running builds for a branch if the branch is deleted.

Additional properties available for GitHub:

-   `trigger_mode` - (Optional) What type of event to trigger builds on. Must be one of:

    -   `code` will create builds when code is pushed to GitHub.
    -   `deployment` will create builds when a deployment is created with the GitHub Deployments API.
    -   `fork` will create builds when the GitHub repository is forked.
    -   `none` will not create any builds based on GitHub activity.

-   `build_pull_request_forks` - (Optional) Whether to create builds for pull requests from third-party forks.
-   `build_pull_request_labels_changed` - (Optional) Whether to create builds for pull requests when labels are added or removed.
-   `prefix_pull_request_fork_branch_names` - (Optional) Prefix branch names for third-party fork builds to ensure they don't trigger branch conditions. For example, the `master` branch from `some-user` will become `some-user:master`.
-   `separate_pull_request_statuses` - (Optional) Whether to create a separate status for pull request builds, allowing you to require a passing pull request build in your [required status checks](https://help.github.com/en/articles/enabling-required-status-checks) in GitHub.
-   `publish_blocked_as_pending` - (Optional) The status to use for blocked builds. Pending can be used with [required status checks](https://help.github.com/en/articles/enabling-required-status-checks) to prevent merging pull requests with blocked builds.

## Attribute Reference

-   `id` - The GraphQL ID of the pipeline
-   `webhook_url` - The Buildkite webhook URL to configure on the repository to trigger builds on this pipeline.
-   `slug` - The slug of the created pipeline.
-   `badge_url` - The pipeline's last build status so you can display build status badge.

## Import

Pipelines can be imported using the `GraphQL ID` (not UUID), e.g.

```
$ terraform import buildkite_pipeline.fleet UGlwZWxpbmUtLS00MzVjYWQ1OC1lODFkLTQ1YWYtODYzNy1iMWNmODA3MDIzOGQ=
```

To find the ID to use, you can use the GraphQL query below. Alternatively, you could use this [pre-saved query](https://buildkite.com/user/graphql/console/04e6ac1d-222e-49f9-8167-4767ab0f5362).

```graphql
query getPipelineId {
    pipeline(slug: "ORGANIZATION_SLUG/PIPELINE_SLUG") {
        id
    }
}
```
