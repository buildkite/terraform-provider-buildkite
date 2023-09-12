---
page_title: Upgrading to v1.0
---

# Upgrading to v1.0

The Buildkite Terraform provider 1.0 is considered stable and ready for production use. If you have been using the
provider prior to the 1.0 release, this guides you through upgrading.

## New Features

### Protocol version 6
The provider has been upgraded to protocol v6. It is therefor only compatible with terraform CLI version 1.0 and later.

### Cluster resources
You are now able to manage cluster resources with the provider. This includes `cluster`, `cluster_queue`, and
`cluster_agent_token`.

### Test Analytics resources
You can now create test suites and assign teams access to them with `test_suite`, and `test_suite_team`.

### Configurable API retry timeouts
API retries and timeouts have been implemented in the majority of resources. This adds reliability to the provider
around API latency and outages.

By default, retries are set to 30 seconds. You can override this by operation type on the provider settings:

```tf
provider "buildkite" {
  organization = "buildkite"
  timeouts = {
    create = "90s"
    delete = "10s"
  }
}
```

### Archive pipeline on delete
Pipeline resources can now be archived instead of deleted when the resource is destroyed. This might be useful for users
who wish to keep the history around but disable the pipeline. This is configurable at the provider level:

```tf
provider "buildkite" {
  organization = "buildkite"
  archive_pipeline_on_delete = true
}
```

## Changed Features

### Pipeline resource `provider_settings` type change
The `provider_settings` attribute on the pipeline resource has been completely overhauled. It is now a nested attribute
(thanks to protocol v6) and has validation on inner attributes.

Previously:

```tf
resource "buildkite_pipeline" "pipeline" {
  name = "deploy"
  repository = "https://github.com/company/project.git"
  provider_settings {
    trigger_mode = "deployment"
  }
}
```

Now, `provider_settings` is a nested attribute:

```tf
resource "buildkite_pipeline" "pipeline" {
  name = "deploy"
  repository = "https://github.com/company/project.git"
  provider_settings = {
    trigger_mode = "deployment"
  }
}
```

### Consistent IDs across resources
All resources now use their GraphQL IDs as the primary ID in the schema.

## Removed Features

- `team` attribute on pipeline resource has been removed.

## Upgrade Guide

### Pin provider version

You should pin your provider installation to the 1.x releases.

```tf
terraform {
  required_providers {
    buildkite = {
      source  = "buildkite/buildkite"
      version = "~> 1.0"
    }
  }
}
```

### Migrate `pipeline.team` usage to `pipeline_team` resource

The `team` attribute on the `pipeline` resource was removed in v1.0 in favour of a separate resource:
[`pipeline_team`](../resources/pipeline_team).

You'll need to upgrade your provider to version `0.27.0` and switch over to the new resource prior to upgrading to v1.0.

Before this change:

```tf
resource "buildkite_team" "deploy" {
  name = "deploy"
  privacy = "VISIBLE"
  default_team = true
  default_member_role = "MEMBER"
}

# only allow the deploy team to build the deploy pipeline
resource "buildkite_pipeline" "pipeline" {
  name = "deploy"
  repository = "https://github.com/company/project.git"
  team {
    slug = "deploy"
    access_level = "BUILD_AND_READ"
  }
}
```

After this change:

```tf
resource "buildkite_team" "deploy" {
  name = "deploy"
  privacy = "VISIBLE"
  default_team = true
  default_member_role = "MEMBER"
}

resource "buildkite_pipeline" "pipeline" {
  name = "deploy"
  repository = "https://github.com/company/project.git"
}

# only allow the deploy team to build the deploy pipeline
resource "buildkite_pipeline_team" "deploy_pipeline" {
  pipeline_id = buildkite_pipeline.pipeline.id
  team_id = buildkite_team.deploy.id
  access_level = "BUILD_AND_READ"
}
```
