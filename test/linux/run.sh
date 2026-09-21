#!/usr/bin/env bash
# Build the Linux CLI and run its behavioural tests in a container.
#
#     bash test/linux/run.sh            amd64
#     ARCH=arm64 bash test/linux/run.sh arm64, if the daemon can run it
#
# Nothing here touches the developer's real config or Cloudflare account: the
# container gets its own home directory and a fixture token.
set -euo pipefail

cd "$(dirname "$0")/../.."

ARCH="${ARCH:-amd64}"
IMAGE="quickflare-linux-test:${ARCH}"
STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT

echo "==> Building the CLI for linux/${ARCH}"
GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 \
    go build -trimpath -ldflags "-s -w" -o "$STAGE/quickflare" ./cmd/quickflare-cli

cp test/linux/Dockerfile test/linux/run-tests.sh "$STAGE/"

echo "==> Building the image"
docker build --quiet --platform "linux/${ARCH}" -t "$IMAGE" "$STAGE" >/dev/null

echo "==> Running tests"
docker run --rm --platform "linux/${ARCH}" "$IMAGE"
