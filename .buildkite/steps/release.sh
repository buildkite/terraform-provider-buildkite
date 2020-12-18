#!/bin/bash -e

# TODO: exit if HEAD isn't tagged with a release
# TODO: exit if the tagger release is already on GitHub

if [ -z "$BUILDKITE_TAG" ]
then
    echo
    echo "Releases only run when a new tag is pushed to github.com"
    echo
    exit 0
else
    echo "Preparing to release: ${BUILDKITE_TAG}"
fi

echo "--- Fetching git tags"

git show-ref --tags -d

get fetch origin

echo "--- importing GPG Secret Key"

if [ -z "$GPG_SECRET_KEY_BASE64" ]
then
      echo "\$GPG_SECRET_KEY_BASE64 env variable must contain a base64 encoded GPG secret key"
      exit 1
fi

echo "${GPG_SECRET_KEY_BASE64}" | base64 -d | gpg --import --no-tty --batch --yes

gpg --list-secret-keys

# GPG_FINGERPRINT is read by goreleaser
export GPG_FINGERPRINT="$(gpg --list-secret-keys --with-colons 2> /dev/null | grep '^sec:' | cut --delimiter ':' --fields 5)"

echo "GPG_FINGERPRINT=${GPG_FINGERPRINT}"

echo "--- Checking GitHub Token"
if [ -z "$GITHUB_TOKEN" ]
then
      echo "\$GITHUB_TOKEN env variable must contain a Github API token with permission to create releases in buildkite/terraform-provider-buildkite"
      exit 1
fi

echo "--- installing goreleaser"

curl -L -o /tmp/goreleaser_Linux_x86_64.tar.gz https://github.com/goreleaser/goreleaser/releases/download/v0.149.0/goreleaser_Linux_x86_64.tar.gz

cd /tmp && echo "a227362d734cda47f7ebed9762e6904edcd115a65084384ecfbad2baebc4c775  goreleaser_Linux_x86_64.tar.gz" | sha256sum -c

tar -zxvf goreleaser_Linux_x86_64.tar.gz

cp goreleaser /work

cd /work

echo "--- running goreleaser"

./goreleaser release --rm-dist
