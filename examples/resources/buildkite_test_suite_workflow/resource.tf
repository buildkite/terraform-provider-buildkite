data "buildkite_test_suite" "application" {
  slug = "application"
}

resource "buildkite_test_suite_workflow" "flaky" {
  test_suite_uuid = data.buildkite_test_suite.application.uuid

  # Settings, filters, and actions update in place. Changing the monitor type or suite replaces the workflow.
  monitor = {
    type              = "transition_count"
    window_size       = 100
    alarm_threshold   = 0.05
    recover_threshold = 0
  }

  filters = [
    { key = "scm.branch", operator = "is", value = "main" }
  ]

  alarm_actions = [
    { type = "add_label", label = "flaky" }
  ]

  recover_actions = [
    { type = "remove_label", label = "flaky" }
  ]
}
