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

After the cluster `UUID` has been found, you can use the below GraphQL query to find the cluster agent token's `GraphQL ID`. Alternatively, this [pre-saved query](https://buildkite.com/user/graphql/console/ad59d580-4b64-4a30-bd71-91dfc687296a) can be used, specifying the organization slug (ORGANIZATION_SLUG) and the cluster `UUID` from above (CLUSTER_UUID).

```graphql
query getClusterAgentTokens {
  organization(slug: "ORGANIZATION_SLUG") {
    cluster(id: "CLUSTER_UUID") {
      agentTokens {
        count
        edges {
          node {
            description
            id
            uuid 
          }
        }
      }
    }
  }
}
```