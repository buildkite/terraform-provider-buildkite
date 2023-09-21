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
(thanks to protocol v6) and has validation on its inner attributes.

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

~> The upgrade for a pipeline's `provider_settings` as part of 1.0 from a block to nested attribute is designed to be a forward moving event. However, if you require moving back to using a Buildkite Terraform provider version earlier than 1.0 after upgrading your pipeline resources with nested attributed `provider_settings`, various options exist. If you have access to state files, you can roll back to a backup managing pipelines using the older block `provider_settings` (with `"schema_version": 0`), converting each pipeline resource's `provider_settings` configuration back to a block and planning against the older provider version. Alternatively, you can remove the pipeline from state and re-import it once the provider downgrade has occurred.

### Consistent IDs across resources
All resources now use their GraphQL IDs as the primary ID in the schema.

## Removed Features

- `team` attribute on pipeline resource has been removed.

## Upgrade Guide

~> If you are coming from a 0.x release of the provider and using `buildkite_pipeline.team` attribute on your resources,
you **must** upgrade to version 0.27.0 prior to upgrading to v1.0. See [Migrate pipeline.team usage to pipeline_team
resource](./upgrade-v1#migrate-pipelineteam-usage-to-pipeline_team-resource) for more info.

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

### Upgrade provider

After pinning the provider version, you can upgrade it by running: `terraform init -upgrade`. This will pull in the
latest release under the 1.x version.

### Refresh state

The next step is to refresh your state file: `terraform refresh`.

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

After applying that change to all `pipeline` resources with a `team` attribute, you can run an apply: `terraform apply`.

This will show something like below. You can see in the plan that it will temporarily remove the team from the pipeline,
then followup with adding it back through the separate resource. This will result in a very small time window where the
team doesn't have access to the pipeline. It should be unnoticeable.

```
Terraform used the selected providers to generate the following execution plan. Resource actions are indicated with the following symbols:
  + create
  ~ update in-place

Terraform will perform the following actions:

  # buildkite_pipeline.pipeline will be updated in-place
  ~ resource "buildkite_pipeline" "pipeline" {
        id                         = "<obfuscated>"
        name                       = "v0.27"
        tags                       = []
        # (10 unchanged attributes hidden)

      - team {
          - access_level     = "READ_ONLY" -> null
          - pipeline_team_id = "<obfuscated>" -> null
          - slug             = "team" -> null
          - team_id          = "<obfuscated>" -> null
        }
    }

  # buildkite_pipeline_team.p_team will be created
  + resource "buildkite_pipeline_team" "p_team" {
      + access_level = "READ_ONLY"
      + id           = (known after apply)
      + pipeline_id  = "<obfuscated>"
      + team_id      = "<obfuscated>"
      + uuid         = (known after apply)
    }

Plan: 1 to add, 1 to change, 0 to destroy.
```
