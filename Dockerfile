FROM 445615400570.dkr.ecr.us-east-1.amazonaws.com/ecr-public/docker/library/golang:1.20@sha256:f7099345b8e4a93c62dc5102e7eb19a9cdbad12e7e322644eeaba355d70e616d

RUN apt-get update \
    && apt-get install -y unzip \
    && go install github.com/mfridman/tparse@latest \
    && go install github.com/lox/buildkite-test-analytics-go@latest

COPY --from=hashicorp/terraform:1.4 /bin/terraform /usr/local/bin/terraform
