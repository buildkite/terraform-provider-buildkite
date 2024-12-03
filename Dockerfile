FROM public.ecr.aws/docker/library/golang:1.23.3@sha256:73f06be4578c9987ce560087e2e2ea6485fb605e3910542cadd8fa09fc5f3e31

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.4@sha256:4dcb45513699e652c771b914f41ec1cc2a0ba9c8d1afa2e8e4aa2ba071b63151 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v1.26.2@sha256:e69fcf552e8eb2ce0d4c4a9b080b5f82ad9f040bb039a203667db0b5274ebfc3 /usr/bin/goreleaser /usr/local/bin/goreleaser
