FROM golang:1.17.2@sha256:124966f5d54a41317ee81ccfe5f849d4f0deef4ed3c5c32c20be855c51c15027

RUN apt-get update \
    && apt-get install -y unzip

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
