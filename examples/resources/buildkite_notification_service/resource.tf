resource "buildkite_notification_service" "webhook" {
  provider_type        = "webhook"
  description          = "Send finished builds to the deployment events service"
  branch_configuration = "main release/*"
  enabled              = true

  scope = "all"

  build_states = {
    build_failed = true
    build_fixed  = true
    build_passed = true
  }

  webhook = {
    url        = "https://example.com/buildkite-events"
    token_mode = "signature"
    events     = ["build.finished"]
  }
}

data "aws_caller_identity" "current" {}

resource "buildkite_notification_service" "event_bridge" {
  provider_type        = "aws_event_bridge"
  description          = "Send build events to Amazon EventBridge"
  branch_configuration = "main"
  enabled              = true

  scope = "all"

  build_states = {
    build_failed = true
    build_fixed  = true
    build_passed = true
  }

  aws_event_bridge = {
    aws_region     = "us-east-1"
    aws_account_id = data.aws_caller_identity.current.account_id
  }
}

resource "aws_cloudwatch_event_bus" "buildkite" {
  name              = buildkite_notification_service.event_bridge.aws_event_bridge.event_source_name
  event_source_name = buildkite_notification_service.event_bridge.aws_event_bridge.event_source_name
}

resource "aws_cloudwatch_event_rule" "buildkite_builds" {
  name           = "buildkite-build-events"
  description    = "Capture Buildkite build events"
  event_bus_name = aws_cloudwatch_event_bus.buildkite.name

  event_pattern = jsonencode({
    "detail-type" = ["Build Started", "Build Finished"]
  })
}
