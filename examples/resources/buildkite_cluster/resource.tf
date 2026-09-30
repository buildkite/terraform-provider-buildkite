# create a cluster
resource "buildkite_cluster" "primary" {
  name        = "Primary cluster"
  description = "Runs the monolith build and deploy"
  emoji       = "🚀"
  color       = "#bada55"
}

# add a pipeline to the cluster
resource "buildkite_pipeline" "monolith" {
  name       = "Monolith"
  repository = "https://github.com/..."
  cluster_id = buildkite_cluster.primary.id
}

resource "buildkite_cluster_queue" "default" {
  cluster_id = buildkite_cluster.primary.id
  key        = "default"
}

# send agent traces from a cluster to an OpenTelemetry tracing notification service, which must be
# enabled, apply to all pipelines, and have no branch filter
resource "buildkite_notification_service" "otel" {
  provider_type = "open_telemetry_tracing"
  description   = "Agent traces"

  open_telemetry_tracing = {
    endpoint = "https://otel.example.com"
  }
}

resource "buildkite_cluster" "traced" {
  name                       = "Traced cluster"
  agent_tracing_service_uuid = buildkite_notification_service.otel.id
}
