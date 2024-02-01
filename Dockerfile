FROM public.ecr.aws/docker/library/golang:1.21.6@sha256:4d1942cb703999c3acd03b8efe5cc588cb5e39bace931d876de170e16d44e2cd

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.4@sha256:5ca7188b7566703fd96c6f84c27d8e7aa4fe1c690803157264b8569580a99712 /bin/terraform /usr/local/bin/terraform
