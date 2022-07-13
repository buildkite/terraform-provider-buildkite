FROM golang:1.18.3@sha256:a452d6273ad03a47c2f29b898d6bb57630e77baf839651ef77d03e4e049c5bf3

RUN apt-get update \
    && apt-get install -y unzip \
    && go install github.com/mfridman/tparse@latest \
    && go install github.com/lox/buildkite-test-analytics-go@latest

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
