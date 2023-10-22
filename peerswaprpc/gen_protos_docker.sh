#!/bin/bash

set -e

# Directory of the script file, independent of where it's called from.
DIR="$(cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd)"

PROTOC_GEN_VERSION=$(go list -f '{{.Version}}' -m google.golang.org/protobuf)
GRPC_GATEWAY_VERSION=$(go list -f '{{.Version}}' -m github.com/grpc-ecosystem/grpc-gateway/v2)

echo "Building protobuf compiler docker image..."
docker build -t protobuf-builder \
  --build-arg PROTOC_GEN_VERSION="$PROTOC_GEN_VERSION" \
  --build-arg GRPC_GATEWAY_VERSION="$GRPC_GATEWAY_VERSION" \
  .

echo "Compiling and formatting *.proto files..."
docker run \
  --rm \
  --user "$UID:$(id -g)" \
  -e UID=$UID \
  -v "$DIR/../:/build" \
  protobuf-builder
