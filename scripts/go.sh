#!/bin/sh
set -eu
root=$(cd "$(dirname "$0")/.." && pwd)
docker volume create gomod >/dev/null
docker volume create gocache >/dev/null
exec docker run --rm --network ynov \
  -v "$root":/src -v gomod:/go/pkg/mod -v gocache:/root/.cache/go-build \
  -e TEST_DATABASE_URL="${TEST_DATABASE_URL:-}" -e GOFLAGS=-buildvcs=false -e CGO_ENABLED=1 \
  -w /src golang:1.25-bookworm "$@"
