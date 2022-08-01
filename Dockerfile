FROM golang:1.18.4@sha256:8a62670f5902989319c4997fe21ecae7fe5aaec8ee44e4e663cfed0a3a8172fc

RUN apt-get update \
    && apt-get install -y unzip \
    && go install github.com/mfridman/tparse@latest \
    && go install github.com/lox/buildkite-test-analytics-go@latest

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
