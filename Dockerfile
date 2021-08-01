FROM golang:1.16.6@sha256:4544ae57fc735d7e415603d194d9fb09589b8ad7acd4d66e928eabfb1ed85ff1

RUN apt-get update \
    && apt-get install -y unzip

ENV TERRAFORM_VERSION=0.14.11

RUN cd /tmp && \
    curl -O https://releases.hashicorp.com/terraform/${TERRAFORM_VERSION}/terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    unzip terraform_${TERRAFORM_VERSION}_linux_amd64.zip && \
    cp terraform /usr/local/bin/terraform
