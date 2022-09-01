FROM golang:1.19.0@sha256:d3f734e1f46ec36da8c1bce67cd48536138085289e24cfc8765f483c401b7d96

RUN apt-get update \
    && apt-get install -y unzip \
    && go install github.com/mfridman/tparse@latest \
    && go install github.com/lox/buildkite-test-analytics-go@latest

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
