#!/bin/bash
set -ueo pipefail

target="${1:-testacc}"
output="$(mktemp)"
trap 'rm -f "$output"' EXIT

make "$target" GOTESTSUM=gotestsum | tee "$output"

printf '```term\n%b\n```' "$(cat "$output")" | buildkite-agent annotate --style info --context "$target"
