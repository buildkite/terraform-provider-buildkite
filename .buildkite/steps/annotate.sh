#!/bin/bash -e

go test -v -cover -json ./... | tee test_output

tparse -all -file test_output | tee tparse_output

printf '```term\n%b\n```' "$(cat tparse_output)" | buildkite-agent annotate --style info
