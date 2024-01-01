FROM public.ecr.aws/docker/library/golang:1.21.5@sha256:672a2286da3ee7a854c3e0a56e0838918d0dbb1c18652992930293312de898a6

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.4@sha256:5ca7188b7566703fd96c6f84c27d8e7aa4fe1c690803157264b8569580a99712 /bin/terraform /usr/local/bin/terraform
