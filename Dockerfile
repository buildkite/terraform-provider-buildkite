FROM public.ecr.aws/docker/library/golang:1.22.4@sha256:0f7691253e132744318ff4b02e0824fec9fa4f3b43b08afd1195599660d51521

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.4@sha256:5ca7188b7566703fd96c6f84c27d8e7aa4fe1c690803157264b8569580a99712 /bin/terraform /usr/local/bin/terraform
COPY --from=goreleaser/goreleaser:v1.26.2@sha256:e69fcf552e8eb2ce0d4c4a9b080b5f82ad9f040bb039a203667db0b5274ebfc3 /usr/bin/goreleaser /usr/local/bin/goreleaser
