# Resource: cluster_agent_token

This resource allows you to create and manage cluster agent tokens.

Buildkite Documentation: https://buildkite.com/docs/clusters/manage-clusters#set-up-clusters-connect-agents-to-a-cluster

## Example Usage

```hcl
provider "buildkite" {}

resource "buildkite_cluster_agent_token" "cluster-token-1" {
  cluster_id = "Q2x1c3Rlci0tLTkyMmVjOTA4LWRmNWItNDhhYS1hMThjLTczMzE0YjQ1ZGYyMA==" 
  description = "agent token for cluster-1" 
}
```

## Argument Reference

* `cluster_id` - (Required) The ID of the cluster that this cluster queue belongs to.
* `description` -  (Required) A description about what this cluster agent token is used for.

## Attribute Reference

* `id` - The Graphql ID of the created cluster queue.
* `uuid` - The UUID of the created cluster queue.
* `cluster_uuid` - The UUID of the cluster that this cluster queue belongs to.
