FROM golang:1.21.0-bookworm

RUN apt-get update && apt-get install -y \
  git \
  protobuf-compiler='3.21.12*' \
  clang-format='1:14.0*'

ARG PROTOC_GEN_VERSION
ARG GRPC_GATEWAY_VERSION

ENV PROTOC_GEN_GO_GRPC_VERSION="v1.1.0"
ENV GOCACHE=/tmp/build/.cache
ENV GOMODCACHE=/tmp/build/.modcache

RUN echo ${PROTOC_GEN_VERSION}

RUN cd /tmp \
  && mkdir -p /tmp/build/.cache \
  && mkdir -p /tmp/build/.modcache \
  && go install google.golang.org/protobuf/cmd/protoc-gen-go@${PROTOC_GEN_VERSION} \
  && go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@${PROTOC_GEN_GO_GRPC_VERSION} \
  && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@${GRPC_GATEWAY_VERSION} \
  && go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@${GRPC_GATEWAY_VERSION} \
  && chmod -R 777 /tmp/build/

WORKDIR /build

CMD ["/bin/sh", "/build/peerswaprpc/gen_protos.sh"]
