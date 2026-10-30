#!/bin/bash
set -ueo pipefail

make testacc GOTESTSUM=gotestsum | tee test_output

printf '```term\n%b\n```' "$(cat test_output)" | buildkite-agent annotate --style info --context "testacc"
