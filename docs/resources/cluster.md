# Resource: cluster

This resource allows you to create, manage and import Clusters.

Buildkite documentation: https://buildkite.com/docs/clusters/overview

## Example Usage

```hcl
provider "buildkitE" {}

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

The `uuid` is used when *getting* a Cluster. The `id` is used when *modifying* a Cluster.   
