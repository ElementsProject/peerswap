#!/bin/sh

protoc -I. \
	--go_out=paths=source_relative:. \
	--go-grpc_out=paths=source_relative:. \
	--grpc-gateway_out=. \
	--grpc-gateway_opt logtostderr=true \
	--grpc-gateway_opt paths=source_relative \
	--grpc-gateway_opt grpc_api_configuration=peerswaprpc/peerswap.yaml \
	peerswaprpc/peerswaprpc.proto

protoc \
	--openapiv2_out=. \
	--openapiv2_opt logtostderr=true \
	--openapiv2_opt grpc_api_configuration=peerswaprpc/peerswap.yaml \
	peerswaprpc/peerswaprpc.protos
