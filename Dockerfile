FROM public.ecr.aws/docker/library/golang:1.22.1@sha256:0b55ab82ac2a54a6f8f85ec8b943b9e470c39e32c109b766bbc1b801f3fa8d3b

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.4@sha256:5ca7188b7566703fd96c6f84c27d8e7aa4fe1c690803157264b8569580a99712 /bin/terraform /usr/local/bin/terraform
