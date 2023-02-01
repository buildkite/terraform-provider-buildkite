FROM golang:1.19.5@sha256:bb9811fad43a7d6fd2173248d8331b2dcf5ac9af20976b1937ecd214c5b8c383

RUN apt-get update \
    && apt-get install -y unzip \
    && go install github.com/mfridman/tparse@latest \
    && go install github.com/lox/buildkite-test-analytics-go@latest

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
