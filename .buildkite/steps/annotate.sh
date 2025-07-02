#!/bin/bash
set -ueo pipefail

go get gotest.tools/gotestsum

make testacc | tee test_output

printf '```term\n%b\n```' "$(cat test_output)" | buildkite-agent annotate --style info --context "testacc"
