# buildkite_registry (Resource)

A Registry in Buildkite is a secure storage location for packages and artifacts that can be used across your pipelines. It supports various package formats including Java, Ruby, and others. Registries enable teams to store and share private packages within their organization while maintaining access control and versioning.

## Example Usage

```terraform
resource "buildkite_registry" "maven" {
  name        = "maven-registry"
  ecosystem   = "java"
  description = "Organization-wide Maven package registry"
  emoji       = ":java:"
  color       = "#BADA55"
  team_ids    = [buildkite_team.example.id, buildkite_team.example2.id]
}
```
