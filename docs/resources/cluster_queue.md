# Resource: cluster_queue

This resource allows you to create and manage cluster queues.

Buildkite Documentation: https://buildkite.com/docs/clusters/manage-clusters#set-up-clusters-create-a-queue

## Example Usage

```hcl
provider "buildkite" {}

resource "buildkite_cluster_queue" "queue1" {
  cluster_id = "Q2x1c3Rlci0tLTMzMDc0ZDhiLTM4MjctNDRkNC05YTQ3LTkwN2E2NWZjODViNg=="
  key = "prod-deploy"
  description = "Prod deployment cluster queue"
}
```

## Argument Reference

* `cluster_id` - (Required) The ID of the cluster that this cluster queue belongs to.
* `key` - (Required) The key of the cluster queue.
* `description` - (Required) This is the description of the cluster queue.

## Attribute Reference

* `id` - The Graphql ID of the created cluster queue.
* `uuid` - The UUID of the created cluster queue.
* `cluster_uuid` - The UUID of the cluster that this cluster queue belongs to.