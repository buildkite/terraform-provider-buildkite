FROM golang:1.17.7@sha256:e06c83493ef6d69c95018da90f2887bf337470db074d3c648b8b648d8e3c441e

RUN apt-get update \
    && apt-get install -y unzip

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
