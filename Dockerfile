FROM golang:1.18.0@sha256:5eb58ca0a747ed2e2f4e069d1116badb02a172cf160d31f801776a2342c12863

RUN apt-get update \
    && apt-get install -y unzip

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
