FROM 445615400570.dkr.ecr.us-east-1.amazonaws.com/ecr-public/docker/library/golang:1.20@sha256:f7099345b8e4a93c62dc5102e7eb19a9cdbad12e7e322644eeaba355d70e616d

RUN apt-get update \
    && apt-get install -y unzip

COPY --from=hashicorp/terraform:1.5.7@sha256:fb39302e870036eaf1bca0b83a4bc005b9edb3766a6f9b4eaa8163999226b441 /bin/terraform /usr/local/bin/terraform
