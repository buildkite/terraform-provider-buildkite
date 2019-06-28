FROM golang:1.11

LABEL "com.github.actions.name"="Release Go Binary"
LABEL "com.github.actions.description"="Publish built Go binaries for GitHub releases"
LABEL "com.github.actions.icon"="cpu"
LABEL "com.github.actions.color"="purple"

LABEL "repository"="http://github.com/jradtilbrook/terraform-provider-buildkite"
LABEL "homepage"="http://github.com/jradtilbrook/terraform-provider-buildkite"
LABEL "maintainer"="Jarryd Tilbrook <jradtilbrook@gmail.com>"

ENV GO111MODULE=on

RUN apt update && apt install -y jq

ADD entrypoint.sh /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
