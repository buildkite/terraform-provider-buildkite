---
page_title: Upgrading to v1.0
---

# Upgrading to v1.0

The Buildkite Terraform provider 1.0 is considered stable and ready for production use. If you have been using the
provider prior to the 1.0 release, this guides you through upgrading.  
If you are starting a new terraform project with this provider, you can start at https://registry.terraform.io/providers/buildkite/buildkite/latest/docs.

## New Features

### Protocol version 6
The provider has been upgraded to protocol v6. It is therefor only compatible with terraform CLI version 1.0 and later.

### Cluster resources
You are now able to manage cluster resources with the provider. This includes [`buildkite_cluster`](../resources/cluster)., [`buildkite_cluster_queue`](../resources/cluster_queue), and
[`buildkite_cluster_agent_token`](../resources/cluster_agent_token).

### Test Analytics resources
You can now create test suites and assign teams access to them with [`buildkite_test_suite`](../resources/test_suite), and [`buildkite_test_suite_team`](../resources/test_suite_team).

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

### Consistent IDs across resources
All resources now use their GraphQL IDs as the primary ID in the schema.

## Removed Features

- `team` attribute on pipeline resource has been removed. It is replaced by the separate resource [`buildkite_pipeline_team`](../resources/pipeline_team).

## Upgrade Guide

~> If you are coming from a 0.x release of the provider and using `buildkite_pipeline.team` attribute on your resources,
you **must** upgrade to version 0.27.0 with the newer resource **using an Administrator scoped API Access Token** prior to upgrading to v1.0. 
See [Migrate pipeline.team usage to buildkite_pipeline_team resource](./upgrade-v1#migrate-pipelineteam-usage-to-buildkite-pipeline_team-resource) for more info.

### Backup the state file

State file backups are created automatically by terraform. You should inspect your state storage location and ensure
there is a valid backup available in the event of corruption from upgrading the provider.

Refer to https://developer.hashicorp.com/terraform/cli/state/recover for more information.

### Pin provider version

You should pin your provider installation to the 1.x releases so you can upgrade as new versions are released.

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

### Migrate `pipeline.team` usage to `buildkite_pipeline_team` resource

The `team` attribute on the `pipeline` resource was removed in v1.0 in favour of a separate resource:
[`buildkite_pipeline_team`](../resources/pipeline_team).

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

### Migrate `pipeline.provider_settings` block to a nested attribute

This is as simple as adding an equal sign (`=`) to the `provider_settings` attribute and running `terraform apply`.

The provider will transparently update the state file to the new schema version. This operation is not automatically
reversible. If you run into issues from upgrading, please raise an issue on GitHub.

See [Pipeline resource `provider_settings` type change](./upgrade-v1#pipeline-resource-provider_settings-type-change) for an example.

#### Rolling back

If you experience issues with the automatic upgrade you can revert your changes and re-instate the backup terraform
state file. Follow the instructions from Terraform on disaster recovery: https://developer.hashicorp.com/terraform/cli/state/recover.
