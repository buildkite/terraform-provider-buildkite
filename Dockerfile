FROM public.ecr.aws/docker/library/golang:1.22.0@sha256:7b297d9abee021bab9046e492506b3c2da8a3722cbf301653186545ecc1e00bb

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.4@sha256:5ca7188b7566703fd96c6f84c27d8e7aa4fe1c690803157264b8569580a99712 /bin/terraform /usr/local/bin/terraform
