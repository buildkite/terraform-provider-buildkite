# Resource: cluster

This resource allows you to create, manage and import Clusters.

Buildkite documentation: https://buildkite.com/docs/clusters/overview

## Example Usage

```hcl
provider "buildkite" {}

resource "buildkite_cluster" "linux" {
    name = "linux_builds"
}
```

## Argument Reference

* `name` - (Required) This is the name of the cluster.
* `description` - (Optional) This is a description for the cluster, this may describe the usage for it, the region, or something else which would help identify the Cluster's purpose.
* `emoji` - (Optional) An emoji to use with the Cluster, this can either be set using `:buildkite:` notation, or with the emoji itself, such as 😎.
* `color` - (Optional) A color to associate with the Cluster. Perhaps a team related color, or one related to an environment. This is set using hex value, such as `#BADA55`.

## Attribute Reference

* `id` - The GraphQL ID that is created with the Cluster.
* `uuid` - The UUID created with the Cluster.


## import
Clusters can be imported using their `ID`, this can be found in the Clusters's `Settings` page, e.g.

```shell
terraform import buildkite_cluster.foo Q2x1c3Rlci0tLTI3ZmFmZjA4LTA3OWEtNDk5ZC1hMmIwLTIzNmY3NWFkMWZjYg==
```

A helpful GraphQL query to get the ID of the target cluster can be found below, or [this pre-saved query](https://buildkite.com/user/graphql/console/a803f254-decf-45a3-8332-a074b0a73483) can be used as a template. You'll need to substitute in your organization's `slug`.

```graphql
query getClusters {
  organization(slug: "ORGANIZATION"){
    clusters(first: 5, order:NAME) {
      edges{
        node {
          id
          name
        }
      }
    }
  }
}
```

