# Data Source: cluster

Use this data source to look up properties on a cluster identified by its name.

Buildkite Documentation: https://buildkite.com/docs/clusters/overview

## Example Usage

You can find a cluster by name and use it to assign a pipeline to the cluster.
Also note that you can create the cluster using the Buildkite Terraform provider as well (including importing).

```hcl
data "buildkite_cluster" "default" {
    name = "default"
}

resource "buildkite_pipeline" "deploy" {
  name = "deploy"
  repository = "git@github.com:org/repo"
  cluster_id = data.buildkite_cluster.default.id
}
```

## Argument Reference

* `name` - (Required) The name of the cluster to lookup.

## Attributes Reference

* `id` - The GraphQL ID of the cluster.
* `uuid` - The UUID of the cluster.
* `description` - The description of the cluster.
* `emoji` - The emoji given the cluster.
* `color` - The color given the cluster.
