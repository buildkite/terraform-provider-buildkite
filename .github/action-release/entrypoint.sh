#!/bin/sh
set -eux

# get dependencies and build
go get -v ./...
go build

EXT=''
if [ "$GOOS" = "windows" ]; then
    EXT='.exe'
fi

# version the binary
TAG_NAME="$(cat $GITHUB_EVENT_PATH | jq -r .release.tag_name)"
NAME="terraform-provider-buildkite_${TAG_NAME}${EXT}"
UPLOAD_URL="$(cat $GITHUB_EVENT_PATH | jq -r .release.upload_url)"
UPLOAD_URL=${UPLOAD_URL/\{?name,label\}/} # remove portion of url
mv terraform-provider-buildkite${EXT} "${NAME}"

# archive it
tar cvfz tmp.tar.gz "${NAME}"
CHECKSUM=$(sha265sum tmp.tar.gz | cut -d' '-f 1)

# upload archive and sha to github release
curl -X POST -H 'Content-Type: application/gzip' -H "Authorization: Bearer ${GITHUB_TOKEN}" --data-binary @tmp.tar.gz "${UPLOAD_URL}?name=${NAME}_${GOOS}_${GOARCH}.tar.gz"
curl -X POST -H 'Content-Type: text/plain' -H "Authorization: Bearer ${GITHUB_TOKEN}" --data ${CHECKSUM} "${UPLOAD_URL}?name=${NAME}_${GOOS}_${GOARCH}_checksum.txt"
