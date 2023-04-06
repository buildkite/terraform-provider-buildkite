#!/bin/bash
set -ueo pipefail

annotate(){
  go test -v -cover -json ./... | tee test_output
  tparse -all -file test_output | tee tparse_output
}

exit_code="$(annotate 2>&1)" || ret=$?

printf "Annotation exited with code $ret"

printf '```term\n%b\n```' "$(cat tparse_output)" | buildkite-agent annotate --style info

buildkite-test-analytics-go < test_output \
  --ci buildkite \
  --key "$BUILDKITE_BUILD_ID" \
  --build-number "$BUILDKITE_BUILD_NUMBER" \
  --job-id "$BUILDKITE_JOB_ID" \
  --branch "$BUILDKITE_BRANCH" \
  --commit-sha "$BUILDKITE_COMMIT" \
  --message "$BUILDKITE_MESSAGE" \
  --build-url "$BUILDKITE_BUILD_URL"
