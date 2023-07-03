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
* `description` - (Optional) The description of the cluster queue.

## Attribute Reference

* `id` - The Graphql ID of the created cluster queue.
* `uuid` - The UUID of the created cluster queue.
* `cluster_uuid` - The UUID of the cluster that this cluster queue belongs to.

## Import

Cluster queues can be imported using its `GraphQL ID`, along with its respective cluster `UUID`, separated by a comma. e.g.

```
$ terraform import buildkite_cluster_queue.test Q2x1c3RlclF1ZXVlLS0tYjJiOGRhNTEtOWY5My00Y2MyLTkyMjktMGRiNzg3ZDQzOTAz,35498aaf-ad05-4fa5-9a07-91bf6cacd2bd 
```

To find the cluster's `UUID` to utilize, you can use the below GraphQL query below. Alternatively, you can use this [pre-saved query](https://buildkite.com/user/graphql/console/3adf0389-02bd-45ef-adcd-4e8e5ae57f25), where you will need fo fill in the organization slug (ORGANIZATION_SLUG) for obtaining the relevant cluster name/`UUID` that the cluster queue is in.

```graphql
query getClusters {
  organization(slug: "ORGANIZATION_SLUG") {
	clusters(first: 50) {
      edges{
        node{
          name
          uuid
        }
      }
	  }
  }
}
```

After the cluster `UUID` has been found, you can use the below GraphQL query to find the cluster queue's `GraphQL ID`. Alternatively, this [pre-saved query](https://buildkite.com/user/graphql/console/1d913905-900e-40e7-8f46-651543487b5a) can be used, specifying the organization slug (ORGANIZATION_SLUG) and the cluster `UUID` from above (CLUSTER_UUID).

```graphql
query getClusterQueues {
  organization(slug: "ORGANIZATION_SLUG") {
    cluster(id: "CLUSTER_UUID") {
      queues(first: 50) {
        edges {
          node {
            id
            key
          }
        }
      }
    }
  }
}
```

